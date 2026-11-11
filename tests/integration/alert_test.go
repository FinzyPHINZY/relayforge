package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestExhaustedDeliveryCreatesOneAlertEvent(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "alerts")
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer failing.Close()
	alertReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer alertReceiver.Close()
	addDeliveryEndpoint(t, pool, org, failing.URL)
	alertEndpoint, _ := addDeliveryEndpoint(t, pool, org, alertReceiver.URL)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE subscriptions SET event_type='relayforge.delivery.exhausted' WHERE endpoint_id=$1`, alertEndpoint); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, server, key)
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	claim, err := delivery.ClaimOne(ctx, pool, "alert-worker", 45*time.Second)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	result := testSender(pool).Send(ctx, *claim)
	if applied, err := delivery.Complete(ctx, pool, *claim, result); err != nil || !applied {
		t.Fatalf("complete=%v err=%v", applied, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE organization_id=$1 AND event_type='relayforge.delivery.exhausted'`, org).Scan(&count); err != nil || count != 1 {
		t.Fatalf("alert count=%d err=%v", count, err)
	}
	if dispatched, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || dispatched != 1 {
		t.Fatalf("alert dispatch=%d err=%v", dispatched, err)
	}
	alertClaim, err := delivery.ClaimOne(ctx, pool, "alert-worker", 45*time.Second)
	if err != nil || alertClaim == nil || alertClaim.EventType != "relayforge.delivery.exhausted" {
		t.Fatalf("alert claim=%+v err=%v", alertClaim, err)
	}
}
