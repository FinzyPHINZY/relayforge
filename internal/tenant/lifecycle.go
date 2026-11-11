package tenant

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/id"
)

// Export contains customer data and credential metadata, never API key hashes or
// encrypted signing secrets. It is a portability export, not a database backup.
var exportQueries = []struct{ name, query string }{
	{"organizations", `SELECT row_to_json(t) FROM (SELECT id,name,status,monthly_event_quota,max_endpoints,created_at,updated_at FROM organizations WHERE id=$1) t`},
	{"api_keys", `SELECT row_to_json(t) FROM (SELECT id,organization_id,member_id,name,key_prefix,scopes,created_at,last_used_at,expires_at,revoked_at FROM api_keys WHERE organization_id=$1) t`},
	{"team_members", `SELECT row_to_json(t) FROM (SELECT * FROM team_members WHERE organization_id=$1) t`},
	{"endpoints", `SELECT row_to_json(t) FROM (SELECT * FROM endpoints WHERE organization_id=$1) t`},
	{"endpoint_secrets", `SELECT row_to_json(t) FROM (SELECT id,organization_id,endpoint_id,key_id,state,created_at,retired_at,overlap_until FROM endpoint_secrets WHERE organization_id=$1) t`},
	{"subscriptions", `SELECT row_to_json(t) FROM (SELECT * FROM subscriptions WHERE organization_id=$1) t`},
	{"events", `SELECT row_to_json(t) FROM (SELECT * FROM events WHERE organization_id=$1) t`},
	{"outbox", `SELECT row_to_json(t) FROM (SELECT * FROM outbox WHERE organization_id=$1) t`},
	{"idempotency_records", `SELECT row_to_json(t) FROM (SELECT organization_id,idempotency_key,event_id,created_at,expires_at FROM idempotency_records WHERE organization_id=$1) t`},
	{"monthly_event_usage", `SELECT row_to_json(t) FROM (SELECT * FROM monthly_event_usage WHERE organization_id=$1) t`},
	{"deliveries", `SELECT row_to_json(t) FROM (SELECT * FROM deliveries WHERE organization_id=$1) t`},
	{"delivery_attempts", `SELECT row_to_json(t) FROM (SELECT * FROM delivery_attempts WHERE organization_id=$1) t`},
	{"replay_requests", `SELECT row_to_json(t) FROM (SELECT * FROM replay_requests WHERE organization_id=$1) t`},
	{"delivery_reviews", `SELECT row_to_json(t) FROM (SELECT * FROM delivery_reviews WHERE organization_id=$1) t`},
	{"delivery_review_history", `SELECT row_to_json(t) FROM (SELECT * FROM delivery_review_history WHERE organization_id=$1) t`},
	{"endpoint_circuits", `SELECT row_to_json(t) FROM (SELECT * FROM endpoint_circuits WHERE organization_id=$1) t`},
	{"organization_scheduling", `SELECT row_to_json(t) FROM (SELECT * FROM organization_scheduling WHERE organization_id=$1) t`},
}

func Suspend(ctx context.Context, pool *pgxpool.Pool, orgID string) error {
	if !id.Valid(orgID) {
		return errors.New("invalid organization ID")
	}
	tag, err := pool.Exec(ctx, `UPDATE organizations SET status='suspended',updated_at=now() WHERE id=$1`, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("organization not found")
	}
	return nil
}

type exportLine struct {
	Type           string          `json:"type"`
	OrganizationID string          `json:"organization_id,omitempty"`
	ExportedAt     *time.Time      `json:"exported_at,omitempty"`
	Table          string          `json:"table,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Records        int             `json:"records,omitempty"`
}

// Export writes a consistent JSON Lines snapshot to a new owner-only file.
func Export(ctx context.Context, pool *pgxpool.Pool, orgID, path string) (err error) {
	if !id.Valid(orgID) || path == "" {
		return errors.New("valid organization ID and output path required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM organizations WHERE id=$1`, orgID).Scan(&status); err != nil {
		return err
	}
	if status != "suspended" {
		return errors.New("suspend organization before export")
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='processing') + (SELECT active_claims FROM organization_scheduling WHERE organization_id=$1)`, orgID).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return fmt.Errorf("active delivery claims remain (%d); wait and retry", active)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	now := time.Now().UTC()
	if err = enc.Encode(exportLine{Type: "manifest", OrganizationID: orgID, ExportedAt: &now}); err != nil {
		return err
	}
	count := 0
	for _, table := range exportQueries {
		rows, queryErr := tx.Query(ctx, table.query, orgID)
		if queryErr != nil {
			return queryErr
		}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			if err = enc.Encode(exportLine{Type: "record", Table: table.name, Data: raw}); err != nil {
				rows.Close()
				return err
			}
			count++
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if err = enc.Encode(exportLine{Type: "complete", Records: count}); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func checkExport(path, orgID string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var first, line exportLine
	if err := dec.Decode(&first); err != nil || first.Type != "manifest" || first.OrganizationID != orgID {
		return errors.New("export manifest does not match organization")
	}
	count := 0
	for {
		line = exportLine{}
		if err := dec.Decode(&line); err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("export is incomplete")
			}
			return err
		}
		if line.Type == "complete" {
			if line.Records != count || dec.Decode(&line) != io.EOF {
				return errors.New("export record count or ending is invalid")
			}
			return nil
		}
		if line.Type != "record" || line.Table == "" || !json.Valid(line.Data) {
			return errors.New("export record is invalid")
		}
		count++
	}
}

// Purge requires a completed export and a suspended, idle organization. The
// database changes are atomic. Keep a separate database backup for restore.
func Purge(ctx context.Context, pool *pgxpool.Pool, orgID, exportPath string) error {
	if !id.Valid(orgID) {
		return errors.New("invalid organization ID")
	}
	if err := checkExport(exportPath, orgID); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM organizations WHERE id=$1 FOR UPDATE`, orgID).Scan(&status)
	if err != nil {
		return err
	}
	if status != "suspended" {
		return errors.New("organization must be suspended")
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='processing') + (SELECT active_claims FROM organization_scheduling WHERE organization_id=$1)`, orgID).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return fmt.Errorf("active delivery claims remain (%d); wait and retry", active)
	}
	// Clear the self-reference before deleting all delivery runs.
	statements := []string{
		`DELETE FROM delivery_review_history WHERE organization_id=$1`,
		`DELETE FROM delivery_reviews WHERE organization_id=$1`,
		`DELETE FROM replay_requests WHERE organization_id=$1`,
		`DELETE FROM delivery_attempts WHERE organization_id=$1`,
		`UPDATE deliveries SET replayed_from_delivery_id=NULL WHERE organization_id=$1`,
		`DELETE FROM deliveries WHERE organization_id=$1`,
		`DELETE FROM outbox WHERE organization_id=$1`,
		`DELETE FROM idempotency_records WHERE organization_id=$1`,
		`DELETE FROM events WHERE organization_id=$1`,
		`DELETE FROM endpoint_circuits WHERE organization_id=$1`,
		`DELETE FROM endpoint_secrets WHERE organization_id=$1`,
		`DELETE FROM subscriptions WHERE organization_id=$1`,
		`DELETE FROM endpoints WHERE organization_id=$1`,
		`DELETE FROM api_keys WHERE organization_id=$1`,
		`DELETE FROM team_members WHERE organization_id=$1`,
		`DELETE FROM monthly_event_usage WHERE organization_id=$1`,
		`DELETE FROM organization_scheduling WHERE organization_id=$1`,
		`DELETE FROM organizations WHERE id=$1`,
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement, orgID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
