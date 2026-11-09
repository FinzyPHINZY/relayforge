package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/observability"
)

func main() {
	shutdownTracing := observability.Start("relayforge-dispatcher")
	defer shutdownTracing(context.Background())
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database setup failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		slog.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	go health.Serve(ctx, pool, ":8081")
	go retireExpiredSecrets(ctx, pool)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		count, err := delivery.DispatchBatch(ctx, pool, 50)
		if err != nil && ctx.Err() == nil {
			slog.Error("dispatch failed", "error", err)
		}
		if count > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func retireExpiredSecrets(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := pool.Exec(ctx, `UPDATE endpoint_secrets SET state='retired',retired_at=overlap_until WHERE state='overlap' AND overlap_until<=now()`); err != nil && ctx.Err() == nil {
				slog.Error("secret retirement failed", "error", err)
			}
		}
	}
}
