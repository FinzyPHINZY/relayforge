package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/health"
)

func replayRequest(t *testing.T, url, key, source, requestKey string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/v1/deliveries/"+source+"/replay", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Idempotency-Key", requestKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		ID        string `json:"id"`
		RunNumber int    `json:"run_number"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 || out.ID == "" {
		t.Fatalf("replay status=%d body=%+v", resp.StatusCode, out)
	}
	return out.ID
}

func TestDeliveryHistoryReplayAndTenantIsolation(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "recovery owner")
	_, otherKey := tenantKey(t, pool, "other tenant")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	eventID := publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "worker-1")
	result := testSender(pool).Send(context.Background(), *claim)
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("completion applied=%v err=%v", applied, err)
	}
	resp, body := request(t, "GET", apiServer.URL+"/v1/deliveries/"+claim.ID, key, "")
	if resp.StatusCode != 200 {
		t.Fatalf("detail status=%d body=%s", resp.StatusCode, body)
	}
	var detail struct {
		Delivery struct {
			Status    string `json:"status"`
			RunNumber int    `json:"run_number"`
		} `json:"delivery"`
		Attempts []struct {
			Outcome    string `json:"outcome"`
			HTTPStatus int    `json:"http_status"`
		} `json:"attempts"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Delivery.Status != "exhausted" || detail.Delivery.RunNumber != 1 || len(detail.Attempts) != 1 || detail.Attempts[0].HTTPStatus != 400 {
		t.Fatalf("detail=%+v", detail)
	}
	resp, _ = request(t, "GET", apiServer.URL+"/v1/deliveries/"+claim.ID, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant detail status=%d", resp.StatusCode)
	}
	foreignReplay, err := http.NewRequest(http.MethodPost, apiServer.URL+"/v1/deliveries/"+claim.ID+"/replay", nil)
	if err != nil {
		t.Fatal(err)
	}
	foreignReplay.Header.Set("Authorization", "Bearer "+otherKey)
	foreignReplay.Header.Set("Idempotency-Key", "foreign")
	foreignResponse, err := http.DefaultClient.Do(foreignReplay)
	if err != nil {
		t.Fatal(err)
	}
	foreignResponse.Body.Close()
	if foreignResponse.StatusCode != 404 {
		t.Fatalf("cross-tenant replay status=%d", foreignResponse.StatusCode)
	}
	resp, body = request(t, "GET", apiServer.URL+"/v1/deliveries?event_id="+eventID+"&endpoint_id="+endpointID+"&status=exhausted", key, "")
	if resp.StatusCode != 200 {
		t.Fatalf("list status=%d body=%s", resp.StatusCode, body)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil || len(list.Data) != 1 || list.Data[0].ID != claim.ID {
		t.Fatalf("filtered list=%+v err=%v", list, err)
	}
	newReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer newReceiver.Close()
	if _, err := pool.Exec(context.Background(), `UPDATE endpoints SET url=$2 WHERE id=$1`, endpointID, newReceiver.URL); err != nil {
		t.Fatal(err)
	}
	replayID := replayRequest(t, apiServer.URL, key, claim.ID, "repair-1")
	if repeated := replayRequest(t, apiServer.URL, key, claim.ID, "repair-1"); repeated != replayID {
		t.Fatalf("idempotent replay IDs %s and %s", replayID, repeated)
	}
	resp, _ = request(t, "GET", apiServer.URL+"/v1/deliveries/"+replayID, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant replay status=%d", resp.StatusCode)
	}
	var run int
	var source, status string
	err = pool.QueryRow(context.Background(), `SELECT run_number,replayed_from_delivery_id,status FROM deliveries WHERE id=$1`, replayID).Scan(&run, &source, &status)
	if err != nil || run != 2 || source != claim.ID || status != "pending" {
		t.Fatalf("replay run=%d source=%s status=%s err=%v", run, source, status, err)
	}
	var originalURL, replayURL string
	if err := pool.QueryRow(context.Background(), `SELECT destination_url FROM deliveries WHERE id=$1`, claim.ID).Scan(&originalURL); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT destination_url FROM deliveries WHERE id=$1`, replayID).Scan(&replayURL); err != nil {
		t.Fatal(err)
	}
	if originalURL != receiver.URL || replayURL != newReceiver.URL {
		t.Fatalf("destination snapshots original=%s replay=%s", originalURL, replayURL)
	}
	resp, body = request(t, "GET", apiServer.URL+"/v1/metrics/summary", key, "")
	if resp.StatusCode != 200 {
		t.Fatalf("metrics status=%d body=%s", resp.StatusCode, body)
	}
	var metrics struct {
		DeliveriesExhausted int `json:"deliveries_exhausted"`
		DeliveriesPending   int `json:"deliveries_pending"`
		DeliveryAttempts    int `json:"delivery_attempts"`
	}
	if err := json.Unmarshal(body, &metrics); err != nil || metrics.DeliveriesExhausted != 1 || metrics.DeliveriesPending != 1 || metrics.DeliveryAttempts != 1 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
	recorder := httptest.NewRecorder()
	health.Handler(pool).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "webhook_deliveries_exhausted 1") {
		t.Fatalf("operator metrics status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	ready := httptest.NewRecorder()
	health.Handler(pool).ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != 200 {
		t.Fatalf("worker readiness status=%d", ready.Code)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE api_keys SET scopes=ARRAY['events:read'] WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, "GET", apiServer.URL+"/v1/deliveries", key, "")
	if resp.StatusCode != 403 {
		t.Fatalf("delivery scope status=%d", resp.StatusCode)
	}
	resp, _ = request(t, "GET", apiServer.URL+"/v1/metrics/summary", key, "")
	if resp.StatusCode != 403 {
		t.Fatalf("metrics scope status=%d", resp.StatusCode)
	}
}

func TestSecretRotationAndEndpointPause(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "rotation")
	received := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("webhook-signature")
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	endpointID, oldSecret := addDeliveryEndpoint(t, pool, org, receiver.URL)
	var oldID string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM endpoint_secrets WHERE endpoint_id=$1 AND state='active'`, endpointID).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	resp, body := request(t, "POST", apiServer.URL+"/v1/endpoints/"+endpointID+"/rotate-secret", key, `{"overlap_seconds":300}`)
	if resp.StatusCode != 201 {
		t.Fatalf("rotation status=%d body=%s", resp.StatusCode, body)
	}
	var rotation struct {
		SigningSecret            string    `json:"signing_secret"`
		SigningSecretID          string    `json:"signing_secret_id"`
		PreviousSigningSecretID  string    `json:"previous_signing_secret_id"`
		PreviousSecretValidUntil time.Time `json:"previous_secret_valid_until"`
	}
	if err := json.Unmarshal(body, &rotation); err != nil {
		t.Fatal(err)
	}
	if rotation.SigningSecret == "" || rotation.SigningSecret == oldSecret || rotation.PreviousSigningSecretID != oldID || time.Until(rotation.PreviousSecretValidUntil) < time.Minute {
		t.Fatalf("rotation metadata=%+v", rotation)
	}
	var state string
	var overlap time.Time
	if err := pool.QueryRow(context.Background(), `SELECT state,overlap_until FROM endpoint_secrets WHERE id=$1`, oldID).Scan(&state, &overlap); err != nil || state != "overlap" {
		t.Fatalf("old secret state=%s err=%v", state, err)
	}
	resp, body = request(t, "GET", apiServer.URL+"/v1/endpoints/"+endpointID, key, "")
	if resp.StatusCode != 200 || strings.Contains(string(body), rotation.SigningSecret) || !strings.Contains(string(body), rotation.SigningSecretID) {
		t.Fatalf("endpoint metadata status=%d body=%s", resp.StatusCode, body)
	}
	publishDeliveryEvent(t, apiServer, key)
	count, err := delivery.DispatchBatch(context.Background(), pool, 50)
	if err != nil || count != 1 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	resp, body = request(t, "PATCH", apiServer.URL+"/v1/endpoints/"+endpointID, key, `{"status":"paused"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("pause status=%d body=%s", resp.StatusCode, body)
	}
	claim, err := delivery.ClaimOne(context.Background(), pool, "worker-1", 45*time.Second)
	if err != nil || claim != nil {
		t.Fatalf("paused claim=%+v err=%v", claim, err)
	}
	resp, body = request(t, "PATCH", apiServer.URL+"/v1/endpoints/"+endpointID, key, `{"status":"active"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("resume status=%d body=%s", resp.StatusCode, body)
	}
	claim, err = delivery.ClaimOne(context.Background(), pool, "worker-1", 45*time.Second)
	if err != nil || claim == nil {
		t.Fatalf("resumed claim=%+v err=%v", claim, err)
	}
	result := testSender(pool).Send(context.Background(), *claim)
	if result.Outcome != "success" {
		t.Fatalf("send=%+v", result)
	}
	if signature := <-received; signature == "" || !strings.HasPrefix(signature, "v1,") {
		t.Fatalf("signature=%q", signature)
	}
	var signingID string
	if err := pool.QueryRow(context.Background(), `SELECT signing_secret_id FROM delivery_attempts WHERE id=$1`, claim.AttemptID).Scan(&signingID); err != nil || signingID != rotation.SigningSecretID {
		t.Fatalf("attempt signing secret=%s err=%v", signingID, err)
	}
}
