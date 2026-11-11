package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/observability"
	"webhook_delivery/internal/retention"
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
	role := os.Getenv("REGION_ROLE")
	if role == "standby" {
		slog.Info("standby region: dispatcher is idle")
		<-ctx.Done()
		return
	}
	if role != "" && role != "primary" {
		slog.Error("invalid REGION_ROLE")
		os.Exit(1)
	}
	go retireExpiredSecrets(ctx, pool)
	days := 30
	if raw := os.Getenv("RETENTION_DAYS"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 30 || n > 3650 {
			slog.Error("invalid retention days")
			os.Exit(1)
		}
		days = n
	}
	go runRetention(ctx, pool, time.Duration(days)*24*time.Hour)
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

func runRetention(ctx context.Context, pool *pgxpool.Pool, age time.Duration) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		for i := 0; i < 100 && ctx.Err() == nil; i++ {
			now := time.Now().UTC()
			stats, err := retention.Sweep(ctx, pool, now, now.Add(-age), 100)
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("retention sweep failed", "error", err)
				}
				break
			}
			if stats.Events > 0 || stats.IdempotencyRecords > 0 {
				slog.Info("retention sweep", "events", stats.Events, "idempotency_records", stats.IdempotencyRecords)
			}
			if stats.Events < 100 && stats.IdempotencyRecords < 100 {
				break
			}
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
