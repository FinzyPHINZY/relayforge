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
)

func TestDeliveryInspectorTimelineDataAndRedaction(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "delivery-inspector")
	_, otherKey := tenantKey(t, pool, "delivery-inspector-other")
	ctx := context.Background()
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	type detail struct {
		Delivery struct {
			Status                 string    `json:"status"`
			NextAttemptAt          time.Time `json:"next_attempt_at"`
			ReplayedFromDeliveryID string    `json:"replayed_from_delivery_id"`
			RunNumber              int       `json:"run_number"`
		} `json:"delivery"`
		Attempts []struct {
			Outcome          string     `json:"outcome"`
			ErrorCode        string     `json:"error_code"`
			HTTPStatus       int        `json:"http_status"`
			ResponseExcerpt  string     `json:"response_excerpt"`
			RetryScheduledAt *time.Time `json:"retry_scheduled_at"`
		} `json:"attempts"`
		EventAcceptedAt    time.Time `json:"event_accepted_at"`
		OutboxDispatchedAt time.Time `json:"outbox_dispatched_at"`
	}
	read := func(id string) detail {
		t.Helper()
		resp, body := request(t, http.MethodGet, server.URL+"/v1/deliveries/"+id, key, "")
		if resp.StatusCode != 200 {
			t.Fatalf("detail %s status=%d body=%s", id, resp.StatusCode, body)
		}
		var got detail
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.EventAcceptedAt.IsZero() || got.OutboxDispatchedAt.IsZero() {
			t.Fatalf("missing acceptance or dispatch time: %+v", got)
		}
		return got
	}
	complete := func(claim *delivery.Claimed, result delivery.Result) {
		t.Helper()
		if applied, err := delivery.Complete(ctx, pool, *claim, result); err != nil || !applied {
			t.Fatalf("complete applied=%v err=%v", applied, err)
		}
	}
	dueNow := func(id string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE deliveries SET next_attempt_at=now()-interval '1 second' WHERE id=$1 AND status='retry_wait'`, id); err != nil {
			t.Fatal(err)
		}
	}

	publishDeliveryEvent(t, server, key)
	first := dispatchAndClaim(t, pool, "inspector-429")
	complete(first, delivery.Result{Outcome: "http_error", HTTPStatus: 429, ErrorCode: "http_429", ResponseExcerpt: "Too Many Requests", Retryable: true})
	waiting := read(first.ID)
	if waiting.Delivery.Status != "retry_wait" || len(waiting.Attempts) != 1 || waiting.Attempts[0].RetryScheduledAt == nil || !waiting.Attempts[0].RetryScheduledAt.Equal(waiting.Delivery.NextAttemptAt) || waiting.Attempts[0].ResponseExcerpt != "Too Many Requests" {
		t.Fatalf("429 retry history=%+v", waiting)
	}
	dueNow(first.ID)
	second, err := delivery.ClaimOne(ctx, pool, "inspector-success", 45*time.Second)
	if err != nil || second == nil {
		t.Fatalf("second claim=%v err=%v", second, err)
	}
	complete(second, delivery.Result{Outcome: "success", HTTPStatus: 204})
	succeeded := read(first.ID)
	if succeeded.Delivery.Status != "succeeded" || len(succeeded.Attempts) != 2 || succeeded.Attempts[1].Outcome != "success" {
		t.Fatalf("success history=%+v", succeeded)
	}

	if _, err := pool.Exec(ctx, `UPDATE endpoints SET max_attempts=1 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, server, key)
	exhaustedClaim := dispatchAndClaim(t, pool, "inspector-exhausted")
	complete(exhaustedClaim, delivery.Result{Outcome: "http_error", HTTPStatus: 400, ErrorCode: "http_400", ResponseExcerpt: `{"token":"private"}`})
	exhausted := read(exhaustedClaim.ID)
	if exhausted.Delivery.Status != "exhausted" || exhausted.Attempts[0].ResponseExcerpt != "[Response body withheld]" || strings.Contains(exhausted.Attempts[0].ResponseExcerpt, "private") {
		t.Fatalf("exhausted detail=%+v", exhausted)
	}
	replayID := replayRequest(t, server.URL, key, exhaustedClaim.ID, "inspector-replay")
	replay := read(replayID)
	if replay.Delivery.RunNumber != 2 || replay.Delivery.ReplayedFromDeliveryID != exhaustedClaim.ID || replay.Delivery.Status != "pending" {
		t.Fatalf("replay detail=%+v", replay)
	}
	// Finish the new run before testing another claim on the same endpoint.
	replayedClaim, err := delivery.ClaimOne(ctx, pool, "inspector-replay", 45*time.Second)
	if err != nil || replayedClaim == nil || replayedClaim.ID != replayID {
		t.Fatalf("replay claim=%v err=%v", replayedClaim, err)
	}
	complete(replayedClaim, delivery.Result{Outcome: "success", HTTPStatus: 204})

	if _, err := pool.Exec(ctx, `UPDATE endpoints SET max_attempts=3 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	publishDeliveryEvent(t, server, key)
	timeoutClaim := dispatchAndClaim(t, pool, "inspector-timeout")
	complete(timeoutClaim, delivery.Result{Outcome: "timeout", ErrorCode: "request_timeout", Retryable: true})
	timedOut := read(timeoutClaim.ID)
	if timedOut.Attempts[0].Outcome != "timeout" || timedOut.Attempts[0].RetryScheduledAt == nil {
		t.Fatalf("timeout detail=%+v", timedOut)
	}
	dueNow(timeoutClaim.ID)
	crashed, err := delivery.ClaimOne(ctx, pool, "inspector-crashed", time.Second)
	if err != nil || crashed == nil || crashed.ID != timeoutClaim.ID {
		t.Fatalf("crashed claim=%v err=%v", crashed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, crashed.ID); err != nil {
		t.Fatal(err)
	}
	if reclaimed, err := delivery.ReclaimExpired(ctx, pool, 10); err != nil || reclaimed != 1 {
		t.Fatalf("reclaimed=%d err=%v", reclaimed, err)
	}
	recovered := read(crashed.ID)
	if len(recovered.Attempts) != 2 || recovered.Attempts[1].Outcome != "unknown" || recovered.Attempts[1].ErrorCode != "lease_expired" || recovered.Attempts[1].RetryScheduledAt == nil {
		t.Fatalf("lease history=%+v", recovered)
	}
	resp, _ := request(t, http.MethodGet, server.URL+"/v1/deliveries/"+first.ID, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant detail status=%d", resp.StatusCode)
	}
}
