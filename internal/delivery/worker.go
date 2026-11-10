package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

// Complete commits the attempt result and delivery transition together. A
// reclaimed or expired lease returns applied=false without changing history.
func Complete(ctx context.Context, pool *pgxpool.Pool, c Claimed, r Result) (bool, error) {
	if r.ErrorCode == "stale_lease" {
		return false, nil
	}
	ctx, span := otel.Tracer("relayforge/worker").Start(ctx, "delivery.complete")
	defer span.End()
	status := "exhausted"
	next := time.Now()
	if r.Outcome == "success" {
		status = "succeeded"
	} else if r.Retryable && c.AttemptNumber < c.MaxAttempts {
		status = "retry_wait"
		delay := RetryDelay(c.AttemptNumber, rand.Float64())
		if r.RetryAfter > delay {
			delay = r.RetryAfter
		}
		next = next.Add(delay)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var updatedID string
	err = tx.QueryRow(ctx, `UPDATE deliveries SET status=$5,lease_owner=NULL,lease_expires_at=NULL,next_attempt_at=$6,last_http_status=$7,last_error_code=$8,completed_at=CASE WHEN $5 IN ('succeeded','exhausted') THEN now() ELSE NULL END,updated_at=now() WHERE id=$1 AND organization_id=$2 AND lease_owner=$3 AND lease_generation=$4 AND status='processing' AND lease_expires_at>now() RETURNING id`, c.ID, c.OrganizationID, c.WorkerID, c.LeaseGeneration, status, next, nullableHTTPStatus(r.HTTPStatus), nullableError(r.ErrorCode)).Scan(&updatedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE delivery_attempts SET finished_at=now(),duration_ms=$3,http_status=$4,outcome=$5,error_code=$6,response_excerpt=$7 WHERE id=$1 AND delivery_id=$2 AND finished_at IS NULL`, c.AttemptID, c.ID, int(r.Duration.Milliseconds()), nullableHTTPStatus(r.HTTPStatus), r.Outcome, nullableError(r.ErrorCode), r.ResponseExcerpt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("attempt reservation missing")
	}
	if r.Retryable && r.Outcome != "success" {
		_, err = tx.Exec(ctx, `UPDATE endpoint_circuits SET failure_streak=CASE WHEN probe_delivery_id=$3 THEN 3 ELSE failure_streak+1 END,open_until=CASE WHEN probe_delivery_id=$3 OR failure_streak+1>=3 THEN now()+interval '1 minute' ELSE NULL END,probe_delivery_id=NULL,updated_at=now() WHERE endpoint_id=$1 AND organization_id=$2 AND (open_until IS NULL OR probe_delivery_id=$3)`, c.EndpointID, c.OrganizationID, c.ID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE endpoint_circuits SET failure_streak=0,open_until=NULL,probe_delivery_id=NULL,updated_at=now() WHERE endpoint_id=$1 AND organization_id=$2 AND (open_until IS NULL OR probe_delivery_id=$3)`, c.EndpointID, c.OrganizationID, c.ID)
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func nullableHTTPStatus(status int) any {
	if status == 0 {
		return nil
	}
	return status
}
func nullableError(code string) any {
	if code == "" {
		return nil
	}
	return code
}

type Engine struct {
	DB           *pgxpool.Pool
	Sender       Sender
	WorkerID     string
	Concurrency  int
	Lease        time.Duration
	DrainTimeout time.Duration
	SendFunc     func(context.Context, Claimed) Result
}

func (e Engine) Run(ctx context.Context) {
	if e.Concurrency < 1 {
		e.Concurrency = 1
	}
	if e.Lease <= 0 {
		e.Lease = 45 * time.Second
	}
	if e.DrainTimeout <= 0 {
		e.DrainTimeout = 35 * time.Second
	}
	sendCtx, cancelSend := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelSend()
	var workers sync.WaitGroup
	for i := 0; i < e.Concurrency; i++ {
		workers.Add(1)
		go func(i int) { defer workers.Done(); e.workLoop(ctx, sendCtx, fmt.Sprintf("%s-%d", e.WorkerID, i)) }(i)
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done := make(chan struct{})
			go func() { workers.Wait(); close(done) }()
			timer := time.NewTimer(e.DrainTimeout)
			defer timer.Stop()
			select {
			case <-done:
				return
			case <-timer.C:
				slog.Warn("worker drain deadline reached")
				cancelSend()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					slog.Error("worker forced stop deadline reached")
				}
			}
			return
		case <-ticker.C:
			count, err := ReclaimExpired(ctx, e.DB, 100)
			if err != nil && ctx.Err() == nil {
				slog.Error("lease reclaim failed", "error", err)
			}
			if count > 0 {
				slog.Info("reclaimed expired leases", "count", count)
			}
		}
	}
}

func (e Engine) workLoop(claimCtx, sendCtx context.Context, workerID string) {
	for claimCtx.Err() == nil {
		claim, err := ClaimOne(claimCtx, e.DB, workerID, e.Lease)
		if err != nil {
			if claimCtx.Err() == nil {
				slog.Error("claim failed", "error", err)
			}
			pause(claimCtx, time.Second)
			continue
		}
		if claim == nil {
			pause(claimCtx, time.Second)
			continue
		}
		attemptCtx, span := otel.Tracer("relayforge/worker").Start(sendCtx, "webhook.attempt")
		var result Result
		if e.SendFunc != nil {
			result = e.SendFunc(attemptCtx, *claim)
		} else {
			result = e.Sender.Send(attemptCtx, *claim)
		}
		applied, err := Complete(attemptCtx, e.DB, *claim, result)
		span.End()
		if err != nil && sendCtx.Err() == nil {
			slog.Error("completion failed", "delivery_id", claim.ID, "error", err)
		}
		if !applied && err == nil {
			slog.Warn("stale completion ignored", "delivery_id", claim.ID)
		}
	}
}

func pause(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
