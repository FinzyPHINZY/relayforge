package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"webhook_delivery/internal/database/sqlc"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/observability"
)

var eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

type eventInput struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	DeliverAt *time.Time      `json:"deliver_at,omitempty"`
}
type eventResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var input eventInput
	if !decode(w, r, &input, 270*1024) {
		return
	}
	if len(input.Type) > 128 || !eventTypePattern.MatchString(input.Type) {
		problem(w, 422, "invalid_event", "Invalid event type")
		return
	}
	var data any
	dec := json.NewDecoder(bytes.NewReader(input.Data))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil || data == nil {
		problem(w, 422, "invalid_event", "Data must be JSON")
		return
	}
	canonical, err := json.Marshal(data)
	if err != nil {
		problem(w, 422, "invalid_event", "Invalid data")
		return
	}
	if len(canonical) > 256*1024 {
		problem(w, 413, "payload_too_large", "Event data exceeds 256 KiB")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 255 {
		problem(w, 422, "invalid_idempotency_key", "Idempotency key is too long")
		return
	}
	orgID := tenant(r)
	ctx, span := otel.Tracer("relayforge/api").Start(r.Context(), "event.accept")
	defer span.End()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	fp := fingerprint(input.Type, canonical)
	if input.DeliverAt != nil {
		fp = fingerprint(input.Type+"\x00"+input.DeliverAt.UTC().Format(time.RFC3339Nano), canonical)
	}
	if key != "" {
		// The advisory lock serializes matching keys before either request changes usage.
		_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orgID+":"+key)
		if err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		var oldID, oldType string
		var oldFP []byte
		var oldCreated time.Time
		err = tx.QueryRow(ctx, `SELECT i.event_id, i.request_fingerprint, e.event_type, e.created_at FROM idempotency_records i JOIN events e ON e.id=i.event_id AND e.organization_id=i.organization_id WHERE i.organization_id=$1 AND i.idempotency_key=$2`, orgID, key).Scan(&oldID, &oldFP, &oldType, &oldCreated)
		if err == nil {
			if !bytes.Equal(oldFP, fp) {
				problem(w, 409, "idempotency_conflict", "Key already used for another event")
				return
			}
			writeJSON(w, 202, eventResponse{ID: oldID, Type: oldType, Status: "accepted", CreatedAt: oldCreated})
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
	}
	if input.DeliverAt != nil {
		now := time.Now().UTC()
		if !input.DeliverAt.After(now) || input.DeliverAt.After(now.Add(30*24*time.Hour)) {
			problem(w, 422, "invalid_schedule", "deliver_at must be in the next 30 days")
			return
		}
	}
	var quota int
	err = tx.QueryRow(ctx, `SELECT monthly_event_quota FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, orgID).Scan(&quota)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var used int
	err = tx.QueryRow(ctx, `SELECT event_count FROM monthly_event_usage WHERE organization_id=$1 AND month=date_trunc('month', now())::date`, orgID).Scan(&used)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if used >= quota {
		problem(w, 429, "quota_exceeded", "Monthly event quota exceeded")
		return
	}
	eventID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	outboxID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	payloadHash := sha256.Sum256(canonical)
	var created time.Time
	err = tx.QueryRow(ctx, `INSERT INTO events (id,organization_id,event_type,payload,idempotency_key,payload_sha256,traceparent) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`, eventID, orgID, input.Type, canonical, nullable(key), payloadHash[:], nullable(observability.TraceParent(ctx))).Scan(&created)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	availableAt := time.Now().UTC()
	if input.DeliverAt != nil {
		availableAt = input.DeliverAt.UTC()
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (id,organization_id,event_id,available_at) VALUES ($1,$2,$3,$4)`, outboxID, orgID, eventID, availableAt)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if key != "" {
		_, err = tx.Exec(ctx, `INSERT INTO idempotency_records (organization_id,idempotency_key,request_fingerprint,event_id) VALUES ($1,$2,$3,$4)`, orgID, key, fp, eventID)
		if err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO monthly_event_usage (organization_id,month,event_count) VALUES ($1,date_trunc('month',now())::date,1) ON CONFLICT (organization_id,month) DO UPDATE SET event_count=monthly_event_usage.event_count+1`, orgID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 202, eventResponse{ID: eventID, Type: input.Type, Status: "accepted", CreatedAt: created})
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	if !id.Valid(chi.URLParam(r, "id")) {
		problem(w, 404, "not_found", "Event not found")
		return
	}
	event, err := sqlc.New(s.DB).GetEvent(r.Context(), sqlc.GetEventParams{ID: chi.URLParam(r, "id"), OrganizationID: tenant(r)})
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Event not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"id": event.ID, "type": event.EventType, "data": json.RawMessage(event.Payload), "created_at": event.CreatedAt.Time, "status": "accepted"})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := pageParams(r)
	if err != nil {
		problem(w, 400, "invalid_pagination", err.Error())
		return
	}
	var rows pgx.Rows
	if cursor == nil {
		rows, err = s.DB.Query(r.Context(), `SELECT id,event_type,created_at FROM events WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, tenant(r), limit+1)
	} else {
		rows, err = s.DB.Query(r.Context(), `SELECT id,event_type,created_at FROM events WHERE organization_id=$1 AND (created_at,id)<($2,$3) ORDER BY created_at DESC,id DESC LIMIT $4`, tenant(r), cursor.Time, cursor.ID, limit+1)
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	items := make([]eventResponse, 0)
	for rows.Next() {
		var e eventResponse
		if err = rows.Scan(&e.ID, &e.Type, &e.CreatedAt); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		e.Status = "accepted"
		items = append(items, e)
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
