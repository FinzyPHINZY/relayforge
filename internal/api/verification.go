package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

// verifyEndpoint proves control of the destination. The receiver returns the
// challenge in its response header to a bounded GET request.
func (s *Server) verifyEndpoint(w http.ResponseWriter, r *http.Request) {
	endpointID := chi.URLParam(r, "id")
	if !id.Valid(endpointID) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	var destination, status string
	var required bool
	err := s.DB.QueryRow(r.Context(), `SELECT url,status,verification_required FROM endpoints WHERE id=$1 AND organization_id=$2`, endpointID, tenant(r)).Scan(&destination, &status, &required)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Endpoint not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if !required || status == "disabled" {
		problem(w, 409, "verification_unavailable", "Endpoint is not awaiting verification")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	client, err := security.PinnedClientWithDemo(ctx, destination, 5*time.Second, s.AllowHTTPTest, s.DemoReceiverOrigin)
	if err != nil {
		problem(w, 422, "destination_blocked", "Destination cannot be verified")
		return
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		problem(w, 503, "internal_error", "Challenge generation failed")
		return
	}
	challenge := base64.RawURLEncoding.EncodeToString(random[:])
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, destination, nil)
	if err != nil {
		problem(w, 422, "invalid_endpoint", "Invalid destination")
		return
	}
	request.Header.Set("Webhook-Verification-Challenge", challenge)
	response, err := client.Do(request)
	if err != nil {
		problem(w, 422, "verification_failed", "Receiver did not answer the challenge")
		return
	}
	defer response.Body.Close()
	answer := response.Header.Get("Webhook-Verification-Response")
	if response.StatusCode != http.StatusOK || subtle.ConstantTimeCompare([]byte(answer), []byte(challenge)) != 1 {
		problem(w, 422, "verification_failed", "Receiver did not echo the challenge")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE endpoints SET verified_at=now(),updated_at=now() WHERE id=$1 AND organization_id=$2 AND url=$3 AND status<>'disabled' AND verification_required`, endpointID, tenant(r), destination)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		problem(w, 409, "endpoint_changed", "Endpoint changed during verification; try again")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "verified"})
}
