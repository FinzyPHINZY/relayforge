package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ENCRYPTION_KEY_BASE64", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	for _, k := range []string{"DASHBOARD_PUBLIC_URL", "MAIL_LOG_BODIES", "MAIL_PROVIDER", "IP_HASH_KEY_BASE64", "DASHBOARD_API_TOKEN", "GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET", "AWS_REGION", "MAIL_FROM"} {
		t.Setenv(k, "")
	}
}

func TestDashboardPublicURLValidation(t *testing.T) {
	good := map[string]string{
		"":                         "",
		"https://app.example.com":  "https://app.example.com",
		"https://app.example.com/": "https://app.example.com",
		"http://localhost:3000":    "http://localhost:3000",
		"http://127.0.0.1:3000/":   "http://127.0.0.1:3000",
	}
	for raw, want := range good {
		baseEnv(t)
		t.Setenv("DASHBOARD_PUBLIC_URL", raw)
		c, err := Load()
		if err != nil || c.DashboardPublicURL != want {
			t.Fatalf("%q: got %q, %v", raw, c.DashboardPublicURL, err)
		}
	}
	for _, raw := range []string{"app.example.com", "ftp://app.example.com", "https://app.example.com/dash", "https://app.example.com?x=1", "https://app.example.com/#f", "https://", "https://user@app.example.com", "javascript:alert(1)"} {
		baseEnv(t)
		t.Setenv("DASHBOARD_PUBLIC_URL", raw)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DASHBOARD_PUBLIC_URL") {
			t.Fatalf("%q: expected DASHBOARD_PUBLIC_URL error, got %v", raw, err)
		}
	}
}

func TestMailLogBodiesOnlyForLocalDashboard(t *testing.T) {
	for _, raw := range []string{"", "http://localhost:3000", "http://127.0.0.1:3000"} {
		baseEnv(t)
		t.Setenv("MAIL_LOG_BODIES", "true")
		t.Setenv("DASHBOARD_PUBLIC_URL", raw)
		if c, err := Load(); err != nil || !c.MailLogBodies {
			t.Fatalf("%q: %v", raw, err)
		}
	}
	for _, raw := range []string{"https://app.example.com", "http://localhost.example.com"} {
		baseEnv(t)
		t.Setenv("MAIL_LOG_BODIES", "true")
		t.Setenv("DASHBOARD_PUBLIC_URL", raw)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MAIL_LOG_BODIES") {
			t.Fatalf("%q: expected MAIL_LOG_BODIES error, got %v", raw, err)
		}
	}
	baseEnv(t)
	t.Setenv("MAIL_LOG_BODIES", "false")
	t.Setenv("DASHBOARD_PUBLIC_URL", "https://app.example.com")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestSignInMailUndelivered(t *testing.T) {
	enabled := func(c Config) Config {
		c.IPHashKey = make([]byte, 32)
		c.DashboardAPIToken = strings.Repeat("t", 32)
		return c
	}
	cases := []struct {
		c    Config
		want bool
	}{
		{enabled(Config{MailProvider: "log", DashboardPublicURL: "https://app.example.com"}), true},
		{enabled(Config{MailProvider: "ses", DashboardPublicURL: "https://app.example.com"}), false},
		{enabled(Config{MailProvider: "log", DashboardPublicURL: "http://localhost:3000"}), false},
		{enabled(Config{MailProvider: "log", DashboardPublicURL: "https://localhost:3000"}), false},
		{Config{MailProvider: "log", DashboardPublicURL: "https://app.example.com"}, false},
	}
	for i, tc := range cases {
		if got := tc.c.SignInMailUndelivered(); got != tc.want {
			t.Fatalf("case %d: got %v want %v", i, got, tc.want)
		}
	}
}
