package tenant

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/accounts"
	"webhook_delivery/internal/audit"
	"webhook_delivery/internal/id"
)

// LinkOwner makes the person who controls email an active owner of an existing
// organization, creating the user when needed. The operator asserts ownership
// of the address, so the user is marked verified, but a magic link must still be
// clicked to sign in. It is idempotent and returns the user ID.
func LinkOwner(ctx context.Context, pool *pgxpool.Pool, orgID, rawEmail string) (string, error) {
	if !id.Valid(orgID) {
		return "", errors.New("invalid organization ID")
	}
	email, err := accounts.NormaliseEmail(rawEmail)
	if err != nil {
		return "", err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// Locking the organization serialises concurrent link runs.
	var orgStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM organizations WHERE id=$1 FOR UPDATE`, orgID).Scan(&orgStatus); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err != nil || orgStatus != "active" {
		return "", errors.New("organization not found or not active")
	}

	local, _, _ := strings.Cut(email, "@")
	var userID, userStatus string
	err = tx.QueryRow(ctx, `SELECT id::text, status FROM users WHERE email=$1 FOR UPDATE`, email).Scan(&userID, &userStatus)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if userID, err = id.New(); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO users (id,email,email_verified_at,display_name,kind) VALUES ($1,$2,now(),$3,'regular')`, userID, email, local); err != nil {
			return "", err
		}
	case err != nil:
		return "", err
	case userStatus != "active":
		return "", errors.New("user is not active")
	default:
		if _, err = tx.Exec(ctx, `UPDATE users SET email_verified_at=now() WHERE id=$1 AND email_verified_at IS NULL`, userID); err != nil {
			return "", err
		}
	}

	var name string
	if err = tx.QueryRow(ctx, `SELECT coalesce(display_name,$2) FROM users WHERE id=$1`, userID, local).Scan(&name); err != nil {
		return "", err
	}

	// Prefer the row already linked to this user; otherwise adopt the row for this email.
	var memberID, role, status string
	var linked bool
	err = tx.QueryRow(ctx, `SELECT id::text, role, status, user_id IS NOT NULL FROM team_members
		WHERE organization_id=$1 AND (user_id=$2 OR email=$3)
		ORDER BY (user_id=$2) DESC NULLS LAST LIMIT 1 FOR UPDATE`, orgID, userID, email).Scan(&memberID, &role, &status, &linked)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if memberID, err = id.New(); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO team_members (id,organization_id,name,email,role,status,user_id) VALUES ($1,$2,$3,$4,'owner','active',$5)`, memberID, orgID, name, email, userID); err != nil {
			return "", err
		}
		err = audit.Write(ctx, tx, audit.Entry{OrganizationID: orgID, ActorRole: "system", Action: "team.member_created", TargetType: "team_member", TargetID: memberID})
	case err != nil:
		return "", err
	case role == "owner" && status == "active" && linked:
		// Already linked: nothing to change or audit.
	default:
		if _, err = tx.Exec(ctx, `UPDATE team_members SET role='owner',status='active',user_id=$2 WHERE id=$1`, memberID, userID); err != nil {
			return "", err
		}
		err = audit.Write(ctx, tx, audit.Entry{OrganizationID: orgID, ActorRole: "system", Action: "team.member_updated", TargetType: "team_member", TargetID: memberID, Details: map[string]any{"role": "owner"}})
	}
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}
