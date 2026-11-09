package delivery

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

type outboxItem struct{ id, organizationID, eventID, eventType string }

// DispatchBatch materializes each pending outbox event and marks it dispatched
// in the same transaction. A crash rolls back both operations.
func DispatchBatch(ctx context.Context, pool *pgxpool.Pool, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, fmt.Errorf("invalid dispatch batch size")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT o.id,o.organization_id,o.event_id,e.event_type FROM outbox o JOIN events e ON e.id=o.event_id AND e.organization_id=o.organization_id WHERE o.status='pending' ORDER BY o.created_at,o.id FOR UPDATE OF o SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	items := make([]outboxItem, 0, limit)
	for rows.Next() {
		var x outboxItem
		if err = rows.Scan(&x.id, &x.organizationID, &x.eventID, &x.eventType); err != nil {
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
	if len(items) > 0 {
		traceCtx, span := otel.Tracer("relayforge/dispatcher").Start(ctx, "outbox.dispatch")
		ctx = traceCtx
		defer span.End()
	}
	for _, item := range items {
		_, err = tx.Exec(ctx, `INSERT INTO deliveries (id,organization_id,event_id,endpoint_id,destination_url,timeout_ms,max_attempts) SELECT gen_random_uuid(),e.organization_id,$2,e.id,e.url,e.timeout_ms,e.max_attempts FROM endpoints e WHERE e.organization_id=$1 AND e.status='active' AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.endpoint_id=e.id AND s.organization_id=e.organization_id AND s.event_type IN ($3,'*')) ON CONFLICT (event_id,endpoint_id,run_number) DO NOTHING`, item.organizationID, item.eventID, item.eventType)
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec(ctx, `UPDATE outbox SET status='dispatched',dispatched_at=now() WHERE id=$1 AND organization_id=$2`, item.id, item.organizationID)
		if err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(items), nil
}
