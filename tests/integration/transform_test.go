package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestDeliveryUsesSnapshotOfFieldMap(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "transform")
	var received map[string]any
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE endpoints SET field_map='{"public_id":"order_id"}'::jsonb WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	resp, body := request(t, "POST", server.URL+"/v1/events", key, `{"type":"order.created","data":{"order_id":"abc","secret":"hide"}}`)
	if resp.StatusCode != 202 {
		t.Fatalf("publish status=%d body=%s", resp.StatusCode, body)
	}
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE endpoints SET field_map='{"changed":"secret"}'::jsonb WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	claim, err := delivery.ClaimOne(ctx, pool, "transform-worker", 45*time.Second)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	result := testSender(pool).Send(ctx, *claim)
	if result.Outcome != "success" {
		t.Fatalf("send=%+v", result)
	}
	data, ok := received["data"].(map[string]any)
	if !ok || len(data) != 1 || data["public_id"] != "abc" {
		t.Fatalf("received=%v", received)
	}
}
