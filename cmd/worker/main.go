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
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/observability"
)

func main() {
	shutdownTracing := observability.Start("relayforge-worker")
	defer shutdownTracing(context.Background())
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
	go health.Serve(ctx, pool, ":8081")
	if cfg.RegionRole == "standby" {
		slog.Info("standby region: worker is idle")
		<-ctx.Done()
		return
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
	endpointConcurrency := 2
	if raw := os.Getenv("ENDPOINT_CONCURRENCY_LIMIT"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			slog.Error("invalid endpoint concurrency limit")
			os.Exit(1)
		}
		endpointConcurrency = n
	}
	tenantConcurrency := 10
	if raw := os.Getenv("TENANT_CONCURRENCY_LIMIT"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 1000 {
			slog.Error("invalid tenant concurrency limit")
			os.Exit(1)
		}
		tenantConcurrency = n
	}
	drainSeconds := 35
	if raw := os.Getenv("WORKER_DRAIN_SECONDS"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 40 {
			slog.Error("invalid worker drain seconds")
			os.Exit(1)
		}
		drainSeconds = n
	}
	engine := delivery.Engine{DB: pool, Sender: delivery.Sender{DB: pool, EncryptionKey: cfg.EncryptionKey, EncryptionKeyID: cfg.EncryptionKeyID, DemoReceiverOrigin: cfg.DemoReceiverOrigin}, WorkerID: workerID, Concurrency: concurrency, EndpointConcurrency: endpointConcurrency, TenantConcurrency: tenantConcurrency, Lease: 45 * time.Second, DrainTimeout: time.Duration(drainSeconds) * time.Second}
	engine.Run(ctx)
}
