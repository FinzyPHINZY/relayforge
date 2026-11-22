package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	const token = "dashboard-token-0123456789abcdef0123"
	cases := []struct {
		name, configured, sentToken, header, want string
	}{
		{"trusted header", token, token, "203.0.113.9", "203.0.113.9"},
		{"trusted ipv6 header", token, token, "2001:db8::1", "2001:db8::1"},
		{"wrong token", token, token + "x", "203.0.113.9", "192.0.2.1"},
		{"missing token", token, "", "203.0.113.9", "192.0.2.1"},
		{"unconfigured token never trusts", "", "", "203.0.113.9", "192.0.2.1"},
		{"unparseable header falls back", token, token, "not-an-ip", "192.0.2.1"},
		{"no header", token, token, "", "192.0.2.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/auth/providers", nil)
			r.RemoteAddr = "192.0.2.1:54321"
			if c.sentToken != "" {
				r.Header.Set("X-RelayForge-Dashboard-Token", c.sentToken)
			}
			if c.header != "" {
				r.Header.Set("X-RelayForge-Client-IP", c.header)
			}
			if got := clientIP(r, c.configured); got != c.want {
				t.Fatalf("clientIP=%q want %q", got, c.want)
			}
		})
	}
}
