package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestOverviewAnalyticsTenantScopedOutcomesAndRetries(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "overview-a")
	_, otherKey := tenantKey(t, pool, "overview-b")
	var receiverStatus atomic.Int32
	receiverStatus.Store(http.StatusNoContent)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(receiverStatus.Load()))
	}))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	complete := func(claim *delivery.Claimed) {
		t.Helper()
		result := testSender(pool).Send(ctx, *claim)
		if applied, err := delivery.Complete(ctx, pool, *claim, result); err != nil || !applied {
			t.Fatalf("complete applied=%v err=%v result=%+v", applied, err, result)
		}
	}

	publishDeliveryEvent(t, server, key)
	complete(dispatchAndClaim(t, pool, "overview-success"))

	if _, err := pool.Exec(ctx, `UPDATE endpoints SET max_attempts=1 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	receiverStatus.Store(http.StatusBadRequest)
	publishDeliveryEvent(t, server, key)
	complete(dispatchAndClaim(t, pool, "overview-exhausted"))

	if _, err := pool.Exec(ctx, `UPDATE endpoints SET max_attempts=3 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	receiverStatus.Store(http.StatusServiceUnavailable)
	publishDeliveryEvent(t, server, key)
	retryClaim := dispatchAndClaim(t, pool, "overview-retry-first")
	complete(retryClaim)
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET next_attempt_at=now()-interval '1 second' WHERE id=$1 AND status='retry_wait'`, retryClaim.ID); err != nil {
		t.Fatal(err)
	}
	receiverStatus.Store(http.StatusNoContent)
	secondClaim, err := delivery.ClaimOne(ctx, pool, "overview-retry-second", 45*time.Second)
	if err != nil || secondClaim == nil {
		t.Fatalf("second claim=%v err=%v", secondClaim, err)
	}
	complete(secondClaim)

	for _, item := range []struct {
		key      string
		events   int64
		success  int64
		exhaust  int64
		attempts int64
		failed   int64
		retries  int64
	}{{key, 3, 2, 1, 4, 2, 1}, {otherKey, 0, 0, 0, 0, 0, 0}} {
		resp, body := request(t, "GET", server.URL+"/v1/analytics/overview?window=24h", item.key, "")
		if resp.StatusCode != 200 {
			t.Fatalf("overview status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Window string `json:"window"`
			Totals struct {
				Events         int64 `json:"events"`
				Succeeded      int64 `json:"succeeded"`
				Exhausted      int64 `json:"exhausted"`
				Attempts       int64 `json:"attempts"`
				FailedAttempts int64 `json:"failed_attempts"`
				Retries        int64 `json:"retries"`
			} `json:"totals"`
			Buckets []struct {
				Events       int64    `json:"events"`
				P95LatencyMS *float64 `json:"p95_latency_ms"`
			} `json:"buckets"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if out.Window != "24h" || len(out.Buckets) != 24 || out.Totals.Events != item.events || out.Totals.Succeeded != item.success || out.Totals.Exhausted != item.exhaust || out.Totals.Attempts != item.attempts || out.Totals.FailedAttempts != item.failed || out.Totals.Retries != item.retries {
			t.Fatalf("wrong overview totals: %+v", out)
		}
		var chartEvents int64
		var hasLatency bool
		for _, bucket := range out.Buckets {
			chartEvents += bucket.Events
			hasLatency = hasLatency || bucket.P95LatencyMS != nil
		}
		if chartEvents != item.events || hasLatency != (item.events > 0) {
			t.Fatalf("bucket events=%d latency=%v want events=%d", chartEvents, hasLatency, item.events)
		}
	}
	for _, window := range []string{"7d", "30d"} {
		resp, body := request(t, "GET", server.URL+"/v1/analytics/overview?window="+window, key, "")
		if resp.StatusCode != 200 {
			t.Fatalf("%s status=%d body=%s", window, resp.StatusCode, body)
		}
		var out struct {
			Buckets []json.RawMessage `json:"buckets"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		want := 7
		if window == "30d" {
			want = 30
		}
		if len(out.Buckets) != want {
			t.Fatalf("%s buckets=%d want=%d", window, len(out.Buckets), want)
		}
	}
	resp, _ := request(t, "GET", server.URL+"/v1/analytics/overview?window=bad", key, "")
	if resp.StatusCode != 400 {
		t.Fatalf("invalid window status=%d", resp.StatusCode)
	}
}
