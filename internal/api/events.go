package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/observability"
)

var eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
var eventTypeSearchPattern = regexp.MustCompile(`^[a-z0-9_.]{1,128}$`)

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
	if len(input.Type) > 128 || !eventTypePattern.MatchString(input.Type) || strings.HasPrefix(input.Type, "relayforge.") {
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
	observability.RecordAccepted(ctx)
	slog.InfoContext(ctx, "event accepted", "event_id", eventID, "outbox_id", outboxID)
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
	var event eventResponse
	err := s.DB.QueryRow(r.Context(), `SELECT id,event_type,created_at FROM events WHERE id=$1 AND organization_id=$2`, chi.URLParam(r, "id"), tenant(r)).Scan(&event.ID, &event.Type, &event.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "Event not found")
		return
	}
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	event.Status = "accepted"
	writeJSON(w, 200, event)
}

type eventDeliverySummary struct {
	Total      int64 `json:"total"`
	Succeeded  int64 `json:"succeeded"`
	Exhausted  int64 `json:"exhausted"`
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	RetryWait  int64 `json:"retry_wait"`
	Cancelled  int64 `json:"cancelled"`
}

type eventListView struct {
	eventResponse
	DeliverySummary eventDeliverySummary `json:"delivery_summary"`
}

func eventFilterTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	// The console labels datetime-local inputs as UTC before submitting them.
	return time.ParseInLocation("2006-01-02T15:04", raw, time.UTC)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := pageParams(r)
	if err != nil || (cursor != nil && !id.Valid(cursor.ID)) {
		problem(w, 400, "invalid_pagination", "Invalid event page")
		return
	}
	q := r.URL.Query()
	var eventID, typePrefix, after, before, cursorTime, cursorID, outcome any
	if raw := strings.TrimSpace(q.Get("event_id")); raw != "" {
		if !id.Valid(raw) {
			problem(w, 400, "invalid_filter", "Invalid event ID")
			return
		}
		eventID = raw
	}
	if raw := strings.TrimSpace(q.Get("type")); raw != "" {
		if !eventTypeSearchPattern.MatchString(raw) {
			problem(w, 400, "invalid_filter", "Invalid event type prefix")
			return
		}
		typePrefix = raw
	}
	if raw := q.Get("created_after"); raw != "" {
		v, parseErr := eventFilterTime(raw)
		if parseErr != nil {
			problem(w, 400, "invalid_filter", "Invalid created_after")
			return
		}
		after = v
	}
	if raw := q.Get("created_before"); raw != "" {
		v, parseErr := eventFilterTime(raw)
		if parseErr != nil {
			problem(w, 400, "invalid_filter", "Invalid created_before")
			return
		}
		before = v
	}
	if after != nil && before != nil && !after.(time.Time).Before(before.(time.Time)) {
		problem(w, 400, "invalid_filter", "created_after must be before created_before")
		return
	}
	if raw := q.Get("outcome"); raw != "" {
		switch raw {
		case "no_delivery", "pending", "processing", "retry_wait", "succeeded", "exhausted", "cancelled":
			outcome = raw
		default:
			problem(w, 400, "invalid_filter", "Invalid delivery outcome")
			return
		}
	}
	if cursor != nil {
		cursorTime, cursorID = cursor.Time, cursor.ID
	}
	rows, err := s.DB.Query(r.Context(), `SELECT e.id,e.event_type,e.created_at,
  summary.total,summary.succeeded,summary.exhausted,summary.pending,summary.processing,summary.retry_wait,summary.cancelled
FROM events e
CROSS JOIN LATERAL (SELECT count(*)::bigint AS total,
  count(*) FILTER (WHERE status='succeeded')::bigint AS succeeded,
  count(*) FILTER (WHERE status='exhausted')::bigint AS exhausted,
  count(*) FILTER (WHERE status='pending')::bigint AS pending,
  count(*) FILTER (WHERE status='processing')::bigint AS processing,
  count(*) FILTER (WHERE status='retry_wait')::bigint AS retry_wait,
  count(*) FILTER (WHERE status='cancelled')::bigint AS cancelled
  FROM deliveries WHERE organization_id=e.organization_id AND event_id=e.id) summary
WHERE e.organization_id=$1
  AND ($2::uuid IS NULL OR e.id=$2)
  AND ($3::text IS NULL OR left(e.event_type,length($3::text))=$3)
  AND ($4::timestamptz IS NULL OR e.created_at >= $4)
  AND ($5::timestamptz IS NULL OR e.created_at < $5)
  AND ($6::timestamptz IS NULL OR (e.created_at,e.id)<($6,$7::uuid))
  AND ($8::text IS NULL OR ($8='no_delivery' AND summary.total=0) OR
       ($8<>'no_delivery' AND EXISTS (SELECT 1 FROM deliveries d WHERE d.organization_id=e.organization_id AND d.event_id=e.id AND d.status=$8)))
ORDER BY e.created_at DESC,e.id DESC LIMIT $9`, tenant(r), eventID, typePrefix, after, before, cursorTime, cursorID, outcome, limit+1)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	items := make([]eventListView, 0)
	for rows.Next() {
		var e eventListView
		if err = rows.Scan(&e.ID, &e.Type, &e.CreatedAt, &e.DeliverySummary.Total, &e.DeliverySummary.Succeeded, &e.DeliverySummary.Exhausted, &e.DeliverySummary.Pending, &e.DeliverySummary.Processing, &e.DeliverySummary.RetryWait, &e.DeliverySummary.Cancelled); err != nil {
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
