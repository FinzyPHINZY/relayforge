package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
)

func TestEndpointPolicyValidationAndTenantIsolation(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "policy owner")
	_, stranger := tenantKey(t, pool, "policy stranger")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	resp, body := request(t, http.MethodPost, server.URL+"/v1/endpoints", key, `{"url":"https://8.8.8.8/webhooks","event_types":["order.created"],"concurrency_limit":4,"rate_limit_per_minute":120}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create with policy status=%d body=%s", resp.StatusCode, body)
	}
	var created struct {
		Endpoint struct {
			ConcurrencyLimit   int `json:"concurrency_limit"`
			RateLimitPerMinute int `json:"rate_limit_per_minute"`
		} `json:"endpoint"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Endpoint.ConcurrencyLimit != 4 || created.Endpoint.RateLimitPerMinute != 120 {
		t.Fatalf("created policy=%+v err=%v", created, err)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/endpoints", key, `{"url":"https://8.8.8.8/webhooks","event_types":["order.created"],"concurrency_limit":0}`)
	if resp.StatusCode != 422 {
		t.Fatalf("explicit zero concurrency status=%d body=%s", resp.StatusCode, body)
	}
	for _, body := range []string{`{"concurrency_limit":0}`, `{"concurrency_limit":101}`, `{"rate_limit_per_minute":-1}`, `{"rate_limit_per_minute":6001}`} {
		resp, data := request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, key, body)
		if resp.StatusCode != 422 {
			t.Fatalf("invalid policy %s: status=%d body=%s", body, resp.StatusCode, data)
		}
	}
	resp, _ = request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, stranger, `{"concurrency_limit":3,"rate_limit_per_minute":30}`)
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant patch status=%d", resp.StatusCode)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, key, `{"concurrency_limit":3,"rate_limit_per_minute":30}`)
	if resp.StatusCode != 200 {
		t.Fatalf("policy patch status=%d body=%s", resp.StatusCode, body)
	}
	var got struct {
		ConcurrencyLimit   int `json:"concurrency_limit"`
		RateLimitPerMinute int `json:"rate_limit_per_minute"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.ConcurrencyLimit != 3 || got.RateLimitPerMinute != 30 {
		t.Fatalf("policy response=%+v err=%v", got, err)
	}
}

func TestEndpointConcurrencyPolicyAcrossWorkers(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "endpoint concurrency")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	if _, err := pool.Exec(context.Background(), `UPDATE endpoints SET concurrency_limit=1 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		publishDeliveryEvent(t, server, key)
	}
	if count, err := delivery.DispatchBatch(context.Background(), pool, 10); err != nil || count != 2 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	var wg sync.WaitGroup
	claims := make(chan *delivery.Claimed, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := delivery.ClaimOneWithLimits(context.Background(), pool, "concurrent-worker", time.Minute, 100, 10)
			if err != nil {
				errors <- err
				return
			}
			if claim != nil {
				claims <- claim
			}
		}()
	}
	wg.Wait()
	close(claims)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if len(claims) != 1 {
		t.Fatalf("simultaneous claims=%d, want 1", len(claims))
	}
	first := <-claims
	if applied, err := delivery.Complete(context.Background(), pool, *first, delivery.Result{Outcome: "success", HTTPStatus: 204}); err != nil || !applied {
		t.Fatalf("complete applied=%v err=%v", applied, err)
	}
	if second, err := delivery.ClaimOneWithLimits(context.Background(), pool, "next-worker", time.Minute, 100, 10); err != nil || second == nil {
		t.Fatalf("claim after release=%+v err=%v", second, err)
	}
}

func TestEndpointRatePolicyPacesClaimsAcrossWorkers(t *testing.T) {
	pool, server, container := setupWithContainer(t)
	org, key := tenantKey(t, pool, "endpoint rate")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	if _, err := pool.Exec(context.Background(), `UPDATE endpoints SET concurrency_limit=2,rate_limit_per_minute=60 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		publishDeliveryEvent(t, server, key)
	}
	if count, err := delivery.DispatchBatch(context.Background(), pool, 10); err != nil || count != 2 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	first, err := delivery.ClaimOneWithLimits(context.Background(), pool, "worker-a", time.Minute, 100, 10)
	if err != nil || first == nil {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	url, err := container.ConnectionString(context.Background(), "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	otherPool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer otherPool.Close()
	if blocked, err := delivery.ClaimOneWithLimits(context.Background(), otherPool, "worker-b", time.Minute, 100, 10); err != nil || blocked != nil {
		t.Fatalf("rate blocked=%+v err=%v", blocked, err)
	}
	var next time.Time
	if err := pool.QueryRow(context.Background(), `SELECT next_rate_at FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&next); err != nil || next.Before(time.Now()) {
		t.Fatalf("next rate slot=%s err=%v", next, err)
	}
	resp, body := request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, key, `{"rate_limit_per_minute":30}`)
	if resp.StatusCode != 200 {
		t.Fatalf("rate edit status=%d body=%s", resp.StatusCode, body)
	}
	var retained time.Time
	if err := pool.QueryRow(context.Background(), `SELECT next_rate_at FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&retained); err != nil || !retained.Equal(next) {
		t.Fatalf("policy edit must retain reserved slot: before=%s after=%s err=%v", next, retained, err)
	}
	// Moving the cursor back avoids a wall-clock sleep while proving that the
	// rate gate, rather than in-flight capacity, held the second attempt.
	if _, err := pool.Exec(context.Background(), `UPDATE endpoint_circuits SET next_rate_at=clock_timestamp()-interval '1 second' WHERE endpoint_id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	if second, err := delivery.ClaimOneWithLimits(context.Background(), otherPool, "worker-b", time.Minute, 100, 10); err != nil || second == nil || second.RateLimitPerMinute != 30 {
		t.Fatalf("after rate slot=%+v err=%v", second, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT next_rate_at FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&next); err != nil || next.Before(time.Now().Add(1500*time.Millisecond)) {
		t.Fatalf("new rate spacing=%s err=%v", next, err)
	}
}

func TestEndpointRatePolicyWithReceiverRetryAfter(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "receiver backpressure")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	if _, err := pool.Exec(context.Background(), `UPDATE endpoints SET rate_limit_per_minute=60 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, server, key)
	claim := dispatchAndClaim(t, pool, "rate-retry-worker")
	result := testSender(pool).Send(context.Background(), *claim)
	if result.HTTPStatus != 429 || !result.Retryable || result.RetryAfter < 89*time.Second {
		t.Fatalf("receiver backpressure result=%+v", result)
	}
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("complete 429: applied=%v err=%v", applied, err)
	}
	var status string
	var nextAttempt time.Time
	var nextRate time.Time
	if err := pool.QueryRow(context.Background(), `SELECT d.status,d.next_attempt_at,ec.next_rate_at FROM deliveries d JOIN endpoint_circuits ec ON ec.endpoint_id=d.endpoint_id WHERE d.id=$1`, claim.ID).Scan(&status, &nextAttempt, &nextRate); err != nil {
		t.Fatal(err)
	}
	if status != "retry_wait" || nextAttempt.Before(time.Now().Add(88*time.Second)) || nextRate.IsZero() {
		t.Fatalf("429 schedule: status=%s next_attempt=%s next_rate=%s", status, nextAttempt, nextRate)
	}
}
