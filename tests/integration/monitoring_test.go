package integration_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/monitor"
)

func TestFailureSignalsReachMetricsAndMonitor(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "monitoring failure")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, server, key)
	claim := dispatchAndClaim(t, pool, "monitor-worker")
	result := testSender(pool).Send(context.Background(), *claim)
	if result.Outcome != "http_error" {
		t.Fatalf("result=%+v", result)
	}
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("complete applied=%v err=%v", applied, err)
	}

	metrics := httptest.NewRecorder()
	health.Handler(pool).ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", metrics.Code, metrics.Body.String())
	}
	for _, metric := range []string{"webhook_delivery_recent_failures_5m 1", "webhook_deliveries_unreviewed_exhausted 1", `webhook_delivery_attempts_recorded{outcome="http_error"} 1`} {
		if !strings.Contains(metrics.Body.String(), metric) {
			t.Fatalf("missing %q in %s", metric, metrics.Body.String())
		}
	}
	checker := monitor.Checker{DB: pool, BacklogAgeLimit: time.Hour, RecentFailureLimit: 1, UnreviewedLimit: 1}
	reasons := checker.Check(context.Background())
	if len(reasons) != 2 || !strings.Contains(strings.Join(reasons, " "), "failed attempts") || !strings.Contains(strings.Join(reasons, " "), "need review") {
		t.Fatalf("reasons=%v", reasons)
	}

	resp, body := request(t, http.MethodPatch, server.URL+"/v1/deliveries/"+claim.ID+"/review", key, `{"status":"investigating"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("review status=%d body=%s", resp.StatusCode, body)
	}
	if reasons = checker.Check(context.Background()); len(reasons) != 1 || !strings.Contains(reasons[0], "failed attempts") {
		t.Fatalf("reasons after review=%v", reasons)
	}
	metrics = httptest.NewRecorder()
	health.Handler(pool).ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), "webhook_deliveries_unreviewed_exhausted 0") {
		data, _ := io.ReadAll(metrics.Result().Body)
		t.Fatalf("metrics after review status=%d body=%s", metrics.Code, data)
	}
}
