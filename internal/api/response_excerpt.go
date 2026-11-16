package api

import "strings"

// A receiver controls its response body. Only short, known diagnostic phrases
// are shown to operators; arbitrary bodies can contain credentials or PII.
func safeResponseExcerpt(raw *string) *string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	value := strings.TrimSpace(*raw)
	switch strings.ToLower(value) {
	case "bad request", "unauthorized", "forbidden", "not found", "request timeout", "too many requests", "rate limited", "retry later", "service unavailable", "internal server error", "bad gateway", "gateway timeout":
		return &value
	default:
		withheld := "[Response body withheld]"
		return &withheld
	}
}
