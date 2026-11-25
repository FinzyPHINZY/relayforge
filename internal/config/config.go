package config

import (
	"encoding/base64"
	"errors"
	"net/url"
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

	DashboardPublicURL, MailFrom, MailProvider, AWSRegion string
	MailLogBodies                                         bool
	IPHashKey                                             []byte
	MagicLinkPerEmailPerHour, MagicLinkPerIPPerHour       int
	GitHubClientID, GitHubClientSecret                    string
	DashboardAPIToken                                     string
	OAuthStartPerIPPerHour                                int
	MaxWorkspacesPerUser                                  int
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
	if c.DashboardPublicURL != "" && !validOrigin(c.DashboardPublicURL) {
		return c, errors.New("DASHBOARD_PUBLIC_URL must be an http(s) origin with no path, query or fragment")
	}
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
	// logged bodies hold live sign-in links; only a local dashboard may use them.
	if c.MailLogBodies && c.DashboardPublicURL != "" && !localOrigin(c.DashboardPublicURL) {
		return c, errors.New("MAIL_LOG_BODIES may only be true when DASHBOARD_PUBLIC_URL is localhost or 127.0.0.1")
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
	c.OAuthStartPerIPPerHour, err = positiveEnv("OAUTH_START_PER_IP_PER_HOUR", 30)
	if err != nil {
		return c, err
	}
	c.MaxWorkspacesPerUser = 3
	if raw := os.Getenv("MAX_WORKSPACES_PER_USER"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return c, errors.New("MAX_WORKSPACES_PER_USER must be between 1 and 100")
		}
		c.MaxWorkspacesPerUser = n
	}
	c.DashboardAPIToken = os.Getenv("DASHBOARD_API_TOKEN")
	if c.DashboardAPIToken != "" && len(c.DashboardAPIToken) < 32 {
		return c, errors.New("DASHBOARD_API_TOKEN must be at least 32 characters")
	}
	c.GitHubClientID = os.Getenv("GITHUB_CLIENT_ID")
	c.GitHubClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	if (c.GitHubClientID == "") != (c.GitHubClientSecret == "") {
		return c, errors.New("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET must be set together")
	}
	return c, nil
}

func (c Config) EmailSignInEnabled() bool {
	return c.DashboardPublicURL != "" && len(c.IPHashKey) == 32 && c.DashboardAPIToken != ""
}

func (c Config) SignInMailUndelivered() bool {
	if !c.EmailSignInEnabled() || c.MailProvider != "log" {
		return false
	}
	u, err := url.Parse(c.DashboardPublicURL)
	return err == nil && u.Scheme == "https" && !localOrigin(c.DashboardPublicURL)
}

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != "" &&
		u.User == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.Contains(raw, "#") && u.Opaque == ""
}

func localOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
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
