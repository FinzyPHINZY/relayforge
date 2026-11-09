package security

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// ValidateEndpointURL checks registration input and all current DNS answers.
// The sender must repeat resolution and pin the approved IP at connection time.
func ValidateEndpointURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("endpoint must be an HTTPS URL without credentials or fragment")
	}
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("unsupported endpoint port")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("private endpoint host")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if !publicIP(addr) {
			return errors.New("private endpoint address")
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	answers, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(answers) == 0 {
		return errors.New("endpoint DNS resolution failed")
	}
	for _, answer := range answers {
		addr, ok := netip.AddrFromSlice(answer.IP)
		if !ok || !publicIP(addr) {
			return errors.New("endpoint resolves to a private address")
		}
	}
	return nil
}

func ValidateEndpointURLWithDemo(ctx context.Context, raw, origin string) error {
	if !isDemoURL(raw, origin) {
		return ValidateEndpointURL(ctx, raw)
	}
	u, _ := url.Parse(raw)
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	answers, err := net.DefaultResolver.LookupIPAddr(lookupCtx, u.Hostname())
	if err != nil || len(answers) == 0 {
		return errors.New("demo receiver DNS unavailable")
	}
	for _, a := range answers {
		ip, ok := netip.AddrFromSlice(a.IP)
		if !ok || !ip.Unmap().IsPrivate() {
			return errors.New("demo receiver must use a private Docker address")
		}
	}
	return nil
}

func isDemoURL(raw, origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	o, err := url.Parse(origin)
	if err != nil || o.Scheme != "http" || o.Host == "" || o.User != nil || o.Path != "" || o.RawQuery != "" || o.Fragment != "" {
		return false
	}
	return u.Scheme == "http" && strings.EqualFold(u.Host, o.Host)
}

func publicIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	blocked := []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8"}
	for _, cidr := range blocked {
		if netip.MustParsePrefix(cidr).Contains(addr) {
			return false
		}
	}
	return true
}
