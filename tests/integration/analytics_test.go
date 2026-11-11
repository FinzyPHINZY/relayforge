package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"webhook_delivery/internal/delivery"
)

func TestDeliveryAnalyticsTenantIsolation(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "analytics-a")
	_, otherKey := tenantKey(t, pool, "analytics-b")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, server, key)
	claim := dispatchAndClaim(t, pool, "analytics-worker")
	result := testSender(pool).Send(context.Background(), *claim)
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("complete=%v err=%v", applied, err)
	}
	for _, item := range []struct {
		key  string
		want int64
	}{{key, 1}, {otherKey, 0}} {
		resp, body := request(t, "GET", server.URL+"/v1/analytics/deliveries?window=24h", item.key, "")
		if resp.StatusCode != 200 {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Buckets []struct {
				Attempts int64 `json:"attempts"`
			} `json:"buckets"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		var total int64
		for _, bucket := range out.Buckets {
			total += bucket.Attempts
		}
		if total != item.want {
			t.Fatalf("attempts=%d want=%d", total, item.want)
		}
	}
}
