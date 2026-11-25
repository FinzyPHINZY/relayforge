package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"

	"webhook_delivery/internal/accounts"
)

func validDashboardToken(r *http.Request, dashboardToken string) bool {
	if dashboardToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-RelayForge-Dashboard-Token")), []byte(dashboardToken)) == 1
}

// clientIP returns X-RelayForge-Client-IP only for requests from the trusted
// dashboard (matching token, parseable IP); otherwise the RemoteAddr host.
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
