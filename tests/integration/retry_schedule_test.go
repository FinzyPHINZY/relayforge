package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
)

func TestRetryAfterScheduleSurvivesWorkerRestart(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "retry-schedule")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE endpoints SET max_attempts=2 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, apiServer, key)
	first := dispatchAndClaim(t, pool, "retry-before-restart")
	result := testSender(pool).Send(ctx, *first)
	if result.HTTPStatus != http.StatusTooManyRequests || !result.Retryable || result.RetryAfter < 119*time.Second {
		t.Fatalf("receiver retry instruction=%+v", result)
	}
	if applied, err := delivery.Complete(ctx, pool, *first, result); err != nil || !applied {
		t.Fatalf("first completion=%v err=%v", applied, err)
	}

	// Use a fresh pool as the next worker would after a process restart.
	restartedPool, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer restartedPool.Close()
	var status string
	var next, historyNext time.Time
	if err := restartedPool.QueryRow(ctx, `SELECT d.status,d.next_attempt_at,a.retry_scheduled_at FROM deliveries d JOIN delivery_attempts a ON a.delivery_id=d.id WHERE d.id=$1 AND a.attempt_number=1`, first.ID).Scan(&status, &next, &historyNext); err != nil {
		t.Fatal(err)
	}
	if status != "retry_wait" || !next.Equal(historyNext) || next.Before(time.Now().Add(118*time.Second)) {
		t.Fatalf("stored retry status=%s next=%s history=%s", status, next, historyNext)
	}
	if claim, err := delivery.ClaimOne(ctx, restartedPool, "retry-after-restart", 45*time.Second); err != nil || claim != nil {
		t.Fatalf("claimed before due: claim=%+v err=%v", claim, err)
	}
	resp, body := request(t, http.MethodGet, apiServer.URL+"/v1/deliveries/"+first.ID, key, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delivery detail=%d body=%s", resp.StatusCode, body)
	}
	var detail struct {
		Attempts []struct {
			RetryScheduledAt *time.Time `json:"retry_scheduled_at"`
		} `json:"attempts"`
	}
	if err := json.Unmarshal(body, &detail); err != nil || len(detail.Attempts) != 1 || detail.Attempts[0].RetryScheduledAt == nil || !detail.Attempts[0].RetryScheduledAt.Equal(next) {
		t.Fatalf("retry history=%+v err=%v", detail, err)
	}
	// Advance only the test database clock for this delivery to avoid a two-minute wait.
	if _, err := restartedPool.Exec(ctx, `UPDATE deliveries SET next_attempt_at=now()-interval '1 second' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := delivery.ClaimOne(ctx, restartedPool, "retry-after-restart", 45*time.Second)
	if err != nil || second == nil || second.ID != first.ID || second.AttemptNumber != 2 {
		t.Fatalf("restarted worker claim=%+v err=%v", second, err)
	}
	result = testSender(restartedPool).Send(ctx, *second)
	if applied, err := delivery.Complete(ctx, restartedPool, *second, result); err != nil || !applied {
		t.Fatalf("second completion=%v err=%v", applied, err)
	}
	if err := restartedPool.QueryRow(ctx, `SELECT status FROM deliveries WHERE id=$1`, first.ID).Scan(&status); err != nil || status != "exhausted" {
		t.Fatalf("max attempts status=%s err=%v", status, err)
	}
}

func TestNetworkFailureSchedulesRetry(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "network-retry")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	receiver.Close()
	publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "network-failure")
	result := testSender(pool).Send(context.Background(), *claim)
	if result.Outcome != "network_error" || !result.Retryable {
		t.Fatalf("network classification=%+v", result)
	}
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied {
		t.Fatalf("retry completion=%v err=%v", applied, err)
	}
	var status string
	var due time.Time
	if err := pool.QueryRow(context.Background(), `SELECT status,next_attempt_at FROM deliveries WHERE id=$1`, claim.ID).Scan(&status, &due); err != nil || status != "retry_wait" || !due.After(time.Now()) {
		t.Fatalf("network retry status=%s due=%s err=%v", status, due, err)
	}
}
