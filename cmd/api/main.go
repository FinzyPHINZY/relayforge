package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/config"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/observability"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	shutdownTracing := observability.Start("relayforge-api")
	defer shutdownTracing(context.Background())
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
	healthCtx, stopHealth := context.WithCancel(ctx)
	defer stopHealth()
	go health.Serve(healthCtx, pool, ":8081")
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: (&api.Server{DB: pool, EncryptionKey: cfg.EncryptionKey, EncryptionKeyID: cfg.EncryptionKeyID}).Router(), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 * 1024}
	go func() {
		slog.Info("API listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "error", err)
	}
}
