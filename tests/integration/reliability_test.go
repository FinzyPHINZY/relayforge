package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/retention"
)

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
