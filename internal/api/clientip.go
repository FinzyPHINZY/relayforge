package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"

	"webhook_delivery/internal/accounts"
)

// validDashboardToken reports whether the request carries the configured
// dashboard token. An unconfigured token never matches.
func validDashboardToken(r *http.Request, dashboardToken string) bool {
	if dashboardToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-RelayForge-Dashboard-Token")), []byte(dashboardToken)) == 1
}

// clientIP returns X-RelayForge-Client-IP only for requests from the trusted
// dashboard (matching token, parseable IP); otherwise the RemoteAddr host.
// The result is normalised (accounts.NormaliseIP: unmapped IPv4, IPv6 /64)
// because it is only ever used as a rate-limit subject or hash input.
func clientIP(r *http.Request, dashboardToken string) string {
	if validDashboardToken(r, dashboardToken) {
		if addr, err := netip.ParseAddr(r.Header.Get("X-RelayForge-Client-IP")); err == nil {
			return accounts.NormaliseIP(addr.String())
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return accounts.NormaliseIP(r.RemoteAddr)
	}
	return accounts.NormaliseIP(host)
}
