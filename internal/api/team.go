package api

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

func roleAllows(role, scope string) bool {
	switch role {
	case "owner":
		return true
	case "admin":
		return scope != "team:write"
	case "operator":
		return scope != "team:write"
	case "viewer":
		return scope == "team:read" || scope == "events:read" || scope == "endpoints:read" || scope == "deliveries:read" || scope == "metrics:read"
	default:
		return false
	}
}

type teamMember struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT id,name,email,role,status,created_at FROM team_members WHERE organization_id=$1 ORDER BY created_at,id LIMIT 500`, tenant(r))
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	members := make([]teamMember, 0)
	for rows.Next() {
		var m teamMember
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Role, &m.Status, &m.CreatedAt); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		members = append(members, m)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"data": members})
}

func ownerOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Context().Value(principalKey).(principal).Role != "owner" {
		problem(w, 403, "forbidden", "Owner role required")
		return false
	}
	return true
}

func validTeamRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}

func (s *Server) createTeamMember(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &in, 2048) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	address, parseErr := mail.ParseAddress(in.Email)
	if len(in.Name) < 1 || len(in.Name) > 120 || len(in.Email) > 254 || parseErr != nil || address.Address != in.Email || !validTeamRole(in.Role) {
		problem(w, 422, "invalid_member", "Invalid name, email, or role")
		return
	}
	memberID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	keyID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	raw, err := security.NewToken("whk_")
	if err != nil {
		problem(w, 503, "internal_error", "Key generation failed")
		return
	}
	prefix, err := security.KeyPrefix(raw)
	if err != nil {
		problem(w, 503, "internal_error", "Key generation failed")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var lock int
	if err = tx.QueryRow(ctx, `SELECT 1 FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, tenant(r)).Scan(&lock); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM team_members WHERE organization_id=$1`, tenant(r)).Scan(&count); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if count >= 100 {
		problem(w, 429, "team_limit", "Team member limit reached")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO team_members (id,organization_id,name,email,role) VALUES ($1,$2,$3,$4,$5)`, memberID, tenant(r), in.Name, in.Email, in.Role)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		problem(w, 409, "member_exists", "Team member already exists")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	// The member role is checked on every request, so a later role change takes
	// effect immediately without issuing a new credential.
	scopes := []string{"events:read", "events:write", "endpoints:read", "endpoints:write", "deliveries:read", "deliveries:write", "metrics:read", "team:read", "team:write"}
	_, err = tx.Exec(ctx, `INSERT INTO api_keys (id,organization_id,member_id,name,key_prefix,key_hash,scopes) VALUES ($1,$2,$3,$4,$5,$6,$7)`, keyID, tenant(r), memberID, in.Name, prefix, security.Hash(raw), scopes)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 201, map[string]any{"id": memberID, "role": in.Role, "api_key": raw})
}

func (s *Server) patchTeamMember(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	memberID := chi.URLParam(r, "id")
	if !id.Valid(memberID) {
		problem(w, 404, "not_found", "Member not found")
		return
	}
	var in struct {
		Role   *string `json:"role"`
		Status *string `json:"status"`
	}
	if !decode(w, r, &in, 1024) {
		return
	}
	if (in.Role == nil && in.Status == nil) || (in.Role != nil && !validTeamRole(*in.Role)) || (in.Status != nil && *in.Status != "active" && *in.Status != "suspended") {
		problem(w, 422, "invalid_member", "Invalid role or status")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE team_members SET role=COALESCE($3,role),status=COALESCE($4,status),updated_at=now() WHERE id=$1 AND organization_id=$2`, memberID, tenant(r), in.Role, in.Status)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "not_found", "Member not found")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}
