package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"webhook_delivery/internal/id"
)

type Claimed struct {
	ID, AttemptID, OrganizationID, EndpointID, EventID, DestinationURL, EventType string
	Payload                                                                       []byte
	EventCreatedAt                                                                time.Time
	Timeout                                                                       time.Duration
	AttemptNumber, MaxAttempts                                                    int
	LeaseGeneration                                                               int64
	WorkerID                                                                      string
}

// ClaimOne atomically reserves a due delivery and its attempt. It returns nil
// when no work is due. The caller must commit before making any HTTP request.
func ClaimOne(ctx context.Context, pool *pgxpool.Pool, workerID string, lease time.Duration) (*Claimed, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var c Claimed
	var timeoutMS int
	err = tx.QueryRow(ctx, `SELECT d.id,d.organization_id,d.endpoint_id,d.event_id,d.destination_url,d.timeout_ms,d.max_attempts,e.event_type,e.payload,e.created_at FROM deliveries d JOIN endpoints ep ON ep.id=d.endpoint_id AND ep.organization_id=d.organization_id JOIN events e ON e.id=d.event_id AND e.organization_id=d.organization_id WHERE d.status IN ('pending','retry_wait') AND d.next_attempt_at<=now() AND ep.status='active' ORDER BY d.next_attempt_at,d.id FOR UPDATE OF d SKIP LOCKED LIMIT 1`).Scan(&c.ID, &c.OrganizationID, &c.EndpointID, &c.EventID, &c.DestinationURL, &timeoutMS, &c.MaxAttempts, &c.EventType, &c.Payload, &c.EventCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	traceCtx, span := otel.Tracer("relayforge/worker").Start(ctx, "delivery.claim")
	ctx = traceCtx
	defer span.End()
	c.AttemptID, err = id.New()
	if err != nil {
		return nil, err
	}
	c.WorkerID = workerID
	c.Timeout = time.Duration(timeoutMS) * time.Millisecond
	err = tx.QueryRow(ctx, `UPDATE deliveries SET status='processing',lease_owner=$2,lease_expires_at=now()+($3::bigint*interval '1 microsecond'),lease_generation=lease_generation+1,attempt_count=attempt_count+1,updated_at=now() WHERE id=$1 AND status IN ('pending','retry_wait') AND attempt_count<max_attempts RETURNING attempt_count,lease_generation`, c.ID, workerID, lease.Microseconds()).Scan(&c.AttemptNumber, &c.LeaseGeneration)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO delivery_attempts (id,organization_id,delivery_id,attempt_number,worker_id,lease_generation) VALUES ($1,$2,$3,$4,$5,$6)`, c.AttemptID, c.OrganizationID, c.ID, c.AttemptNumber, workerID, c.LeaseGeneration)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

// ReclaimExpired closes abandoned attempts and makes their deliveries due again.
// Incrementing the generation rejects a late completion from the old owner.
func ReclaimExpired(ctx context.Context, pool *pgxpool.Pool, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	type expired struct {
		id, org      string
		attempt, max int
	}
	rows, err := tx.Query(ctx, `SELECT id,organization_id,attempt_count,max_attempts FROM deliveries WHERE status='processing' AND lease_expires_at<=now() ORDER BY lease_expires_at,id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	items := make([]expired, 0, limit)
	for rows.Next() {
		var x expired
		if err = rows.Scan(&x.id, &x.org, &x.attempt, &x.max); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, x := range items {
		_, err = tx.Exec(ctx, `UPDATE delivery_attempts SET finished_at=now(),error_code='lease_expired' WHERE delivery_id=$1 AND organization_id=$2 AND attempt_number=$3 AND finished_at IS NULL`, x.id, x.org, x.attempt)
		if err != nil {
			return 0, err
		}
		status := "retry_wait"
		if x.attempt >= x.max {
			status = "exhausted"
		}
		_, err = tx.Exec(ctx, `UPDATE deliveries SET status=$2,lease_owner=NULL,lease_expires_at=NULL,lease_generation=lease_generation+1,next_attempt_at=now(),last_error_code='lease_expired',completed_at=CASE WHEN $2='exhausted' THEN now() ELSE NULL END,updated_at=now() WHERE id=$1`, x.id, status)
		if err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(items), nil
}
