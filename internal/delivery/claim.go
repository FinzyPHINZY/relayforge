package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/observability"
)

type Claimed struct {
	ID, AttemptID, OrganizationID, EndpointID, EventID, DestinationURL, EventType string
	Payload                                                                       []byte
	EventCreatedAt                                                                time.Time
	Timeout                                                                       time.Duration
	AttemptNumber, MaxAttempts                                                    int
	LeaseGeneration                                                               int64
	WorkerID                                                                      string
	TraceParent                                                                   string
}

// ClaimOne atomically reserves a due delivery and its attempt. It returns nil
// when no work is due. The caller must commit before making any HTTP request.
func ClaimOne(ctx context.Context, pool *pgxpool.Pool, workerID string, lease time.Duration) (*Claimed, error) {
	return ClaimOneWithLimits(ctx, pool, workerID, lease, 2, 10)
}

// ClaimOneWithLimits shares endpoint and tenant capacity across all workers.
// Least-recently-served tenant order keeps a busy tenant from monopolizing claims.
func ClaimOneWithLimits(ctx context.Context, pool *pgxpool.Pool, workerID string, lease time.Duration, endpointLimit, tenantLimit int) (*Claimed, error) {
	if endpointLimit < 1 || tenantLimit < 1 {
		return nil, fmt.Errorf("claim limits must be positive")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var c Claimed
	var timeoutMS int
	var parent *string
	err = tx.QueryRow(ctx, `SELECT d.id,d.organization_id,d.endpoint_id,d.event_id,d.destination_url,d.timeout_ms,d.max_attempts,e.event_type,e.payload,e.created_at,d.traceparent FROM deliveries d JOIN organizations org ON org.id=d.organization_id JOIN endpoints ep ON ep.id=d.endpoint_id AND ep.organization_id=d.organization_id JOIN endpoint_circuits ec ON ec.endpoint_id=ep.id AND ec.organization_id=ep.organization_id JOIN organization_scheduling os ON os.organization_id=d.organization_id JOIN events e ON e.id=d.event_id AND e.organization_id=d.organization_id WHERE d.status IN ('pending','retry_wait') AND d.next_attempt_at<=now() AND org.status='active' AND ep.status='active' AND ec.active_claims<$1 AND os.active_claims<$2 AND (ec.open_until IS NULL OR (ec.open_until<=now() AND ec.probe_delivery_id IS NULL)) ORDER BY os.last_claimed_at NULLS FIRST,d.next_attempt_at,d.id FOR UPDATE OF d,ec,os SKIP LOCKED FOR SHARE OF org LIMIT 1`, endpointLimit, tenantLimit).Scan(&c.ID, &c.OrganizationID, &c.EndpointID, &c.EventID, &c.DestinationURL, &timeoutMS, &c.MaxAttempts, &c.EventType, &c.Payload, &c.EventCreatedAt, &parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parent != nil {
		c.TraceParent = *parent
	}
	traceCtx, span := otel.Tracer("relayforge/worker").Start(observability.ContextWithTraceParent(ctx, c.TraceParent), "delivery.claim")
	ctx = traceCtx
	defer span.End()
	c.TraceParent = observability.TraceParent(ctx)
	c.AttemptID, err = id.New()
	if err != nil {
		return nil, err
	}
	c.WorkerID = workerID
	c.Timeout = time.Duration(timeoutMS) * time.Millisecond
	tag, err := tx.Exec(ctx, `UPDATE endpoint_circuits SET active_claims=active_claims+1,probe_delivery_id=CASE WHEN open_until IS NOT NULL AND open_until<=now() THEN $2 ELSE probe_delivery_id END,updated_at=now() WHERE endpoint_id=$1 AND organization_id=$3`, c.EndpointID, c.ID, c.OrganizationID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, fmt.Errorf("endpoint scheduling row missing")
	}
	tag, err = tx.Exec(ctx, `UPDATE organization_scheduling SET active_claims=active_claims+1,last_claimed_at=clock_timestamp(),updated_at=now() WHERE organization_id=$1`, c.OrganizationID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, fmt.Errorf("tenant scheduling row missing")
	}
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
		id, org, endpoint string
		attempt, max      int
	}
	rows, err := tx.Query(ctx, `SELECT id,organization_id,endpoint_id,attempt_count,max_attempts FROM deliveries WHERE status='processing' AND lease_expires_at<=now() ORDER BY lease_expires_at,id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	items := make([]expired, 0, limit)
	for rows.Next() {
		var x expired
		if err = rows.Scan(&x.id, &x.org, &x.endpoint, &x.attempt, &x.max); err != nil {
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
		tag, e := tx.Exec(ctx, `UPDATE endpoint_circuits SET active_claims=active_claims-1,probe_delivery_id=CASE WHEN probe_delivery_id=$1 THEN NULL ELSE probe_delivery_id END,updated_at=now() WHERE endpoint_id=$2 AND organization_id=$3 AND active_claims>0`, x.id, x.endpoint, x.org)
		if e != nil {
			return 0, e
		}
		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf("endpoint claim accounting missing during reclaim")
		}
		tag, err = tx.Exec(ctx, `UPDATE organization_scheduling SET active_claims=active_claims-1,updated_at=now() WHERE organization_id=$1 AND active_claims>0`, x.org)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf("tenant claim accounting missing during reclaim")
		}
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
