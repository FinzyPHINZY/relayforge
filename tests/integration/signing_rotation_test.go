package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/security"
)

func TestRotatedSignatureAndExpiredSecretErasure(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "signing-rotation")
	type receipt struct {
		id, signature, timestamp string
		body                     []byte
	}
	received := make(chan receipt, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- receipt{r.Header.Get("webhook-id"), r.Header.Get("webhook-signature"), r.Header.Get("webhook-timestamp"), body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	endpointID, oldSecret := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	var oldID string
	if err := pool.QueryRow(ctx, `SELECT id FROM endpoint_secrets WHERE endpoint_id=$1 AND state='active'`, endpointID).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	firstEvent := publishDeliveryEvent(t, apiServer, key)
	first := dispatchAndClaim(t, pool, "signing-before-rotation")
	firstResult := testSender(pool).Send(ctx, *first)
	if applied, err := delivery.Complete(ctx, pool, *first, firstResult); err != nil || !applied || firstResult.Outcome != "success" {
		t.Fatalf("first delivery=%+v applied=%v err=%v", firstResult, applied, err)
	}
	firstReceipt := <-received
	if first.ID != firstReceipt.id || first.EventID != firstEvent || !matchesSignature(oldSecret, firstReceipt) {
		t.Fatal("first delivery signature or identity invalid")
	}

	resp, body := request(t, http.MethodPost, apiServer.URL+"/v1/endpoints/"+endpointID+"/rotate-secret", key, `{"overlap_seconds":300}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("rotate status=%d body=%s", resp.StatusCode, body)
	}
	var rotation struct {
		Secret     string    `json:"signing_secret"`
		ID         string    `json:"signing_secret_id"`
		PreviousID string    `json:"previous_signing_secret_id"`
		ValidUntil time.Time `json:"previous_secret_valid_until"`
	}
	if err := json.Unmarshal(body, &rotation); err != nil || rotation.Secret == "" || rotation.Secret == oldSecret || rotation.ID == oldID || rotation.PreviousID != oldID || !rotation.ValidUntil.After(time.Now()) {
		t.Fatalf("rotation metadata invalid: err=%v", err)
	}
	var oldState string
	var oldCiphertext []byte
	if err := pool.QueryRow(ctx, `SELECT state,ciphertext FROM endpoint_secrets WHERE id=$1`, oldID).Scan(&oldState, &oldCiphertext); err != nil || oldState != "overlap" || len(oldCiphertext) == 0 {
		t.Fatalf("old secret overlap state=%s ciphertext=%d err=%v", oldState, len(oldCiphertext), err)
	}
	resp, body = request(t, http.MethodGet, apiServer.URL+"/v1/endpoints/"+endpointID, key, "")
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body), oldSecret) || strings.Contains(string(body), rotation.Secret) {
		t.Fatalf("secret disclosed after rotation status=%d", resp.StatusCode)
	}

	publishDeliveryEvent(t, apiServer, key)
	second := dispatchAndClaim(t, pool, "signing-after-rotation")
	secondResult := testSender(pool).Send(ctx, *second)
	if applied, err := delivery.Complete(ctx, pool, *second, secondResult); err != nil || !applied || secondResult.Outcome != "success" {
		t.Fatalf("second delivery=%+v applied=%v err=%v", secondResult, applied, err)
	}
	secondReceipt := <-received
	if second.ID != secondReceipt.id || !matchesSignature(rotation.Secret, secondReceipt) || matchesSignature(oldSecret, secondReceipt) {
		t.Fatal("rotation did not replace the signing key")
	}
	var signingID string
	if err := pool.QueryRow(ctx, `SELECT signing_secret_id FROM delivery_attempts WHERE id=$1`, second.AttemptID).Scan(&signingID); err != nil || signingID != rotation.ID {
		t.Fatalf("attempt signing secret=%s err=%v", signingID, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE endpoint_secrets SET overlap_until=now()-interval '1 second' WHERE id=$1`, oldID); err != nil {
		t.Fatal(err)
	}
	if count, err := delivery.RetireExpiredSecrets(ctx, pool); err != nil || count != 1 {
		t.Fatalf("retired count=%d err=%v", count, err)
	}
	var nonce []byte
	if err := pool.QueryRow(ctx, `SELECT state,ciphertext,nonce FROM endpoint_secrets WHERE id=$1`, oldID).Scan(&oldState, &oldCiphertext, &nonce); err != nil || oldState != "retired" || len(oldCiphertext) != 0 || len(nonce) != 0 {
		t.Fatalf("retired secret state=%s ciphertext=%d nonce=%d err=%v", oldState, len(oldCiphertext), len(nonce), err)
	}
	resp, body = request(t, http.MethodGet, apiServer.URL+"/v1/endpoints/"+endpointID, key, "")
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body), oldSecret) || strings.Contains(string(body), rotation.Secret) || !strings.Contains(string(body), `"state":"retired"`) {
		t.Fatalf("retired metadata invalid status=%d", resp.StatusCode)
	}
}

func matchesSignature(secret string, got struct {
	id, signature, timestamp string
	body                     []byte
}) bool {
	timestamp, err := strconv.ParseInt(got.timestamp, 10, 64)
	return err == nil && got.signature == security.WebhookSignature(secret, timestamp, got.body)
}
