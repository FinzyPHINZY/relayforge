package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

type endpointInput struct {
	URL         string   `json:"url"`
	Description string   `json:"description"`
	EventTypes  []string `json:"event_types"`
	TimeoutMS   int      `json:"timeout_ms"`
	MaxAttempts int      `json:"max_attempts"`
}
type endpointPatch struct {
	URL         *string   `json:"url"`
	Description *string   `json:"description"`
	EventTypes  *[]string `json:"event_types"`
	Status      *string   `json:"status"`
	TimeoutMS   *int      `json:"timeout_ms"`
	MaxAttempts *int      `json:"max_attempts"`
}
type endpointView struct {
	ID              string    `json:"id"`
	SigningSecretID string    `json:"signing_secret_id"`
	URL             string    `json:"url"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	EventTypes      []string  `json:"event_types"`
	TimeoutMS       int       `json:"timeout_ms"`
	MaxAttempts     int       `json:"max_attempts"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func validEventTypes(types []string) bool {
	if len(types) == 0 || len(types) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, t := range types {
		if len(t) > 128 || !eventTypePattern.MatchString(t) {
			return false
		}
		if seen[t] {
			return false
		}
		seen[t] = true
	}
	return true
}

func (s *Server) createEndpoint(w http.ResponseWriter, r *http.Request) {
	var in endpointInput
	if !decode(w, r, &in, 16*1024) {
		return
	}
	if len(in.URL) > 2048 || security.ValidateEndpointURL(r.Context(), in.URL) != nil || len(in.Description) > 1000 || !validEventTypes(in.EventTypes) {
		problem(w, 422, "invalid_endpoint", "Invalid endpoint configuration")
		return
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 10000
	}
	if in.MaxAttempts == 0 {
		in.MaxAttempts = 8
	}
	if in.TimeoutMS < 1000 || in.TimeoutMS > 30000 || in.MaxAttempts < 1 || in.MaxAttempts > 8 {
		problem(w, 422, "invalid_endpoint", "Invalid timeout or retry limit")
		return
	}
	ctx := r.Context()
	orgID := tenant(r)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var max int
	err = tx.QueryRow(ctx, `SELECT max_endpoints FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, orgID).Scan(&max)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var count int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM endpoints WHERE organization_id=$1 AND status<>'disabled'`, orgID).Scan(&count)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if count >= max {
		problem(w, 429, "endpoint_quota_exceeded", "Endpoint quota exceeded")
		return
	}
	endpointID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	secretID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	secret, err := security.NewToken("whsec_")
	if err != nil {
		problem(w, 503, "internal_error", "Secret generation failed")
		return
	}
	ciphertext, nonce, err := security.Encrypt(s.EncryptionKey, secret, endpointID)
	if err != nil {
		problem(w, 503, "internal_error", "Secret encryption failed")
		return
	}
	var view endpointView
	err = tx.QueryRow(ctx, `INSERT INTO endpoints (id,organization_id,url,description,timeout_ms,max_attempts) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,url,description,status,timeout_ms,max_attempts,created_at,updated_at`, endpointID, orgID, in.URL, in.Description, in.TimeoutMS, in.MaxAttempts).Scan(&view.ID, &view.URL, &view.Description, &view.Status, &view.TimeoutMS, &view.MaxAttempts, &view.CreatedAt, &view.UpdatedAt)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO endpoint_secrets (id,organization_id,endpoint_id,ciphertext,nonce,key_id) VALUES ($1,$2,$3,$4,$5,$6)`, secretID, orgID, endpointID, ciphertext, nonce, s.EncryptionKeyID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	for _, t := range in.EventTypes {
		if _, err = tx.Exec(ctx, `INSERT INTO subscriptions (organization_id,endpoint_id,event_type) VALUES ($1,$2,$3)`, orgID, endpointID, t); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	view.EventTypes = in.EventTypes
	view.SigningSecretID = secretID
	writeJSON(w, 201, map[string]any{"endpoint": view, "signing_secret": secret, "signing_secret_id": secretID})
}

func (s *Server) getEndpoint(w http.ResponseWriter, r *http.Request) {
	if !id.Valid(chi.URLParam(r, "id")) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	view, err := s.loadEndpoint(r, chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) loadEndpoint(r *http.Request, endpointID string) (endpointView, error) {
	var v endpointView
	err := s.DB.QueryRow(r.Context(), `SELECT e.id,e.url,e.description,e.status,e.timeout_ms,e.max_attempts,e.created_at,e.updated_at,(SELECT s.id FROM endpoint_secrets s WHERE s.endpoint_id=e.id AND s.state='active') FROM endpoints e WHERE e.id=$1 AND e.organization_id=$2`, endpointID, tenant(r)).Scan(&v.ID, &v.URL, &v.Description, &v.Status, &v.TimeoutMS, &v.MaxAttempts, &v.CreatedAt, &v.UpdatedAt, &v.SigningSecretID)
	if err != nil {
		return v, err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT event_type FROM subscriptions WHERE endpoint_id=$1 AND organization_id=$2 ORDER BY event_type`, endpointID, tenant(r))
	if err != nil {
		return v, err
	}
	defer rows.Close()
	v.EventTypes = make([]string, 0)
	for rows.Next() {
		var typ string
		if err = rows.Scan(&typ); err != nil {
			return v, err
		}
		v.EventTypes = append(v.EventTypes, typ)
	}
	return v, rows.Err()
}

func (s *Server) listEndpoints(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := pageParams(r)
	if err != nil {
		problem(w, 400, "invalid_pagination", err.Error())
		return
	}
	var rows pgx.Rows
	if cursor == nil {
		rows, err = s.DB.Query(r.Context(), `SELECT id,created_at FROM endpoints WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, tenant(r), limit+1)
	} else {
		rows, err = s.DB.Query(r.Context(), `SELECT id,created_at FROM endpoints WHERE organization_id=$1 AND (created_at,id)<($2,$3) ORDER BY created_at DESC,id DESC LIMIT $4`, tenant(r), cursor.Time, cursor.ID, limit+1)
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	type key struct {
		id      string
		created time.Time
	}
	ids := []key{}
	for rows.Next() {
		var x string
		var created time.Time
		if err = rows.Scan(&x, &created); err != nil {
			break
		}
		ids = append(ids, key{x, created})
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var next string
	if len(ids) > limit {
		ids = ids[:limit]
		last := ids[len(ids)-1]
		next = encodeCursor(last.created, last.id)
	}
	items := make([]endpointView, 0, len(ids))
	for _, x := range ids {
		v, e := s.loadEndpoint(r, x.id)
		if e != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		items = append(items, v)
	}
	writeJSON(w, 200, map[string]any{"data": items, "next_cursor": next})
}

func (s *Server) patchEndpoint(w http.ResponseWriter, r *http.Request) {
	if !id.Valid(chi.URLParam(r, "id")) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	var in endpointPatch
	if !decode(w, r, &in, 16*1024) {
		return
	}
	if in.URL != nil && (len(*in.URL) > 2048 || security.ValidateEndpointURL(r.Context(), *in.URL) != nil) {
		problem(w, 422, "invalid_endpoint", "Invalid URL")
		return
	}
	if in.Description != nil && len(*in.Description) > 1000 {
		problem(w, 422, "invalid_endpoint", "Description too long")
		return
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "paused" {
		problem(w, 422, "invalid_endpoint", "Invalid status")
		return
	}
	if in.TimeoutMS != nil && (*in.TimeoutMS < 1000 || *in.TimeoutMS > 30000) {
		problem(w, 422, "invalid_endpoint", "Invalid timeout")
		return
	}
	if in.MaxAttempts != nil && (*in.MaxAttempts < 1 || *in.MaxAttempts > 8) {
		problem(w, 422, "invalid_endpoint", "Invalid retry limit")
		return
	}
	if in.EventTypes != nil && !validEventTypes(*in.EventTypes) {
		problem(w, 422, "invalid_endpoint", "Invalid subscriptions")
		return
	}
	ctx := r.Context()
	orgID := tenant(r)
	endpointID := chi.URLParam(r, "id")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM endpoints WHERE id=$1 AND organization_id=$2 FOR UPDATE`, endpointID, orgID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if current == "disabled" {
		problem(w, 409, "endpoint_disabled", "Disabled endpoint cannot be updated")
		return
	}
	_, err = tx.Exec(ctx, `UPDATE endpoints SET url=COALESCE($3,url),description=COALESCE($4,description),status=COALESCE($5,status),timeout_ms=COALESCE($6,timeout_ms),max_attempts=COALESCE($7,max_attempts),updated_at=now() WHERE id=$1 AND organization_id=$2`, endpointID, orgID, in.URL, in.Description, in.Status, in.TimeoutMS, in.MaxAttempts)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if in.EventTypes != nil {
		_, err = tx.Exec(ctx, `DELETE FROM subscriptions WHERE endpoint_id=$1 AND organization_id=$2`, endpointID, orgID)
		if err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		for _, t := range *in.EventTypes {
			_, err = tx.Exec(ctx, `INSERT INTO subscriptions (organization_id,endpoint_id,event_type) VALUES ($1,$2,$3)`, orgID, endpointID, t)
			if err != nil {
				problem(w, 503, "storage_unavailable", "Storage unavailable")
				return
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	s.getEndpoint(w, r)
}

func (s *Server) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	if !id.Valid(chi.URLParam(r, "id")) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	cmd, err := s.DB.Exec(r.Context(), `UPDATE endpoints SET status='disabled',updated_at=now() WHERE id=$1 AND organization_id=$2`, chi.URLParam(r, "id"), tenant(r))
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if cmd.RowsAffected() == 0 {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	w.WriteHeader(204)
}
