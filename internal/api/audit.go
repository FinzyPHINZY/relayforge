package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/audit"
	"webhook_delivery/internal/id"
)

func writeAudit(r *http.Request, tx pgx.Tx, action, targetType, targetID string, details map[string]any) error {
	p := r.Context().Value(principalKey).(principal)
	return audit.Write(r.Context(), tx, audit.Entry{
		OrganizationID: p.OrgID, ActorKeyID: p.KeyID, ActorUserID: p.UserID, ActorRole: p.Role,
		Action: action, TargetType: targetType, TargetID: targetID, Details: details,
	})
}

type auditView struct {
	ID            string          `json:"id"`
	ActorAPIKeyID *string         `json:"actor_api_key_id,omitempty"`
	ActorKeyName  string          `json:"actor_key_name"`
	ActorRole     string          `json:"actor_role"`
	Action        string          `json:"action"`
	TargetType    string          `json:"target_type"`
	TargetID      string          `json:"target_id"`
	Details       json.RawMessage `json:"details"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Audit records contain identifiers and allowlisted metadata only. Owners can
// inspect their own organization; records remain after individual keys change.
func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "owner", "admin") {
		return
	}
	limit, cursor, err := pageParams(r)
	if err != nil || (cursor != nil && !id.Valid(cursor.ID)) {
		problem(w, 400, "invalid_pagination", "Invalid audit page")
		return
	}
	var rows pgx.Rows
	const columns = `a.id,a.actor_api_key_id,COALESCE(k.name,CASE WHEN a.actor_api_key_id IS NULL THEN 'System' ELSE 'Deleted key' END),a.actor_role,a.action,a.target_type,a.target_id,a.details,a.created_at FROM audit_events a LEFT JOIN api_keys k ON k.id=a.actor_api_key_id AND k.organization_id=a.organization_id`
	if cursor == nil {
		rows, err = s.DB.Query(r.Context(), `SELECT `+columns+` WHERE a.organization_id=$1 ORDER BY a.created_at DESC,a.id DESC LIMIT $2`, tenant(r), limit+1)
	} else {
		rows, err = s.DB.Query(r.Context(), `SELECT `+columns+` WHERE a.organization_id=$1 AND (a.created_at,a.id)<($2,$3) ORDER BY a.created_at DESC,a.id DESC LIMIT $4`, tenant(r), cursor.Time, cursor.ID, limit+1)
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	items := make([]auditView, 0, limit+1)
	for rows.Next() {
		var entry auditView
		if err = rows.Scan(&entry.ID, &entry.ActorAPIKeyID, &entry.ActorKeyName, &entry.ActorRole, &entry.Action, &entry.TargetType, &entry.TargetID, &entry.Details, &entry.CreatedAt); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		items = append(items, entry)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, 200, map[string]any{"data": items, "next_cursor": next})
}
