package integration_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
)

func TestEndpointSSRFRegistrationVerificationAndDelivery(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "ssrf-check")
	invalid := []string{
		"not-a-url",
		"http://8.8.8.8/hook",
		"https://127.0.0.1/hook",
		"https://[::1]/hook",
		"https://[::ffff:127.0.0.1]/hook",
		"https://10.0.0.1/hook",
		"https://169.254.169.254/latest/meta-data/",
		"https://[fe80::1]/hook",
		"https://user:pass@8.8.8.8/hook",
		"https://8.8.8.8:8443/hook",
		"https://8.8.8.8/hook#fragment",
	}
	for _, destination := range invalid {
		resp, body := request(t, http.MethodPost, server.URL+"/v1/endpoints", key, `{"url":"`+destination+`","event_types":["order.created"]}`)
		if resp.StatusCode != http.StatusUnprocessableEntity || !bytes.Contains(body, []byte(`"code":"invalid_endpoint"`)) || strings.Contains(string(body), destination) {
			t.Fatalf("registration accepted or disclosed destination %q: status=%d", destination, resp.StatusCode)
		}
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM endpoints WHERE organization_id=$1`, org).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid endpoints stored=%d err=%v", count, err)
	}

	var receiverCalls atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receiverCalls.Add(1)
		w.Header().Set("Webhook-Verification-Response", r.Header.Get("Webhook-Verification-Challenge"))
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	localServer := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test", AllowHTTPTest: true}).Router())
	defer localServer.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	for _, destination := range []string{"https://127.0.0.1/hook", "https://169.254.169.254/latest/meta-data/", "https://10.0.0.1/hook"} {
		if _, err := pool.Exec(ctx, `UPDATE endpoints SET url=$2,verification_required=true,verified_at=NULL WHERE id=$1`, endpointID, destination); err != nil {
			t.Fatal(err)
		}
		resp, body := request(t, http.MethodPost, localServer.URL+"/v1/endpoints/"+endpointID+"/verify", key, "")
		if resp.StatusCode != http.StatusUnprocessableEntity || !bytes.Contains(body, []byte(`"code":"destination_blocked"`)) || strings.Contains(string(body), destination) {
			t.Fatalf("verification accepted or disclosed destination %q: status=%d", destination, resp.StatusCode)
		}
		// A previously verified endpoint could have changed DNS or destination.
		// Force the old verification state to prove the sender blocks it again.
		if _, err := pool.Exec(ctx, `UPDATE endpoints SET verified_at=now() WHERE id=$1`, endpointID); err != nil {
			t.Fatal(err)
		}
		publishDeliveryEvent(t, localServer, key)
		claim := dispatchAndClaim(t, pool, "ssrf-worker")
		result := testSender(pool).Send(ctx, *claim)
		if result.Outcome != "security_blocked" || result.Retryable {
			t.Fatalf("unsafe send result=%+v", result)
		}
		if applied, err := delivery.Complete(ctx, pool, *claim, result); err != nil || !applied {
			t.Fatalf("unsafe send completion=%v err=%v", applied, err)
		}
	}
	if receiverCalls.Load() != 0 {
		t.Fatalf("unsafe destination reached receiver %d times", receiverCalls.Load())
	}

	if _, err := pool.Exec(ctx, `UPDATE endpoints SET url=$2,verified_at=NULL WHERE id=$1`, endpointID, receiver.URL); err != nil {
		t.Fatal(err)
	}
	resp, body := request(t, http.MethodPost, localServer.URL+"/v1/endpoints/"+endpointID+"/verify", key, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid local fixture verification=%d body=%s", resp.StatusCode, body)
	}
	publishDeliveryEvent(t, localServer, key)
	claim := dispatchAndClaim(t, pool, "ssrf-safe-worker")
	result := testSender(pool).Send(ctx, *claim)
	if result.Outcome != "success" {
		t.Fatalf("valid local fixture send=%+v", result)
	}
	if applied, err := delivery.Complete(ctx, pool, *claim, result); err != nil || !applied {
		t.Fatalf("valid local fixture completion=%v err=%v", applied, err)
	}
	if receiverCalls.Load() != 2 { // Verification GET and webhook POST.
		t.Fatalf("valid receiver calls=%d", receiverCalls.Load())
	}
}
