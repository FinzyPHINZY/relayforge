package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL, ListenAddr, EncryptionKeyID, RedisURL string
	EncryptionKey                                      []byte
	EventRateLimit, ManagementRateLimit                int
	DemoReceiverOrigin                                 string
	RegionRole                                         string

	// Accounts (optional; wired in cmd/api later).
	DashboardPublicURL, MailFrom, MailProvider, AWSRegion string
	MailLogBodies                                         bool
	IPHashKey                                             []byte
	MagicLinkPerEmailPerHour, MagicLinkPerIPPerHour       int
	GitHubClientID, GitHubClientSecret                    string
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), ListenAddr: os.Getenv("LISTEN_ADDR"), EncryptionKeyID: os.Getenv("ENCRYPTION_KEY_ID"), RedisURL: os.Getenv("REDIS_URL")}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.EncryptionKeyID == "" {
		c.EncryptionKeyID = "local-v1"
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	var err error
	c.EncryptionKey, err = base64.StdEncoding.DecodeString(os.Getenv("ENCRYPTION_KEY_BASE64"))
	if err != nil || len(c.EncryptionKey) != 32 {
		return c, errors.New("ENCRYPTION_KEY_BASE64 must encode 32 bytes")
	}
	c.EventRateLimit, err = positiveEnv("EVENT_RATE_LIMIT_PER_MINUTE", 600)
	if err != nil {
		return c, err
	}
	c.ManagementRateLimit, err = positiveEnv("MANAGEMENT_RATE_LIMIT_PER_MINUTE", 120)
	if err != nil {
		return c, err
	}
	c.DemoReceiverOrigin = os.Getenv("DEMO_RECEIVER_ORIGIN")
	c.RegionRole = os.Getenv("REGION_ROLE")
	if c.RegionRole == "" {
		c.RegionRole = "primary"
	}
	if c.RegionRole != "primary" && c.RegionRole != "standby" {
		return c, errors.New("REGION_ROLE must be primary or standby")
	}
	if c.DemoReceiverOrigin != "" && c.DemoReceiverOrigin != "http://receiver:8090" {
		return c, errors.New("DEMO_RECEIVER_ORIGIN must be http://receiver:8090 or empty")
	}
	c.DashboardPublicURL = strings.TrimRight(os.Getenv("DASHBOARD_PUBLIC_URL"), "/")
	c.MailFrom = os.Getenv("MAIL_FROM")
	c.MailProvider = os.Getenv("MAIL_PROVIDER")
	if c.MailProvider == "" {
		c.MailProvider = "log"
	}
	c.AWSRegion = os.Getenv("AWS_REGION")
	if raw := os.Getenv("MAIL_LOG_BODIES"); raw != "" {
		c.MailLogBodies, err = strconv.ParseBool(raw)
		if err != nil {
			return c, errors.New("MAIL_LOG_BODIES must be a boolean")
		}
	}
	switch c.MailProvider {
	case "log":
	case "ses":
		if c.AWSRegion != "eu-north-1" {
			return c, errors.New("AWS_REGION must be eu-north-1 when MAIL_PROVIDER is ses")
		}
		if c.MailFrom == "" {
			return c, errors.New("MAIL_FROM is required when MAIL_PROVIDER is ses")
		}
	default:
		return c, errors.New("MAIL_PROVIDER must be log or ses")
	}
	if raw := os.Getenv("IP_HASH_KEY_BASE64"); raw != "" {
		c.IPHashKey, err = base64.StdEncoding.DecodeString(raw)
		if err != nil || len(c.IPHashKey) != 32 {
			return c, errors.New("IP_HASH_KEY_BASE64 must encode 32 bytes")
		}
	}
	c.MagicLinkPerEmailPerHour, err = positiveEnv("MAGIC_LINK_PER_EMAIL_PER_HOUR", 5)
	if err != nil {
		return c, err
	}
	c.MagicLinkPerIPPerHour, err = positiveEnv("MAGIC_LINK_PER_IP_PER_HOUR", 20)
	if err != nil {
		return c, err
	}
	c.GitHubClientID = os.Getenv("GITHUB_CLIENT_ID")
	c.GitHubClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	if (c.GitHubClientID == "") != (c.GitHubClientSecret == "") {
		return c, errors.New("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET must be set together")
	}
	return c, nil
}

func positiveEnv(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100000 {
		return 0, errors.New(name + " must be between 1 and 100000")
	}
	return n, nil
}
