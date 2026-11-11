package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestScheduledEventWaitsUntilDue(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "scheduled")
	addDeliveryEndpoint(t, pool, org, "https://example.com/webhook")
	due := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	body := fmt.Sprintf(`{"type":"order.created","data":{"order_id":"scheduled"},"deliver_at":%q}`, due.Format(time.RFC3339))
	resp, content := request(t, http.MethodPost, server.URL+"/v1/events", key, body)
	if resp.StatusCode != 202 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, content)
	}
	if count, err := delivery.DispatchBatch(context.Background(), pool, 50); err != nil || count != 0 {
		t.Fatalf("early dispatch=%d err=%v", count, err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE outbox SET available_at=now()-interval '1 second' WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	if count, err := delivery.DispatchBatch(context.Background(), pool, 50); err != nil || count != 1 {
		t.Fatalf("due dispatch=%d err=%v", count, err)
	}
}
