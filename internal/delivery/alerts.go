package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
)

const exhaustedAlertType = "relayforge.delivery.exhausted"

// enqueueExhaustedAlert shares the delivery transition's transaction.
func enqueueExhaustedAlert(ctx context.Context, tx pgx.Tx, orgID, deliveryID, endpointID, eventID, eventType, errorCode string, httpStatus int) error {
	if eventType == exhaustedAlertType {
		return nil
	}
	var subscribed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscriptions s JOIN endpoints e ON e.id=s.endpoint_id AND e.organization_id=s.organization_id WHERE s.organization_id=$1 AND s.event_type=$2 AND e.status='active')`, orgID, exhaustedAlertType).Scan(&subscribed); err != nil {
		return err
	}
	if !subscribed {
		return nil
	}
	alertID, err := id.New()
	if err != nil {
		return err
	}
	outboxID, err := id.New()
	if err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"delivery_id": deliveryID, "endpoint_id": endpointID, "event_id": eventID, "error_code": errorCode, "http_status": httpStatus})
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	if _, err = tx.Exec(ctx, `INSERT INTO events (id,organization_id,event_type,payload,payload_sha256) VALUES ($1,$2,$3,$4,$5)`, alertID, orgID, exhaustedAlertType, data, hash[:]); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (id,organization_id,event_id) VALUES ($1,$2,$3)`, outboxID, orgID, alertID)
	return err
}
