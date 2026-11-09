package retention

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Stats struct{ Events, IdempotencyRecords int }

// Sweep removes only expired events whose outbox is dispatched and whose
// deliveries are terminal and older than cutoff. Every batch is atomic.
func Sweep(ctx context.Context, pool *pgxpool.Pool, now, cutoff time.Time, limit int) (Stats, error) {
	var stats Stats
	if !cutoff.Before(now) || limit < 1 || limit > 500 {
		return stats, errors.New("invalid retention parameters")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM idempotency_records WHERE ctid IN (
  SELECT ctid FROM idempotency_records WHERE expires_at<$1 AND created_at<=$1::timestamptz-interval '24 hours'
  ORDER BY expires_at FOR UPDATE SKIP LOCKED LIMIT $2
 )`, now, limit)
	if err != nil {
		return stats, err
	}
	stats.IdempotencyRecords = int(tag.RowsAffected())
	rows, err := tx.Query(ctx, `SELECT e.id FROM events e JOIN outbox o ON o.event_id=e.id
  WHERE e.expires_at<$1 AND e.created_at<$2 AND o.status='dispatched'
  AND NOT EXISTS(SELECT 1 FROM deliveries d WHERE d.event_id=e.id AND (d.status NOT IN ('succeeded','exhausted','cancelled') OR d.completed_at IS NULL OR d.completed_at>=$2))
  ORDER BY e.expires_at,e.id FOR UPDATE OF e SKIP LOCKED LIMIT $3`, now, cutoff, limit)
	if err != nil {
		return stats, err
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var x string
		if err = rows.Scan(&x); err != nil {
			rows.Close()
			return stats, err
		}
		ids = append(ids, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return stats, err
	}
	rows.Close()
	for _, eventID := range ids {
		for _, statement := range []string{
			`DELETE FROM replay_requests WHERE source_delivery_id IN (SELECT id FROM deliveries WHERE event_id=$1) OR replay_delivery_id IN (SELECT id FROM deliveries WHERE event_id=$1)`,
			`DELETE FROM delivery_attempts WHERE delivery_id IN (SELECT id FROM deliveries WHERE event_id=$1)`,
			`DELETE FROM deliveries WHERE event_id=$1`,
			`DELETE FROM outbox WHERE event_id=$1`,
			`DELETE FROM idempotency_records WHERE event_id=$1`,
			`DELETE FROM events WHERE id=$1`,
		} {
			if _, err = tx.Exec(ctx, statement, eventID); err != nil {
				return stats, err
			}
		}
		stats.Events++
	}
	if err = tx.Commit(ctx); err != nil {
		return Stats{}, err
	}
	return stats, nil
}
