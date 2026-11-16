package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"webhook_delivery/internal/delivery"
)

func TestEndpointManagementPolicySnapshotsAndPermissions(t *testing.T) {
	pool, server := setup(t)
	org, ownerKey := tenantKey(t, pool, "endpoint-management")
	_, otherKey := tenantKey(t, pool, "endpoint-management-other")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE api_keys SET scopes=array_append(scopes,'team:write') WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	firstEvent := publishDeliveryEvent(t, server, ownerKey)
	if n, err := delivery.DispatchBatch(ctx, pool, 1); err != nil || n != 1 {
		t.Fatalf("first dispatch=%d err=%v", n, err)
	}
	for _, invalid := range []string{`{"timeout_ms":999}`, `{"timeout_ms":30001}`, `{"max_attempts":0}`, `{"max_attempts":9}`} {
		resp, body := request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, ownerKey, invalid)
		if resp.StatusCode != 422 {
			t.Fatalf("invalid policy %s status=%d body=%s", invalid, resp.StatusCode, body)
		}
	}
	resp, _ := request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, otherKey, `{"timeout_ms":2500}`)
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant status=%d", resp.StatusCode)
	}
	resp, body := request(t, http.MethodPost, server.URL+"/v1/team/members", ownerKey, `{"name":"Viewer","email":"endpoint-viewer@example.test","role":"viewer"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("viewer creation status=%d body=%s", resp.StatusCode, body)
	}
	var member struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &member); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, member.APIKey, `{"timeout_ms":2500}`)
	if resp.StatusCode != 403 {
		t.Fatalf("viewer patch status=%d", resp.StatusCode)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, ownerKey, `{"timeout_ms":2500,"max_attempts":3,"concurrency_limit":4,"rate_limit_per_minute":120}`)
	if resp.StatusCode != 200 {
		t.Fatalf("patch status=%d body=%s", resp.StatusCode, body)
	}
	var policy struct {
		TimeoutMS          int `json:"timeout_ms"`
		MaxAttempts        int `json:"max_attempts"`
		ConcurrencyLimit   int `json:"concurrency_limit"`
		RateLimitPerMinute int `json:"rate_limit_per_minute"`
	}
	if err := json.Unmarshal(body, &policy); err != nil || policy.TimeoutMS != 2500 || policy.MaxAttempts != 3 || policy.ConcurrencyLimit != 4 || policy.RateLimitPerMinute != 120 {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
	secondEvent := publishDeliveryEvent(t, server, ownerKey)
	if n, err := delivery.DispatchBatch(ctx, pool, 1); err != nil || n != 1 {
		t.Fatalf("second dispatch=%d err=%v", n, err)
	}
	for _, item := range []struct {
		eventID  string
		timeout  int
		attempts int
	}{{firstEvent, 10000, 8}, {secondEvent, 2500, 3}} {
		var timeout, attempts int
		if err := pool.QueryRow(ctx, `SELECT timeout_ms,max_attempts FROM deliveries WHERE event_id=$1 AND organization_id=$2`, item.eventID, org).Scan(&timeout, &attempts); err != nil || timeout != item.timeout || attempts != item.attempts {
			t.Fatalf("delivery %s timeout=%d attempts=%d err=%v", item.eventID, timeout, attempts, err)
		}
	}
}
