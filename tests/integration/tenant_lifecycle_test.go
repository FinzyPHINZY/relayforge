package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/tenant"
)

func TestTenantSuspendExportAndPurge(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "tenant-to-remove")
	otherOrg, _ := tenantKey(t, pool, "tenant-to-keep")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	ctx := context.Background()
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	claim, err := delivery.ClaimOne(ctx, pool, "tenant-test-worker", 45*time.Second)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	result := testSender(pool).Send(ctx, *claim)
	if applied, err := delivery.Complete(ctx, pool, *claim, result); err != nil || !applied {
		t.Fatalf("completion=%v err=%v", applied, err)
	}
	replayRequest(t, apiServer.URL, key, claim.ID, "tenant-test-replay")
	publishDeliveryEvent(t, apiServer, key)
	if err := tenant.Suspend(ctx, pool, org); err != nil {
		t.Fatal(err)
	}
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 0 {
		t.Fatalf("suspended tenant dispatched: %d, %v", count, err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE organization_id=$1 AND status='pending'`, org).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending outbox=%d err=%v", pending, err)
	}
	path := filepath.Join(t.TempDir(), "tenant.jsonl")
	if err := tenant.Export(ctx, pool, org, path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"table":"events"`) || strings.Contains(string(b), `"key_hash"`) || strings.Contains(string(b), `"ciphertext"`) {
		t.Fatal("export omitted events or exposed credential material")
	}
	if err := tenant.Purge(ctx, pool, org, path); err != nil {
		t.Fatal(err)
	}
	var gone, kept int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id=$1`, org).Scan(&gone); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id=$1`, otherOrg).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if gone != 0 || kept != 1 {
		t.Fatalf("deleted=%d other=%d", gone, kept)
	}
}

func TestSuspendedTenantCannotClaimAndWrongExportCannotPurge(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "suspended-claims")
	otherOrg, _ := tenantKey(t, pool, "other-export")
	addDeliveryEndpoint(t, pool, org, "https://example.com/webhooks")
	publishDeliveryEvent(t, apiServer, key)
	ctx := context.Background()
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	if err := tenant.Suspend(ctx, pool, org); err != nil {
		t.Fatal(err)
	}
	if claim, err := delivery.ClaimOne(ctx, pool, "suspended-worker", 45*time.Second); err != nil || claim != nil {
		t.Fatalf("suspended tenant claimed: %+v, %v", claim, err)
	}
	if err := tenant.Suspend(ctx, pool, otherOrg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "other.jsonl")
	if err := tenant.Export(ctx, pool, otherOrg, path); err != nil {
		t.Fatal(err)
	}
	if err := tenant.Purge(ctx, pool, org, path); err == nil {
		t.Fatal("purged organization with another tenant's export")
	}
}

func TestTenantExportWaitsForActiveAttempt(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "tenant-active-attempt")
	addDeliveryEndpoint(t, pool, org, "https://example.com/webhooks")
	publishDeliveryEvent(t, apiServer, key)
	ctx := context.Background()
	if count, err := delivery.DispatchBatch(ctx, pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	if claim, err := delivery.ClaimOne(ctx, pool, "tenant-active-worker", 45*time.Second); err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if err := tenant.Suspend(ctx, pool, org); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "must-not-exist.jsonl")
	if err := tenant.Export(ctx, pool, org, path); err == nil {
		t.Fatal("exported while an HTTP attempt could still be in flight")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected export file: %v", err)
	}
}
