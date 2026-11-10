package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/retention"
)

func TestCommittedEventSurvivesLostHTTPResponse(t *testing.T) {
	pool, retryServer := setup(t)
	org, key := tenantKey(t, pool, "lost response")
	// Buffer the application response, then close the connection without sending it.
	// The handler has committed before the simulated network loss occurs.
	app := (&api.Server{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test"}).Router()
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := httptest.NewRecorder()
		app.ServeHTTP(recorded, r)
		if recorded.Code != http.StatusAccepted {
			t.Errorf("first acceptance status=%d body=%s", recorded.Code, recorded.Body.String())
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack response: %v", err)
			return
		}
		conn.Close()
	}))
	defer firstServer.Close()
	body := `{"type":"order.created","data":{"order_id":"lost-response"}}`
	first, err := http.NewRequest(http.MethodPost, firstServer.URL+"/v1/events", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	first.Header.Set("Authorization", "Bearer "+key)
	first.Header.Set("Content-Type", "application/json")
	first.Header.Set("Idempotency-Key", "lost-response")
	response, err := http.DefaultClient.Do(first)
	if err == nil {
		response.Body.Close()
		t.Fatal("expected the committed response to be lost")
	}
	second, err := http.NewRequest(http.MethodPost, retryServer.URL+"/v1/events", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	second.Header = first.Header.Clone()
	response, err = http.DefaultClient.Do(second)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("retry status=%d", response.StatusCode)
	}
	var event struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&event); err != nil || event.ID == "" {
		t.Fatalf("retry event=%+v err=%v", event, err)
	}
	var events, outbox int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM events WHERE organization_id=$1),(SELECT count(*) FROM outbox WHERE organization_id=$1)`, org).Scan(&events, &outbox); err != nil {
		t.Fatal(err)
	}
	if events != 1 || outbox != 1 {
		t.Fatalf("events=%d outbox=%d", events, outbox)
	}
}

func TestDispatchFailureRollsBackMaterializedDeliveries(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "dispatch rollback")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	eventID := publishDeliveryEvent(t, apiServer, key)
	if _, err := pool.Exec(context.Background(), `CREATE FUNCTION fail_dispatch_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='dispatched' THEN RAISE EXCEPTION 'injected dispatch failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `CREATE TRIGGER fail_dispatch_update BEFORE UPDATE ON outbox FOR EACH ROW EXECUTE FUNCTION fail_dispatch_update()`); err != nil {
		t.Fatal(err)
	}
	if _, err := delivery.DispatchBatch(context.Background(), pool, 50); err == nil {
		t.Fatal("expected dispatch to fail after delivery insertion")
	}
	var deliveries int
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM deliveries WHERE event_id=$1),(SELECT status FROM outbox WHERE event_id=$1)`, eventID).Scan(&deliveries, &status); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 || status != "pending" {
		t.Fatalf("failed dispatch left deliveries=%d outbox=%s", deliveries, status)
	}
	if _, err := pool.Exec(context.Background(), `DROP TRIGGER fail_dispatch_update ON outbox`); err != nil {
		t.Fatal(err)
	}
	if n, err := delivery.DispatchBatch(context.Background(), pool, 50); err != nil || n != 1 {
		t.Fatalf("recovered dispatch=%d err=%v", n, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM deliveries WHERE event_id=$1`, eventID).Scan(&deliveries); err != nil || deliveries != 1 {
		t.Fatalf("recovered deliveries=%d err=%v", deliveries, err)
	}
}

func TestSlowReceiverTimesOutWithoutBlockingRecovery(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "slow receiver")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	if _, err := pool.Exec(context.Background(), `UPDATE endpoints SET timeout_ms=1000 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "timeout-worker")
	if claim.Timeout != time.Second {
		t.Fatalf("attempt timeout=%s", claim.Timeout)
	}
	result := testSender(pool).Send(context.Background(), *claim)
	if result.Outcome != "timeout" || !result.Retryable {
		t.Fatalf("slow receiver classification=%+v", result)
	}
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("timeout completion=%v err=%v", applied, err)
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM deliveries WHERE id=$1`, claim.ID).Scan(&status); err != nil || status != "retry_wait" {
		t.Fatalf("timeout delivery status=%s err=%v", status, err)
	}
}

func TestWorkerShutdownDrainsClaimedAttempt(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "drain")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	if n, err := delivery.DispatchBatch(context.Background(), pool, 50); err != nil || n != 1 {
		t.Fatalf("dispatch=%d err=%v", n, err)
	}
	started := make(chan string, 1)
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	engine := delivery.Engine{DB: pool, WorkerID: "drain-worker", Concurrency: 1, Lease: 45 * time.Second, DrainTimeout: 2 * time.Second, SendFunc: func(sendCtx context.Context, c delivery.Claimed) delivery.Result {
		started <- c.ID
		select {
		case <-release:
			return delivery.Result{Outcome: "success", HTTPStatus: 204}
		case <-sendCtx.Done():
			return delivery.Result{Outcome: "network_error", ErrorCode: "cancelled", Retryable: true}
		}
	}}
	go func() { engine.Run(ctx); close(done) }()
	var deliveryID string
	select {
	case deliveryID = <-started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("worker did not claim")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("worker returned before attempt finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not drain")
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM deliveries WHERE id=$1`, deliveryID).Scan(&status); err != nil || status != "succeeded" {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func TestDatabaseRestartPreservesAcceptedBacklog(t *testing.T) {
	pool, apiServer, container := setupWithContainer(t)
	org, key := tenantKey(t, pool, "database restart")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	eventID := publishDeliveryEvent(t, apiServer, key)
	stopTimeout := 10 * time.Second
	if err := container.Stop(context.Background(), &stopTimeout); err != nil {
		t.Fatal(err)
	}
	response, body := request(t, http.MethodPost, apiServer.URL+"/v1/events", key, `{"type":"order.created","data":{"order_id":"during-outage"}}`)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("outage acceptance status=%d body=%s", response.StatusCode, body)
	}
	failedCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, err := delivery.DispatchBatch(failedCtx, pool, 50)
	cancel()
	if err == nil {
		t.Fatal("dispatch succeeded while PostgreSQL stopped")
	}
	if err := container.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Docker may assign a new published port when the container starts again.
	url, err := container.ConnectionString(context.Background(), "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	recoveredPool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer recoveredPool.Close()
	deadline := time.Now().Add(20 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = recoveredPool.Ping(pingCtx)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("database did not recover: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	claim := dispatchAndClaim(t, recoveredPool, "restart-worker")
	if claim.EventID != eventID {
		t.Fatalf("event=%s want=%s", claim.EventID, eventID)
	}
	result := testSender(recoveredPool).Send(context.Background(), *claim)
	if applied, err := delivery.Complete(context.Background(), recoveredPool, *claim, result); err != nil || !applied || result.Outcome != "success" {
		t.Fatalf("completion=%v result=%+v err=%v", applied, result, err)
	}
}

func TestRetentionKeepsPendingAndRecentReplay(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "retention")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	oldEvent := publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "retention-worker")
	result := testSender(pool).Send(context.Background(), *claim)
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("completion=%v err=%v", applied, err)
	}
	replayID := replayRequest(t, apiServer.URL, key, claim.ID, "retention-replay")
	pendingEvent := publishDeliveryEvent(t, apiServer, key)
	now := time.Now().UTC()
	old := now.Add(-45 * 24 * time.Hour)
	for _, eventID := range []string{oldEvent, pendingEvent} {
		if _, err := pool.Exec(context.Background(), `UPDATE events SET created_at=$2,expires_at=$2 WHERE id=$1`, eventID, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(context.Background(), `UPDATE deliveries SET created_at=$2,completed_at=$2 WHERE id=$1`, claim.ID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO idempotency_records (organization_id,idempotency_key,request_fingerprint,event_id,created_at,expires_at) VALUES ($1,'expired',decode('01','hex'),$2,$3,$4),($1,'young',decode('01','hex'),$2,$5,$6)`, org, pendingEvent, now.Add(-48*time.Hour), now.Add(-25*time.Hour), now.Add(-time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	stats, err := retention.Sweep(context.Background(), pool, now, now.Add(-30*24*time.Hour), 100)
	if err != nil || stats.Events != 0 || stats.IdempotencyRecords != 1 {
		t.Fatalf("first sweep=%+v err=%v", stats, err)
	}
	replayClaim, err := delivery.ClaimOne(context.Background(), pool, "retention-worker-2", 45*time.Second)
	if err != nil || replayClaim == nil || replayClaim.ID != replayID {
		t.Fatalf("replay claim=%+v err=%v", replayClaim, err)
	}
	replayResult := testSender(pool).Send(context.Background(), *replayClaim)
	if applied, e := delivery.Complete(context.Background(), pool, *replayClaim, replayResult); e != nil || !applied {
		t.Fatalf("replay completion=%v err=%v", applied, e)
	}
	// A recently completed replay must remain available even when it was created long ago.
	stats, err = retention.Sweep(context.Background(), pool, now, now.Add(-30*24*time.Hour), 100)
	if err != nil || stats.Events != 0 {
		t.Fatalf("recently completed replay sweep=%+v err=%v", stats, err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE deliveries SET created_at=$2,completed_at=$2 WHERE id=$1`, replayID, old); err != nil {
		t.Fatal(err)
	}
	stats, err = retention.Sweep(context.Background(), pool, now, now.Add(-30*24*time.Hour), 100)
	if err != nil || stats.Events != 1 {
		t.Fatalf("second sweep=%+v err=%v", stats, err)
	}
	var oldCount, pendingCount, replayCount int
	for _, x := range []struct {
		query string
		arg   string
		dst   *int
	}{
		{`SELECT count(*) FROM events WHERE id=$1`, oldEvent, &oldCount},
		{`SELECT count(*) FROM events WHERE id=$1`, pendingEvent, &pendingCount},
		{`SELECT count(*) FROM replay_requests WHERE replay_delivery_id=$1`, replayID, &replayCount},
	} {
		if err := pool.QueryRow(context.Background(), x.query, x.arg).Scan(x.dst); err != nil {
			t.Fatal(err)
		}
	}
	if oldCount != 0 || pendingCount != 1 || replayCount != 0 {
		t.Fatalf("old=%d pending=%d replay=%d", oldCount, pendingCount, replayCount)
	}
}
