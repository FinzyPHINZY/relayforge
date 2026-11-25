package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/accounts"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/config"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/mail"
	"webhook_delivery/internal/observability"
	"webhook_delivery/internal/ratelimit"
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
	var limiter *ratelimit.Redis
	var distributedLimiter api.DistributedRateLimiter
	if cfg.RedisURL != "" {
		client, err := ratelimit.NewClient(cfg.RedisURL)
		if err != nil {
			slog.Error("redis configuration failed", "error", err)
			os.Exit(1)
		}
		defer client.Close()
		limiter = ratelimit.NewRedis(client)
		distributedLimiter = limiter
	}
	healthCtx, stopHealth := context.WithCancel(ctx)
	defer stopHealth()
	if limiter == nil {
		go health.Serve(healthCtx, pool, ":8081")
	} else {
		go health.Serve(healthCtx, pool, ":8081", limiter.Ping)
	}
	var mailer mail.Mailer = &mail.LogMailer{Logger: slog.Default(), LogBody: cfg.MailLogBodies}
	if cfg.MailProvider == "ses" {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
		if err != nil {
			slog.Error("AWS configuration failed", "error", err)
			os.Exit(1)
		}
		mailer = &mail.SESMailer{Client: sesv2.NewFromConfig(awsCfg), From: cfg.MailFrom}
	}
	if cfg.SignInMailUndelivered() {
		slog.Warn("email sign-in is enabled but MAIL_PROVIDER=log: sign-in emails are written to the log, not delivered")
	}
	magicLinks := accounts.MagicLinkConfig{DashboardURL: cfg.DashboardPublicURL, Mailer: mailer, PerEmailPerHour: cfg.MagicLinkPerEmailPerHour, PerIPPerHour: cfg.MagicLinkPerIPPerHour}
	if limiter != nil { // never store a typed nil: a nil limiter makes sign-in fail closed
		magicLinks.Limiter = limiter
	}
	var github *accounts.GitHubConfig
	if cfg.GitHubClientID != "" && cfg.DashboardPublicURL != "" {
		github = &accounts.GitHubConfig{ClientID: cfg.GitHubClientID, ClientSecret: cfg.GitHubClientSecret, RedirectURL: cfg.DashboardPublicURL + "/auth/github/callback"}
	}
	server := &api.Server{
		DB: pool, DistributedLimiter: distributedLimiter, EncryptionKey: cfg.EncryptionKey, EncryptionKeyID: cfg.EncryptionKeyID,
		EventRateLimit: cfg.EventRateLimit, ManagementRateLimit: cfg.ManagementRateLimit, DemoReceiverOrigin: cfg.DemoReceiverOrigin, RegionRole: cfg.RegionRole,
		Accounts: &accounts.Store{DB: pool, IPHashKey: cfg.IPHashKey}, MagicLinks: magicLinks, GitHub: github,
		DashboardToken: cfg.DashboardAPIToken, OAuthStartPerIPPerHour: cfg.OAuthStartPerIPPerHour,
		MaxWorkspacesPerUser: cfg.MaxWorkspacesPerUser,
	}
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: server.Router(), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 * 1024}
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
