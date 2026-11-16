package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

type endpointInput struct {
	URL                  string            `json:"url"`
	Description          string            `json:"description"`
	EventTypes           []string          `json:"event_types"`
	TimeoutMS            int               `json:"timeout_ms"`
	MaxAttempts          int               `json:"max_attempts"`
	ConcurrencyLimit     *int              `json:"concurrency_limit"`
	RateLimitPerMinute   int               `json:"rate_limit_per_minute"`
	VerificationRequired bool              `json:"verification_required"`
	FieldMap             map[string]string `json:"field_map"`
}
type endpointPatch struct {
	URL                *string            `json:"url"`
	Description        *string            `json:"description"`
	EventTypes         *[]string          `json:"event_types"`
	Status             *string            `json:"status"`
	TimeoutMS          *int               `json:"timeout_ms"`
	MaxAttempts        *int               `json:"max_attempts"`
	ConcurrencyLimit   *int               `json:"concurrency_limit"`
	RateLimitPerMinute *int               `json:"rate_limit_per_minute"`
	FieldMap           *map[string]string `json:"field_map"`
}
type endpointView struct {
	ID                   string            `json:"id"`
	SigningSecretID      string            `json:"signing_secret_id"`
	SigningSecrets       []secretMetadata  `json:"signing_secrets"`
	URL                  string            `json:"url"`
	Description          string            `json:"description"`
	Status               string            `json:"status"`
	EventTypes           []string          `json:"event_types"`
	TimeoutMS            int               `json:"timeout_ms"`
	MaxAttempts          int               `json:"max_attempts"`
	ConcurrencyLimit     int               `json:"concurrency_limit"`
	RateLimitPerMinute   int               `json:"rate_limit_per_minute"`
	NextRateAt           *time.Time        `json:"next_rate_at,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	CircuitStatus        string            `json:"circuit_status"`
	CircuitOpenUntil     *time.Time        `json:"circuit_open_until,omitempty"`
	VerificationRequired bool              `json:"verification_required"`
	VerifiedAt           *time.Time        `json:"verified_at,omitempty"`
	FieldMap             map[string]string `json:"field_map"`
}

type secretMetadata struct {
	ID           string     `json:"id"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"created_at"`
	OverlapUntil *time.Time `json:"overlap_until,omitempty"`
	RetiredAt    *time.Time `json:"retired_at,omitempty"`
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
	if len(in.URL) > 2048 || security.ValidateEndpointURLWithDemo(r.Context(), in.URL, s.DemoReceiverOrigin) != nil {
		problem(w, 422, "invalid_endpoint", "Use an HTTPS endpoint on port 443 that resolves only to public IP addresses")
		return
	}
	if len(in.Description) > 1000 || !validEventTypes(in.EventTypes) {
		problem(w, 422, "invalid_endpoint", "Invalid endpoint configuration")
		return
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 10000
	}
	if in.MaxAttempts == 0 {
		in.MaxAttempts = 8
	}
	concurrencyLimit := 2
	if in.ConcurrencyLimit != nil {
		concurrencyLimit = *in.ConcurrencyLimit
	}
	if in.TimeoutMS < 1000 || in.TimeoutMS > 30000 || in.MaxAttempts < 1 || in.MaxAttempts > 8 || concurrencyLimit < 1 || concurrencyLimit > 100 || in.RateLimitPerMinute < 0 || in.RateLimitPerMinute > 6000 {
		problem(w, 422, "invalid_endpoint", "Invalid timeout, retry limit, or delivery limit")
		return
	}
	if err := delivery.ValidateFieldMap(in.FieldMap); err != nil {
		problem(w, 422, "invalid_field_map", err.Error())
		return
	}
	if in.FieldMap == nil {
		in.FieldMap = map[string]string{}
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
	err = tx.QueryRow(ctx, `INSERT INTO endpoints (id,organization_id,url,description,timeout_ms,max_attempts,concurrency_limit,rate_limit_per_minute,verification_required,field_map) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id,url,description,status,timeout_ms,max_attempts,concurrency_limit,rate_limit_per_minute,created_at,updated_at`, endpointID, orgID, in.URL, in.Description, in.TimeoutMS, in.MaxAttempts, concurrencyLimit, in.RateLimitPerMinute, in.VerificationRequired, in.FieldMap).Scan(&view.ID, &view.URL, &view.Description, &view.Status, &view.TimeoutMS, &view.MaxAttempts, &view.ConcurrencyLimit, &view.RateLimitPerMinute, &view.CreatedAt, &view.UpdatedAt)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var secretCreated time.Time
	err = tx.QueryRow(ctx, `INSERT INTO endpoint_secrets (id,organization_id,endpoint_id,ciphertext,nonce,key_id) VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at`, secretID, orgID, endpointID, ciphertext, nonce, s.EncryptionKeyID).Scan(&secretCreated)
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
	if err = writeAudit(r, tx, "endpoint.created", "endpoint", endpointID, map[string]any{"event_type_count": len(in.EventTypes), "verification_required": in.VerificationRequired, "concurrency_limit": concurrencyLimit, "rate_limit_per_minute": in.RateLimitPerMinute}); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	view.EventTypes = in.EventTypes
	view.CircuitStatus = "closed"
	view.VerificationRequired = in.VerificationRequired
	view.FieldMap = in.FieldMap
	view.SigningSecretID = secretID
	view.SigningSecrets = []secretMetadata{{ID: secretID, State: "active", CreatedAt: secretCreated}}
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
	err := s.DB.QueryRow(r.Context(), `SELECT e.id,e.url,e.description,e.status,e.timeout_ms,e.max_attempts,e.concurrency_limit,e.rate_limit_per_minute,ec.next_rate_at,e.created_at,e.updated_at,(SELECT s.id FROM endpoint_secrets s WHERE s.endpoint_id=e.id AND s.state='active'),CASE WHEN ec.open_until IS NULL THEN 'closed' WHEN ec.probe_delivery_id IS NOT NULL OR ec.open_until<=now() THEN 'half_open' ELSE 'open' END,ec.open_until,e.verification_required,e.verified_at,e.field_map FROM endpoints e JOIN endpoint_circuits ec ON ec.endpoint_id=e.id AND ec.organization_id=e.organization_id WHERE e.id=$1 AND e.organization_id=$2`, endpointID, tenant(r)).Scan(&v.ID, &v.URL, &v.Description, &v.Status, &v.TimeoutMS, &v.MaxAttempts, &v.ConcurrencyLimit, &v.RateLimitPerMinute, &v.NextRateAt, &v.CreatedAt, &v.UpdatedAt, &v.SigningSecretID, &v.CircuitStatus, &v.CircuitOpenUntil, &v.VerificationRequired, &v.VerifiedAt, &v.FieldMap)
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
	if err = rows.Err(); err != nil {
		return v, err
	}
	rows.Close()
	secrets, err := s.DB.Query(r.Context(), `SELECT id,state,created_at,overlap_until,retired_at FROM endpoint_secrets WHERE endpoint_id=$1 AND organization_id=$2 ORDER BY created_at DESC,id DESC`, endpointID, tenant(r))
	if err != nil {
		return v, err
	}
	defer secrets.Close()
	v.SigningSecrets = make([]secretMetadata, 0)
	for secrets.Next() {
		var m secretMetadata
		if err = secrets.Scan(&m.ID, &m.State, &m.CreatedAt, &m.OverlapUntil, &m.RetiredAt); err != nil {
			return v, err
		}
		v.SigningSecrets = append(v.SigningSecrets, m)
	}
	return v, secrets.Err()
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
	if in.URL == nil && in.Description == nil && in.EventTypes == nil && in.Status == nil && in.TimeoutMS == nil && in.MaxAttempts == nil && in.ConcurrencyLimit == nil && in.RateLimitPerMinute == nil && in.FieldMap == nil {
		problem(w, 422, "invalid_endpoint", "No endpoint changes provided")
		return
	}
	if in.URL != nil && (len(*in.URL) > 2048 || security.ValidateEndpointURLWithDemo(r.Context(), *in.URL, s.DemoReceiverOrigin) != nil) {
		problem(w, 422, "invalid_endpoint", "Use an HTTPS endpoint on port 443 that resolves only to public IP addresses")
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
	if in.ConcurrencyLimit != nil && (*in.ConcurrencyLimit < 1 || *in.ConcurrencyLimit > 100) {
		problem(w, 422, "invalid_endpoint", "Invalid concurrency limit")
		return
	}
	if in.RateLimitPerMinute != nil && (*in.RateLimitPerMinute < 0 || *in.RateLimitPerMinute > 6000) {
		problem(w, 422, "invalid_endpoint", "Invalid delivery rate limit")
		return
	}
	if in.EventTypes != nil && !validEventTypes(*in.EventTypes) {
		problem(w, 422, "invalid_endpoint", "Invalid subscriptions")
		return
	}
	if in.FieldMap != nil {
		if err := delivery.ValidateFieldMap(*in.FieldMap); err != nil {
			problem(w, 422, "invalid_field_map", err.Error())
			return
		}
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
	_, err = tx.Exec(ctx, `UPDATE endpoints SET url=COALESCE($3,url),description=COALESCE($4,description),status=COALESCE($5,status),timeout_ms=COALESCE($6,timeout_ms),max_attempts=COALESCE($7,max_attempts),verified_at=CASE WHEN $3::text IS NOT NULL AND $3::text IS DISTINCT FROM url THEN NULL ELSE verified_at END,field_map=COALESCE($8,field_map),concurrency_limit=COALESCE($9,concurrency_limit),rate_limit_per_minute=COALESCE($10,rate_limit_per_minute),updated_at=now() WHERE id=$1 AND organization_id=$2`, endpointID, orgID, in.URL, in.Description, in.Status, in.TimeoutMS, in.MaxAttempts, in.FieldMap, in.ConcurrencyLimit, in.RateLimitPerMinute)
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
	if err = writeAudit(r, tx, "endpoint.updated", "endpoint", endpointID, endpointAuditDetails(in)); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
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
	ctx := r.Context()
	endpointID := chi.URLParam(r, "id")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM endpoints WHERE id=$1 AND organization_id=$2 FOR UPDATE`, endpointID, tenant(r)).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if status == "disabled" {
		w.WriteHeader(204)
		return
	}
	_, err = tx.Exec(ctx, `UPDATE endpoints SET status='disabled',updated_at=now() WHERE id=$1 AND organization_id=$2`, endpointID, tenant(r))
	if err == nil {
		err = writeAudit(r, tx, "endpoint.disabled", "endpoint", endpointID, nil)
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

func endpointAuditDetails(in endpointPatch) map[string]any {
	fields := make([]string, 0, 9)
	for _, item := range []struct {
		name string
		set  bool
	}{{"url", in.URL != nil}, {"description", in.Description != nil}, {"event_types", in.EventTypes != nil}, {"status", in.Status != nil}, {"timeout_ms", in.TimeoutMS != nil}, {"max_attempts", in.MaxAttempts != nil}, {"concurrency_limit", in.ConcurrencyLimit != nil}, {"rate_limit_per_minute", in.RateLimitPerMinute != nil}, {"field_map", in.FieldMap != nil}} {
		if item.set {
			fields = append(fields, item.name)
		}
	}
	details := map[string]any{"fields": fields}
	if in.Status != nil {
		details["status"] = *in.Status
	}
	if in.ConcurrencyLimit != nil {
		details["concurrency_limit"] = *in.ConcurrencyLimit
	}
	if in.RateLimitPerMinute != nil {
		details["rate_limit_per_minute"] = *in.RateLimitPerMinute
	}
	return details
}
