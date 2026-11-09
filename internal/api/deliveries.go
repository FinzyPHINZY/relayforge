package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"webhook_delivery/internal/id"
)

type deliveryView struct {
	ID                     string     `json:"id"`
	EventID                string     `json:"event_id"`
	EndpointID             string     `json:"endpoint_id"`
	RunNumber              int        `json:"run_number"`
	ReplayedFromDeliveryID *string    `json:"replayed_from_delivery_id,omitempty"`
	DestinationURL         string     `json:"destination_url"`
	Status                 string     `json:"status"`
	AttemptCount           int        `json:"attempt_count"`
	MaxAttempts            int        `json:"max_attempts"`
	NextAttemptAt          time.Time  `json:"next_attempt_at"`
	LastHTTPStatus         *int       `json:"last_http_status,omitempty"`
	LastErrorCode          *string    `json:"last_error_code,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
}

const deliveryColumns = `id,event_id,endpoint_id,run_number,replayed_from_delivery_id,destination_url,status,attempt_count,max_attempts,next_attempt_at,last_http_status,last_error_code,created_at,updated_at,completed_at`

func scanDelivery(row pgx.Row) (deliveryView, error) {
	var d deliveryView
	err := row.Scan(&d.ID, &d.EventID, &d.EndpointID, &d.RunNumber, &d.ReplayedFromDeliveryID, &d.DestinationURL, &d.Status, &d.AttemptCount, &d.MaxAttempts, &d.NextAttemptAt, &d.LastHTTPStatus, &d.LastErrorCode, &d.CreatedAt, &d.UpdatedAt, &d.CompletedAt)
	return d, err
}

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := pageParams(r)
	if err != nil {
		problem(w, 400, "invalid_pagination", err.Error())
		return
	}
	q := r.URL.Query()
	var eventID, endpointID, status any
	if raw := q.Get("event_id"); raw != "" {
		if !id.Valid(raw) {
			problem(w, 400, "invalid_filter", "Invalid event ID")
			return
		}
		eventID = raw
	}
	if raw := q.Get("endpoint_id"); raw != "" {
		if !id.Valid(raw) {
			problem(w, 400, "invalid_filter", "Invalid endpoint ID")
			return
		}
		endpointID = raw
	}
	if raw := q.Get("status"); raw != "" {
		switch raw {
		case "pending", "processing", "succeeded", "retry_wait", "exhausted", "cancelled":
			status = raw
		default:
			problem(w, 400, "invalid_filter", "Invalid delivery status")
			return
		}
	}
	var after, before any
	if raw := q.Get("created_after"); raw != "" {
		v, e := time.Parse(time.RFC3339, raw)
		if e != nil {
			problem(w, 400, "invalid_filter", "Invalid created_after")
			return
		}
		after = v
	}
	if raw := q.Get("created_before"); raw != "" {
		v, e := time.Parse(time.RFC3339, raw)
		if e != nil {
			problem(w, 400, "invalid_filter", "Invalid created_before")
			return
		}
		before = v
	}
	var cursorTime, cursorID any
	if cursor != nil {
		if !id.Valid(cursor.ID) {
			problem(w, 400, "invalid_pagination", "Invalid cursor")
			return
		}
		cursorTime = cursor.Time
		cursorID = cursor.ID
	}
	rows, err := s.DB.Query(r.Context(), `SELECT `+deliveryColumns+` FROM deliveries WHERE organization_id=$1 AND ($2::uuid IS NULL OR event_id=$2) AND ($3::uuid IS NULL OR endpoint_id=$3) AND ($4::text IS NULL OR status=$4) AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at < $6) AND ($7::timestamptz IS NULL OR (created_at,id)<($7,$8::uuid)) ORDER BY created_at DESC,id DESC LIMIT $9`, tenant(r), eventID, endpointID, status, after, before, cursorTime, cursorID, limit+1)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	items := make([]deliveryView, 0)
	for rows.Next() {
		d, e := scanDelivery(rows)
		if e != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		items = append(items, d)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var next string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, 200, map[string]any{"data": items, "next_cursor": next})
}

type attemptView struct {
	ID              string     `json:"id"`
	AttemptNumber   int        `json:"attempt_number"`
	SigningSecretID *string    `json:"signing_secret_id,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	DurationMS      *int       `json:"duration_ms,omitempty"`
	HTTPStatus      *int       `json:"http_status,omitempty"`
	Outcome         string     `json:"outcome"`
	ErrorCode       *string    `json:"error_code,omitempty"`
	ResponseExcerpt *string    `json:"response_excerpt,omitempty"`
}

func (s *Server) getDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := chi.URLParam(r, "id")
	if !id.Valid(deliveryID) {
		problem(w, 404, "not_found", "Delivery not found")
		return
	}
	d, err := scanDelivery(s.DB.QueryRow(r.Context(), `SELECT `+deliveryColumns+` FROM deliveries WHERE id=$1 AND organization_id=$2`, deliveryID, tenant(r)))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Delivery not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,attempt_number,signing_secret_id,started_at,finished_at,duration_ms,http_status,outcome,error_code,response_excerpt FROM delivery_attempts WHERE delivery_id=$1 AND organization_id=$2 ORDER BY attempt_number`, deliveryID, tenant(r))
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	attempts := make([]attemptView, 0)
	for rows.Next() {
		var a attemptView
		if err = rows.Scan(&a.ID, &a.AttemptNumber, &a.SigningSecretID, &a.StartedAt, &a.FinishedAt, &a.DurationMS, &a.HTTPStatus, &a.Outcome, &a.ErrorCode, &a.ResponseExcerpt); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		attempts = append(attempts, a)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"delivery": d, "attempts": attempts})
}

func (s *Server) replayDelivery(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "id")
	if !id.Valid(sourceID) {
		problem(w, 404, "not_found", "Delivery not found")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 255 {
		problem(w, 422, "invalid_idempotency_key", "Idempotency-Key is required and must be at most 255 characters")
		return
	}
	ctx, span := otel.Tracer("relayforge/api").Start(r.Context(), "delivery.replay")
	defer span.End()
	orgID := tenant(r)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var eventID, endpointID string
	err = tx.QueryRow(ctx, `SELECT event_id,endpoint_id FROM deliveries WHERE id=$1 AND organization_id=$2`, sourceID, orgID).Scan(&eventID, &endpointID)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Delivery not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, orgID+":"+eventID+":"+endpointID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var oldID, oldStatus string
	var oldRun int
	err = tx.QueryRow(ctx, `SELECT d.id,d.run_number,d.status FROM replay_requests rr JOIN deliveries d ON d.id=rr.replay_delivery_id AND d.organization_id=rr.organization_id WHERE rr.organization_id=$1 AND rr.source_delivery_id=$2 AND rr.idempotency_key=$3`, orgID, sourceID, key).Scan(&oldID, &oldRun, &oldStatus)
	if err == nil {
		writeJSON(w, 202, map[string]any{"id": oldID, "replayed_from_delivery_id": sourceID, "run_number": oldRun, "status": oldStatus})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var sourceStatus, endpointStatus string
	err = tx.QueryRow(ctx, `SELECT d.status,e.status FROM deliveries d JOIN endpoints e ON e.id=d.endpoint_id AND e.organization_id=d.organization_id WHERE d.id=$1 AND d.organization_id=$2 FOR UPDATE OF d,e`, sourceID, orgID).Scan(&sourceStatus, &endpointStatus)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if sourceStatus != "exhausted" && sourceStatus != "cancelled" {
		problem(w, 409, "not_replayable", "Only terminal failed deliveries can be replayed")
		return
	}
	if endpointStatus != "active" {
		problem(w, 409, "endpoint_inactive", "Endpoint must be active to replay")
		return
	}
	var runNumber int
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(run_number),0)+1 FROM deliveries WHERE organization_id=$1 AND event_id=$2 AND endpoint_id=$3`, orgID, eventID, endpointID).Scan(&runNumber)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	newID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO deliveries (id,organization_id,event_id,endpoint_id,run_number,replayed_from_delivery_id,destination_url,timeout_ms,max_attempts) SELECT $1,$2,$3,e.id,$4,$5,e.url,e.timeout_ms,e.max_attempts FROM endpoints e WHERE e.id=$6 AND e.organization_id=$2 AND e.status='active'`, newID, orgID, eventID, runNumber, sourceID, endpointID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO replay_requests (organization_id,source_delivery_id,idempotency_key,replay_delivery_id) VALUES ($1,$2,$3,$4)`, orgID, sourceID, key, newID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 202, map[string]any{"id": newID, "replayed_from_delivery_id": sourceID, "run_number": runNumber, "status": "pending"})
}
