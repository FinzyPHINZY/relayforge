package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

// roleRank orders roles for the key role ceiling; unknown roles rank 0.
var roleRank = map[string]int{"owner": 4, "admin": 3, "operator": 2, "viewer": 1}

// normaliseKeyName trims name and reports whether it holds 1..80 characters.
func normaliseKeyName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= 80
}

type keyCreator struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"display_name"`
}

type keyView struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Prefix     string      `json:"prefix"`
	Role       string      `json:"role"`
	Scopes     []string    `json:"scopes"`
	CreatedBy  *keyCreator `json:"created_by"`
	CreatedAt  time.Time   `json:"created_at"`
	LastUsedAt *time.Time  `json:"last_used_at"`
	ExpiresAt  *time.Time  `json:"expires_at"`
	RevokedAt  *time.Time  `json:"revoked_at"`
}

// Keys without a stored role (bootstrap, pre-accounts) act as owner unless
// tied to a member, whose live role applies.
const keyColumns = `k.id,k.name,k.key_prefix,COALESCE(m.role,k.role,'owner'),k.scopes,u.id,u.display_name,k.created_at,k.last_used_at,k.expires_at,k.revoked_at
	FROM api_keys k LEFT JOIN team_members m ON m.id=k.member_id AND m.organization_id=k.organization_id LEFT JOIN users u ON u.id=k.created_by_user_id`

func scanKey(row pgx.Row) (keyView, error) {
	var k keyView
	var creatorID, creatorName *string
	err := row.Scan(&k.ID, &k.Name, &k.Prefix, &k.Role, &k.Scopes, &creatorID, &creatorName, &k.CreatedAt, &k.LastUsedAt, &k.ExpiresAt, &k.RevokedAt)
	if creatorID != nil {
		k.CreatedBy = &keyCreator{ID: *creatorID, DisplayName: creatorName}
	}
	return k, err
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT `+keyColumns+` WHERE k.organization_id=$1 ORDER BY k.created_at DESC,k.id DESC LIMIT 200`, tenant(r))
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	keys := make([]keyView, 0)
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		keys = append(keys, k)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"data": keys})
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	var in struct {
		Name          string   `json:"name"`
		Role          string   `json:"role"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays *int     `json:"expires_in_days"`
	}
	if !decode(w, r, &in, 4096) {
		return
	}
	name, ok := normaliseKeyName(in.Name)
	if !ok || roleRank[in.Role] == 0 || (in.ExpiresInDays != nil && (*in.ExpiresInDays < 1 || *in.ExpiresInDays > 365)) {
		problem(w, 422, "invalid_key", "Invalid name, role, or expiry")
		return
	}
	if roleRank[in.Role] > roleRank[p.Role] {
		problem(w, 403, "scope_exceeds_role", "Key role exceeds your role")
		return
	}
	scopes := make([]string, 0, len(in.Scopes))
	for _, scope := range in.Scopes {
		if !slices.Contains(allScopes, scope) || !roleAllows(in.Role, scope) {
			problem(w, 422, "invalid_scopes", "Scopes are unknown or exceed the key role")
			return
		}
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		problem(w, 422, "invalid_scopes", "At least one scope is required")
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
	var createdBy *string
	if p.UserID != "" {
		createdBy = &p.UserID
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO api_keys (id,organization_id,name,key_prefix,key_hash,scopes,role,created_by_user_id,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $9::int IS NULL THEN NULL ELSE now()+make_interval(days => $9::int) END)`,
		keyID, tenant(r), name, prefix, security.Hash(raw), scopes, in.Role, createdBy, in.ExpiresInDays)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	view, err := scanKey(tx.QueryRow(ctx, `SELECT `+keyColumns+` WHERE k.id=$1`, keyID))
	if err == nil {
		err = writeAudit(r, tx, "api_key.issued", "api_key", keyID, map[string]any{"role": in.Role, "scopes": scopes})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 201, struct {
		Key string `json:"key"`
		keyView
	}{raw, view})
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "id")
	if !id.Valid(keyID) {
		problem(w, 404, "not_found", "API key not found")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	// Scan the stored id: the path id may differ in hex case.
	var storedID string
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,revoked_at FROM api_keys WHERE id=$1 AND organization_id=$2 FOR UPDATE`, keyID, tenant(r)).Scan(&storedID, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "API key not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if storedID == r.Context().Value(principalKey).(principal).KeyID {
		problem(w, 409, "cannot_revoke_self", "A key cannot revoke itself")
		return
	}
	if revokedAt != nil {
		w.WriteHeader(204)
		return
	}
	_, err = tx.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND organization_id=$2`, storedID, tenant(r))
	if err == nil {
		err = writeAudit(r, tx, "api_key.revoked", "api_key", storedID, nil)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	w.WriteHeader(204)
}
