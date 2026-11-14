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
	"webhook_delivery/internal/monitor"
	"webhook_delivery/internal/security"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	webhookURL := os.Getenv("MONITOR_WEBHOOK_URL")
	if databaseURL == "" || webhookURL == "" {
		slog.Error("DATABASE_URL and MONITOR_WEBHOOK_URL are required")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	validateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := security.ValidateEndpointURL(validateCtx, webhookURL); err != nil {
		cancel()
		slog.Error("invalid monitor notification destination")
		os.Exit(1)
	}
	cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		slog.Error("database setup failed")
		os.Exit(1)
	}
	defer pool.Close()
	backlogSeconds := envSeconds("MONITOR_BACKLOG_AGE_SECONDS", 300, 60, 86400)
	intervalSeconds := envSeconds("MONITOR_INTERVAL_SECONDS", 60, 15, 3600)
	checker := monitor.Checker{DB: pool, Services: map[string]string{
		"api": "http://api:8081/health/ready", "dispatcher": "http://dispatcher:8081/health/ready", "worker": "http://worker:8081/health/ready",
	}, BacklogAgeLimit: time.Duration(backlogSeconds) * time.Second}
	notifier := monitor.WebhookNotifier{URL: webhookURL}
	m := monitor.Monitor{Check: checker.Check, Notify: notifier.Notify}
	slog.Info("monitor started", "interval_seconds", intervalSeconds, "backlog_age_seconds", backlogSeconds)
	m.Run(ctx, time.Duration(intervalSeconds)*time.Second)
}

func envSeconds(name string, fallback, min, max int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		slog.Error("invalid monitor interval or threshold", "name", name)
		os.Exit(1)
	}
	return n
}
