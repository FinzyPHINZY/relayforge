package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
)

var eventEmailPattern = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

// Redaction is conservative at every nesting level. This endpoint is still
// restricted because unrecognized user-defined fields can contain sensitive
// values; it is an operator inspection aid, not a data-loss-prevention system.
func redactEventValue(value any, depth int) any {
	if depth > 32 {
		return "[REDACTED: depth limit]"
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(key))
			sensitive := false
			for _, part := range []string{"password", "passwd", "secret", "token", "authorization", "credential", "apikey", "privatekey", "ssn", "cardnumber", "email"} {
				if strings.Contains(normalized, part) {
					sensitive = true
					break
				}
			}
			if sensitive {
				out[key] = "[REDACTED]"
			} else {
				out[key] = redactEventValue(child, depth+1)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = redactEventValue(child, depth+1)
		}
		return out
	case string:
		lower := strings.ToLower(v)
		if strings.Contains(lower, "bearer ") || strings.Contains(lower, "whk_") || strings.Contains(lower, "whsec_") || strings.Contains(lower, "sk_live_") {
			return "[REDACTED]"
		}
		return eventEmailPattern.ReplaceAllString(v, "[REDACTED_EMAIL]")
	}
	return value
}

func (s *Server) revealEventPayload(w http.ResponseWriter, r *http.Request) {
	role := r.Context().Value(principalKey).(principal).Role
	if role != "owner" && role != "admin" {
		problem(w, 403, "forbidden", "Owner or admin role required")
		return
	}
	eventID := chi.URLParam(r, "id")
	if !id.Valid(eventID) {
		problem(w, 404, "not_found", "Event not found")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var raw []byte
	err = tx.QueryRow(r.Context(), `SELECT payload FROM events WHERE id=$1 AND organization_id=$2`, eventID, tenant(r)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Event not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err := writeAudit(r, tx, "event.payload_viewed", "event", eventID, nil); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"data": redactEventValue(data, 0)})
}
