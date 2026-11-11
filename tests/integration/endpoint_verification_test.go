package integration_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
)

func TestEndpointVerificationGatesDelivery(t *testing.T) {
	pool, _ := setup(t)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Webhook-Verification-Response", r.Header.Get("Webhook-Verification-Challenge"))
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	server := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: bytes.Repeat([]byte{1}, 32), EncryptionKeyID: "test", AllowHTTPTest: true}).Router())
	defer server.Close()
	org, key := tenantKey(t, pool, "verification")
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL+"/hook")
	if _, err := pool.Exec(t.Context(), `UPDATE endpoints SET verification_required=true WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, server, key)
	if count, err := delivery.DispatchBatch(t.Context(), pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch=%d err=%v", count, err)
	}
	var before int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM deliveries WHERE endpoint_id=$1`, endpointID).Scan(&before); err != nil || before != 0 {
		t.Fatalf("before=%d err=%v", before, err)
	}
	resp, content := request(t, http.MethodPost, server.URL+"/v1/endpoints/"+endpointID+"/verify", key, "")
	if resp.StatusCode != 200 {
		t.Fatalf("verify status=%d body=%s", resp.StatusCode, content)
	}
	publishDeliveryEvent(t, server, key)
	if count, err := delivery.DispatchBatch(t.Context(), pool, 50); err != nil || count != 1 {
		t.Fatalf("dispatch after verify=%d err=%v", count, err)
	}
	var after int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM deliveries WHERE endpoint_id=$1`, endpointID).Scan(&after); err != nil || after != 1 {
		t.Fatalf("after=%d err=%v", after, err)
	}
}
