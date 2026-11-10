package integration_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestEndpointCircuitOpensAndAllowsOneRecoveryProbe(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "circuit")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		publishDeliveryEvent(t, apiServer, key)
		claim := dispatchAndClaim(t, pool, "worker")
		applied, err := delivery.Complete(ctx, pool, *claim, delivery.Result{Outcome: "http_error", HTTPStatus: 503, Retryable: true})
		if err != nil || !applied {
			t.Fatalf("failure %d: applied=%v err=%v", i, applied, err)
		}
	}
	var streak int
	var openUntil time.Time
	if err := pool.QueryRow(ctx, `SELECT failure_streak,open_until FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&streak, &openUntil); err != nil || streak != 3 || !openUntil.After(time.Now()) {
		t.Fatalf("circuit streak=%d open_until=%s err=%v", streak, openUntil, err)
	}
	resp, body := request(t, http.MethodGet, apiServer.URL+"/v1/endpoints/"+endpointID, key, "")
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte(`"circuit_status":"open"`)) {
		t.Fatalf("endpoint circuit status=%d body=%s", resp.StatusCode, body)
	}
	for i := 0; i < 2; i++ {
		publishDeliveryEvent(t, apiServer, key)
	}
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 2 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	if claim, err := delivery.ClaimOne(ctx, pool, "blocked", time.Minute); err != nil || claim != nil {
		t.Fatalf("claim while open=%+v err=%v", claim, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE endpoint_circuits SET open_until=now()-interval '1 second' WHERE endpoint_id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	claims := make(chan *delivery.Claimed, 2)
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := delivery.ClaimOne(ctx, pool, "probe", time.Minute)
			if err != nil {
				errCh <- err
			} else if claim != nil {
				claims <- claim
			}
		}()
	}
	wg.Wait()
	close(claims)
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if len(claims) != 1 {
		t.Fatalf("half-open probes=%d; want one", len(claims))
	}
	probe := <-claims
	if applied, err := delivery.Complete(ctx, pool, *probe, delivery.Result{Outcome: "success", HTTPStatus: 204}); err != nil || !applied {
		t.Fatalf("probe completion: applied=%v err=%v", applied, err)
	}
	var open *time.Time
	if err := pool.QueryRow(ctx, `SELECT failure_streak,open_until FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&streak, &open); err != nil || streak != 0 || open != nil {
		t.Fatalf("closed circuit streak=%d open=%v err=%v", streak, open, err)
	}
	if claim, err := delivery.ClaimOne(ctx, pool, "after-probe", time.Minute); err != nil || claim == nil {
		t.Fatalf("claim after successful probe=%+v err=%v", claim, err)
	}
}

func TestFailedAndAbandonedProbesDoNotLeaveCircuitStuck(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "probe recovery")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		publishDeliveryEvent(t, apiServer, key)
	}
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 2 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE endpoint_circuits SET failure_streak=3,open_until=now()-interval '1 second' WHERE endpoint_id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	first, err := delivery.ClaimOne(ctx, pool, "probe-1", time.Minute)
	if err != nil || first == nil {
		t.Fatalf("first probe=%+v err=%v", first, err)
	}
	if applied, err := delivery.Complete(ctx, pool, *first, delivery.Result{Outcome: "http_error", HTTPStatus: 503, Retryable: true}); err != nil || !applied {
		t.Fatalf("failed probe completion: applied=%v err=%v", applied, err)
	}
	if claim, err := delivery.ClaimOne(ctx, pool, "blocked", time.Minute); err != nil || claim != nil {
		t.Fatalf("claim while reopened=%+v err=%v", claim, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE endpoint_circuits SET open_until=now()-interval '1 second' WHERE endpoint_id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	second, err := delivery.ClaimOne(ctx, pool, "probe-2", time.Minute)
	if err != nil || second == nil {
		t.Fatalf("second probe=%+v err=%v", second, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := delivery.ReclaimExpired(ctx, pool, 10); err != nil || count != 1 {
		t.Fatalf("reclaim count=%d err=%v", count, err)
	}
	var probeID *string
	if err := pool.QueryRow(ctx, `SELECT probe_delivery_id FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&probeID); err != nil || probeID != nil {
		t.Fatalf("abandoned probe marker=%v err=%v", probeID, err)
	}
	if claim, err := delivery.ClaimOne(ctx, pool, "probe-3", time.Minute); err != nil || claim == nil {
		t.Fatalf("probe after reclaim=%+v err=%v", claim, err)
	}
}
