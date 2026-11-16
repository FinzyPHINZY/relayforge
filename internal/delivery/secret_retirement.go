package delivery

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RetireExpiredSecrets removes recoverable key material after the overlap ends.
// Secret metadata remains for delivery and audit history.
func RetireExpiredSecrets(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `UPDATE endpoint_secrets SET state='retired',retired_at=overlap_until,ciphertext=''::bytea,nonce=''::bytea WHERE state='overlap' AND overlap_until<=now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
