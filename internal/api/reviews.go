package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
)

type reviewInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (s *Server) reviewDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := chi.URLParam(r, "id")
	if !id.Valid(deliveryID) {
		problem(w, 404, "not_found", "Delivery not found")
		return
	}
	var in reviewInput
	if !decode(w, r, &in, 2048) {
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if (in.Status != "investigating" && in.Status != "resolved") || len(in.Note) > 500 || (in.Status == "resolved" && in.Note == "") {
		problem(w, 422, "invalid_review", "Choose investigating or resolved; resolution requires a note of at most 500 bytes")
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
	var deliveryStatus string
	err = tx.QueryRow(ctx, `SELECT status FROM deliveries WHERE id=$1 AND organization_id=$2 FOR UPDATE`, deliveryID, orgID).Scan(&deliveryStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Delivery not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if deliveryStatus != "exhausted" {
		problem(w, 409, "not_reviewable", "Only exhausted deliveries can be reviewed")
		return
	}
	var currentStatus, currentNote string
	err = tx.QueryRow(ctx, `SELECT status,note FROM delivery_reviews WHERE delivery_id=$1 AND organization_id=$2`, deliveryID, orgID).Scan(&currentStatus, &currentNote)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err == nil && currentStatus == in.Status && currentNote == in.Note {
		writeJSON(w, 200, map[string]string{"status": in.Status, "note": in.Note})
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO delivery_reviews (delivery_id,organization_id,status,note) VALUES ($1,$2,$3,$4) ON CONFLICT (delivery_id) DO UPDATE SET status=EXCLUDED.status,note=EXCLUDED.note,updated_at=now()`, deliveryID, orgID, in.Status, in.Note)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	actionID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	keyID := r.Context().Value(principalKey).(principal).KeyID
	_, err = tx.Exec(ctx, `INSERT INTO delivery_review_history (id,delivery_id,organization_id,api_key_id,status,note) VALUES ($1,$2,$3,$4,$5,$6)`, actionID, deliveryID, orgID, keyID, in.Status, in.Note)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Status, "note": in.Note})
}
