package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestTenantSchedulingAndEndpointCap(t *testing.T) {
	pool, apiServer := setup(t)
	orgA, keyA := tenantKey(t, pool, "busy tenant")
	orgB, keyB := tenantKey(t, pool, "quiet tenant")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointA, _ := addDeliveryEndpoint(t, pool, orgA, receiver.URL)
	addDeliveryEndpoint(t, pool, orgB, receiver.URL)
	for i := 0; i < 3; i++ {
		publishDeliveryEvent(t, apiServer, keyA)
	}
	publishDeliveryEvent(t, apiServer, keyB)
	ctx := context.Background()
	if count, err := delivery.DispatchBatch(ctx, pool, 10); err != nil || count != 4 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET next_attempt_at=now()-interval '2 minutes' WHERE organization_id=$1`, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET next_attempt_at=now()-interval '1 minute' WHERE organization_id=$1`, orgB); err != nil {
		t.Fatal(err)
	}
	first, err := delivery.ClaimOneWithLimits(ctx, pool, "first", time.Minute, 2, 2)
	if err != nil || first == nil || first.OrganizationID != orgA {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	second, err := delivery.ClaimOneWithLimits(ctx, pool, "second", time.Minute, 2, 2)
	if err != nil || second == nil || second.OrganizationID != orgB {
		t.Fatalf("fair second claim=%+v err=%v", second, err)
	}
	third, err := delivery.ClaimOneWithLimits(ctx, pool, "third", time.Minute, 2, 2)
	if err != nil || third == nil || third.OrganizationID != orgA {
		t.Fatalf("third claim=%+v err=%v", third, err)
	}
	if blocked, err := delivery.ClaimOneWithLimits(ctx, pool, "blocked", time.Minute, 2, 2); err != nil || blocked != nil {
		t.Fatalf("tenant cap claim=%+v err=%v", blocked, err)
	}
	var endpointActive, tenantActive int
	if err := pool.QueryRow(ctx, `SELECT active_claims FROM endpoint_circuits WHERE endpoint_id=$1`, endpointA).Scan(&endpointActive); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT active_claims FROM organization_scheduling WHERE organization_id=$1`, orgA).Scan(&tenantActive); err != nil {
		t.Fatal(err)
	}
	if endpointActive != 2 || tenantActive != 2 {
		t.Fatalf("active claims endpoint=%d tenant=%d", endpointActive, tenantActive)
	}
	if applied, err := delivery.Complete(ctx, pool, *first, delivery.Result{Outcome: "success", HTTPStatus: 204}); err != nil || !applied {
		t.Fatalf("complete first: applied=%v err=%v", applied, err)
	}
	if next, err := delivery.ClaimOneWithLimits(ctx, pool, "after-complete", time.Minute, 2, 2); err != nil || next == nil || next.OrganizationID != orgA {
		t.Fatalf("claim after capacity released=%+v err=%v", next, err)
	}
}

func TestExpiredLeaseReleasesEndpointAndTenantCapacity(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "capacity reclaim")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	for i := 0; i < 2; i++ {
		publishDeliveryEvent(t, apiServer, key)
	}
	ctx := context.Background()
	if count, err := delivery.DispatchBatch(ctx, pool, 10); err != nil || count != 2 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	first, err := delivery.ClaimOneWithLimits(ctx, pool, "first", time.Minute, 1, 1)
	if err != nil || first == nil {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	if blocked, err := delivery.ClaimOneWithLimits(ctx, pool, "blocked", time.Minute, 1, 1); err != nil || blocked != nil {
		t.Fatalf("endpoint cap claim=%+v err=%v", blocked, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := delivery.ReclaimExpired(ctx, pool, 10); err != nil || count != 1 {
		t.Fatalf("reclaim count=%d err=%v", count, err)
	}
	var endpointActive, tenantActive int
	if err := pool.QueryRow(ctx, `SELECT active_claims FROM endpoint_circuits WHERE endpoint_id=$1`, endpointID).Scan(&endpointActive); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT active_claims FROM organization_scheduling WHERE organization_id=$1`, org).Scan(&tenantActive); err != nil {
		t.Fatal(err)
	}
	if endpointActive != 0 || tenantActive != 0 {
		t.Fatalf("capacity after reclaim endpoint=%d tenant=%d", endpointActive, tenantActive)
	}
	if next, err := delivery.ClaimOneWithLimits(ctx, pool, "after-reclaim", time.Minute, 1, 1); err != nil || next == nil {
		t.Fatalf("claim after reclaim=%+v err=%v", next, err)
	}
}
