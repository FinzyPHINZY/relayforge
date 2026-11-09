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
	"webhook_delivery/internal/config"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/id"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database setup failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		slog.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	workerID, err := id.New()
	if err != nil {
		slog.Error("ID generation failed", "error", err)
		os.Exit(1)
	}
	concurrency := 10
	if raw := os.Getenv("WORKER_CONCURRENCY"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			slog.Error("invalid worker concurrency")
			os.Exit(1)
		}
		concurrency = n
	}
	engine := delivery.Engine{DB: pool, Sender: delivery.Sender{DB: pool, EncryptionKey: cfg.EncryptionKey, EncryptionKeyID: cfg.EncryptionKeyID}, WorkerID: workerID, Concurrency: concurrency, Lease: 45 * time.Second}
	engine.Run(ctx)
}
