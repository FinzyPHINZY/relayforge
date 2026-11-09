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

type rotateInput struct {
	OverlapSeconds *int `json:"overlap_seconds"`
}

func (s *Server) rotateEndpointSecret(w http.ResponseWriter, r *http.Request) {
	endpointID := chi.URLParam(r, "id")
	if !id.Valid(endpointID) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	var in rotateInput
	if !decode(w, r, &in, 1024) {
		return
	}
	overlap := 3600
	if in.OverlapSeconds != nil {
		overlap = *in.OverlapSeconds
	}
	if overlap < 300 || overlap > 24*3600 {
		problem(w, 422, "invalid_overlap", "Overlap must be between 300 and 86400 seconds")
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
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM endpoints WHERE id=$1 AND organization_id=$2 FOR UPDATE`, endpointID, orgID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if status == "disabled" {
		problem(w, 409, "endpoint_disabled", "Disabled endpoint cannot rotate secrets")
		return
	}
	newID, err := id.New()
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
	// Retire any older overlap before opening a new one; only one previous
	// secret needs to remain valid for receiver migration.
	_, err = tx.Exec(ctx, `UPDATE endpoint_secrets SET state='retired',retired_at=now(),overlap_until=NULL WHERE endpoint_id=$1 AND organization_id=$2 AND state='overlap'`, endpointID, orgID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var previousID string
	var overlapUntil time.Time
	err = tx.QueryRow(ctx, `UPDATE endpoint_secrets SET state='overlap',overlap_until=now()+($3::integer*interval '1 second') WHERE endpoint_id=$1 AND organization_id=$2 AND state='active' RETURNING id,overlap_until`, endpointID, orgID, overlap).Scan(&previousID, &overlapUntil)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO endpoint_secrets (id,organization_id,endpoint_id,ciphertext,nonce,key_id) VALUES ($1,$2,$3,$4,$5,$6)`, newID, orgID, endpointID, ciphertext, nonce, s.EncryptionKeyID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 201, map[string]any{"signing_secret": secret, "signing_secret_id": newID, "previous_signing_secret_id": previousID, "previous_secret_valid_until": overlapUntil})
}
