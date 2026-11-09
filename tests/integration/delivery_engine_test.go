package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

var testEncryptionKey = bytes.Repeat([]byte{1}, 32)

func addDeliveryEndpoint(t testing.TB, pool *pgxpool.Pool, org, url string) (string, string) {
	t.Helper()
	ctx := context.Background()
	endpointID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	secretID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	secret := "whsec_test_delivery_secret"
	ciphertext, nonce, err := security.Encrypt(testEncryptionKey, secret, endpointID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO endpoints (id,organization_id,url) VALUES ($1,$2,$3)`, endpointID, org, url)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO endpoint_secrets (id,organization_id,endpoint_id,ciphertext,nonce,key_id) VALUES ($1,$2,$3,$4,$5,'test')`, secretID, org, endpointID, ciphertext, nonce)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO subscriptions (organization_id,endpoint_id,event_type) VALUES ($1,$2,'order.created')`, org, endpointID)
	if err != nil {
		t.Fatal(err)
	}
	return endpointID, secret
}

func publishDeliveryEvent(t *testing.T, server *httptest.Server, key string) string {
	t.Helper()
	resp, body := request(t, "POST", server.URL+"/v1/events", key, `{"type":"order.created","data":{"order_id":"delivery-test"}}`)
	if resp.StatusCode != 202 {
		t.Fatalf("publish status=%d body=%s", resp.StatusCode, body)
	}
	var event struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	return event.ID
}

func dispatchAndClaim(t *testing.T, pool *pgxpool.Pool, worker string) *delivery.Claimed {
	t.Helper()
	ctx := context.Background()
	count, err := delivery.DispatchBatch(ctx, pool, 50)
	if err != nil || count != 1 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	claim, err := delivery.ClaimOne(ctx, pool, worker, 45*time.Second)
	if err != nil || claim == nil {
		t.Fatalf("claim=%v err=%v", claim, err)
	}
	return claim
}

func testSender(pool *pgxpool.Pool) delivery.Sender {
	return delivery.Sender{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test", AllowHTTPTest: true}
}

func TestSignedPublishToDelivery(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "signed delivery")
	type received struct {
		id, attempt, timestamp, signature string
		body                              []byte
	}
	requests := make(chan received, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- received{r.Header.Get("webhook-id"), r.Header.Get("webhook-attempt"), r.Header.Get("webhook-timestamp"), r.Header.Get("webhook-signature"), body}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	endpointID, secret := addDeliveryEndpoint(t, pool, org, receiver.URL)
	eventID := publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "worker-1")
	if claim.EventID != eventID || claim.EndpointID != endpointID || claim.DestinationURL != receiver.URL {
		t.Fatal("delivery snapshot mismatch")
	}
	result := testSender(pool).Send(context.Background(), *claim)
	if result.Outcome != "success" || result.HTTPStatus != 204 {
		t.Fatalf("send result=%+v", result)
	}
	applied, err := delivery.Complete(context.Background(), pool, *claim, result)
	if err != nil || !applied {
		t.Fatalf("complete applied=%v err=%v", applied, err)
	}
	got := <-requests
	if got.id != claim.ID || got.attempt != "1" {
		t.Fatalf("headers id=%q attempt=%q", got.id, got.attempt)
	}
	ts, err := strconv.ParseInt(got.timestamp, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if got.signature != security.WebhookSignature(secret, ts, got.body) {
		t.Fatal("signature did not cover transmitted bytes")
	}
	var status string
	var attempts int
	var secretID string
	err = pool.QueryRow(context.Background(), `SELECT d.status,d.attempt_count,a.signing_secret_id FROM deliveries d JOIN delivery_attempts a ON a.delivery_id=d.id WHERE d.id=$1`, claim.ID).Scan(&status, &attempts, &secretID)
	if err != nil || status != "succeeded" || attempts != 1 || secretID == "" {
		t.Fatalf("state=%s attempts=%d secret=%q err=%v", status, attempts, secretID, err)
	}
	count, err := delivery.DispatchBatch(context.Background(), pool, 50)
	if err != nil || count != 0 {
		t.Fatalf("duplicate dispatch count=%d err=%v", count, err)
	}
}

func TestRetryThenSuccess(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "retry")
	var mu sync.Mutex
	ids := []string{}
	attempts := []string{}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ids = append(ids, r.Header.Get("webhook-id"))
		attempts = append(attempts, r.Header.Get("webhook-attempt"))
		n := len(ids)
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	first := dispatchAndClaim(t, pool, "worker-1")
	r1 := testSender(pool).Send(context.Background(), *first)
	if r1.HTTPStatus != 503 || !r1.Retryable {
		t.Fatalf("first result=%+v", r1)
	}
	applied, err := delivery.Complete(context.Background(), pool, *first, r1)
	if err != nil || !applied {
		t.Fatalf("first completion: %v %v", applied, err)
	}
	_, err = pool.Exec(context.Background(), `UPDATE deliveries SET next_attempt_at=now() WHERE id=$1 AND status='retry_wait'`, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := delivery.ClaimOne(context.Background(), pool, "worker-2", 45*time.Second)
	if err != nil || second == nil || second.AttemptNumber != 2 {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	r2 := testSender(pool).Send(context.Background(), *second)
	applied, err = delivery.Complete(context.Background(), pool, *second, r2)
	if err != nil || !applied || r2.Outcome != "success" {
		t.Fatalf("second result=%+v applied=%v err=%v", r2, applied, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ids) != 2 || ids[0] != ids[1] || attempts[0] != "1" || attempts[1] != "2" {
		t.Fatalf("ids=%v attempts=%v", ids, attempts)
	}
}

func TestExpiredLeaseRejectsStaleCompletion(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "fencing")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	first := dispatchAndClaim(t, pool, "worker-1")
	_, err := pool.Exec(context.Background(), `UPDATE deliveries SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	count, err := delivery.ReclaimExpired(context.Background(), pool, 10)
	if err != nil || count != 1 {
		t.Fatalf("reclaim count=%d err=%v", count, err)
	}
	applied, err := delivery.Complete(context.Background(), pool, *first, delivery.Result{Outcome: "success", HTTPStatus: 204})
	if err != nil || applied {
		t.Fatalf("stale applied=%v err=%v", applied, err)
	}
	second, err := delivery.ClaimOne(context.Background(), pool, "worker-2", 45*time.Second)
	if err != nil || second == nil || second.LeaseGeneration <= first.LeaseGeneration {
		t.Fatalf("new claim=%+v err=%v", second, err)
	}
}

func TestConcurrentClaimHasOneOwner(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "concurrent")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	count, err := delivery.DispatchBatch(context.Background(), pool, 50)
	if err != nil || count != 1 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}
	var wg sync.WaitGroup
	claimed := make(chan *delivery.Claimed, 20)
	errorsCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c, e := delivery.ClaimOne(context.Background(), pool, "worker-"+strconv.Itoa(n), 45*time.Second)
			if e != nil {
				errorsCh <- e
			} else if c != nil {
				claimed <- c
			}
		}(i)
	}
	wg.Wait()
	close(claimed)
	close(errorsCh)
	for e := range errorsCh {
		t.Error(e)
	}
	if len(claimed) != 1 {
		t.Fatalf("simultaneous owners=%d", len(claimed))
	}
}

func TestReceiverAcceptedBeforeWorkerCrashCanReceiveDuplicate(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "crash after send")
	var mu sync.Mutex
	var ids []string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ids = append(ids, r.Header.Get("webhook-id"))
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	first := dispatchAndClaim(t, pool, "worker-1")
	if result := testSender(pool).Send(context.Background(), *first); result.Outcome != "success" {
		t.Fatalf("first send: %+v", result)
	}
	// The receiver accepted the request, but the worker died before Complete.
	_, err := pool.Exec(context.Background(), `UPDATE deliveries SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := delivery.ReclaimExpired(context.Background(), pool, 10); err != nil || count != 1 {
		t.Fatalf("reclaimed=%d err=%v", count, err)
	}
	second, err := delivery.ClaimOne(context.Background(), pool, "worker-2", 45*time.Second)
	if err != nil || second == nil || second.AttemptNumber != 2 {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	result := testSender(pool).Send(context.Background(), *second)
	if applied, err := delivery.Complete(context.Background(), pool, *second, result); err != nil || !applied || result.Outcome != "success" {
		t.Fatalf("second completion result=%+v applied=%v err=%v", result, applied, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ids) != 2 || ids[0] != first.ID || ids[1] != first.ID {
		t.Fatalf("receiver IDs=%v", ids)
	}
	var firstOutcome, secondOutcome string
	err = pool.QueryRow(context.Background(), `SELECT a.outcome,b.outcome FROM delivery_attempts a JOIN delivery_attempts b ON b.delivery_id=a.delivery_id AND b.attempt_number=2 WHERE a.delivery_id=$1 AND a.attempt_number=1`, first.ID).Scan(&firstOutcome, &secondOutcome)
	if err != nil || firstOutcome != "unknown" || secondOutcome != "success" {
		t.Fatalf("attempt history=%s,%s err=%v", firstOutcome, secondOutcome, err)
	}
}

func TestNonRetryableHTTPExhausts(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "non retryable")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "worker-1")
	result := testSender(pool).Send(context.Background(), *claim)
	if result.HTTPStatus != 400 || result.Retryable {
		t.Fatalf("classification=%+v", result)
	}
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("completion applied=%v err=%v", applied, err)
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM deliveries WHERE id=$1`, claim.ID).Scan(&status); err != nil || status != "exhausted" {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func TestConcurrentDispatchCreatesOneInitialDelivery(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "dispatch race")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	eventID := publishDeliveryEvent(t, apiServer, key)
	var wg sync.WaitGroup
	errorsCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := delivery.DispatchBatch(context.Background(), pool, 50)
			if err != nil {
				errorsCh <- err
			}
		}()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
	var count int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM deliveries WHERE event_id=$1 AND endpoint_id=$2 AND run_number=1`, eventID, endpointID).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("initial delivery count=%d err=%v", count, err)
	}
	var status string
	err = pool.QueryRow(context.Background(), `SELECT status FROM outbox WHERE event_id=$1`, eventID).Scan(&status)
	if err != nil || status != "dispatched" {
		t.Fatalf("outbox status=%s err=%v", status, err)
	}
}
