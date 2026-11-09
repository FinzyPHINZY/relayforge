package security

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// PinnedClient resolves immediately before sending and dials only an approved
// address. Each call creates a fresh transport so no prior connection bypasses
// DNS revalidation. HTTP is restricted to loopback test fixtures when enabled.
func PinnedClient(ctx context.Context, raw string, timeout time.Duration, allowHTTPTest bool) (*http.Client, error) {
	return pinnedClientWithLookup(ctx, raw, timeout, allowHTTPTest, net.DefaultResolver.LookupIPAddr)
}

func pinnedClientWithLookup(ctx context.Context, raw string, timeout time.Duration, allowHTTPTest bool, lookup func(context.Context, string) ([]net.IPAddr, error)) (*http.Client, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("invalid destination URL")
	}
	if u.Scheme != "https" && !(allowHTTPTest && u.Scheme == "http") {
		return nil, errors.New("HTTPS required")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if u.Scheme == "https" && port != "443" {
		return nil, errors.New("unsupported HTTPS port")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	addresses := []netip.Addr{}
	if ip, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = append(addresses, ip.Unmap())
	} else {
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		answers, lookupErr := lookup(lookupCtx, host)
		if lookupErr != nil || len(answers) == 0 {
			return nil, errors.New("destination DNS unavailable")
		}
		for _, answer := range answers {
			ip, ok := netip.AddrFromSlice(answer.IP)
			if !ok {
				return nil, errors.New("invalid DNS answer")
			}
			addresses = append(addresses, ip.Unmap())
		}
	}
	for _, ip := range addresses {
		if u.Scheme == "https" && !publicIP(ip) {
			return nil, errors.New("blocked destination address")
		}
		if u.Scheme == "http" && !ip.IsLoopback() {
			return nil, errors.New("test HTTP must use loopback")
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("destination has no addresses")
	}
	pinned := net.JoinHostPort(addresses[0].String(), port)
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12},
		DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(dialCtx, "tcp", pinned)
		},
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: timeout, MaxResponseHeaderBytes: 16 * 1024,
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
