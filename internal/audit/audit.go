package audit

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
)

type Entry struct {
	OrganizationID string
	ActorKeyID     string // Empty only for trusted system actions.
	ActorUserID    string // Set when a signed-in user acts; empty for key or system actors.
	ActorRole      string
	Action         string
	TargetType     string
	TargetID       string
	Details        map[string]any // Pass only non-secret, allowlisted values.
}

// Write must use the caller's mutation transaction so an action and its
// audit record either commit together or neither commits.
func Write(ctx context.Context, tx pgx.Tx, entry Entry) error {
	if !id.Valid(entry.OrganizationID) || !id.Valid(entry.TargetID) || (entry.ActorKeyID != "" && !id.Valid(entry.ActorKeyID)) || (entry.ActorUserID != "" && !id.Valid(entry.ActorUserID)) {
		return errors.New("invalid audit identifier")
	}
	if (entry.ActorRole == "system") != (entry.ActorKeyID == "" && entry.ActorUserID == "") {
		return errors.New("invalid audit actor")
	}
	if entry.Details == nil {
		entry.Details = map[string]any{}
	}
	details, err := json.Marshal(entry.Details)
	if err != nil {
		return err
	}
	auditID, err := id.New()
	if err != nil {
		return err
	}
	var actor, actorUser any
	if entry.ActorKeyID != "" {
		actor = entry.ActorKeyID
	}
	if entry.ActorUserID != "" {
		actorUser = entry.ActorUserID
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,organization_id,actor_api_key_id,actor_user_id,actor_role,action,target_type,target_id,details) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, auditID, entry.OrganizationID, actor, actorUser, entry.ActorRole, entry.Action, entry.TargetType, entry.TargetID, details)
	return err
}
