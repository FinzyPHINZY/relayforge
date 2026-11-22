package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/id"
)

func TestExhaustedDeliveryReviewIsAuditedAndTenantScoped(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "review owner")
	_, otherKey := tenantKey(t, pool, "review outsider")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, apiServer, key)
	claim := dispatchAndClaim(t, pool, "worker")
	result := testSender(pool).Send(context.Background(), *claim)
	if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied || result.Retryable {
		t.Fatalf("exhausted completion: applied=%v result=%+v err=%v", applied, result, err)
	}
	url := apiServer.URL + "/v1/deliveries/" + claim.ID
	resp, body := request(t, http.MethodGet, apiServer.URL+"/v1/deliveries?review_status=unreviewed", key, "")
	if resp.StatusCode != 200 {
		t.Fatalf("unreviewed queue: status=%d body=%s", resp.StatusCode, body)
	}
	var queue struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &queue); err != nil || len(queue.Data) != 1 || queue.Data[0].ID != claim.ID {
		t.Fatalf("unreviewed queue=%s err=%v", body, err)
	}
	for i := 0; i < 2; i++ {
		resp, body = request(t, http.MethodPatch, url+"/review", key, `{"status":"investigating","note":"checking receiver"}`)
		if resp.StatusCode != 200 {
			t.Fatalf("investigating status=%d body=%s", resp.StatusCode, body)
		}
	}
	resp, _ = request(t, http.MethodPatch, url+"/review", key, `{"status":"resolved"}`)
	if resp.StatusCode != 422 {
		t.Fatalf("resolution without note status=%d", resp.StatusCode)
	}
	resp, body = request(t, http.MethodPatch, url+"/review", key, `{"status":"resolved","note":"receiver fixed"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("resolution status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodGet, url, key, "")
	var detail struct {
		Delivery struct {
			ReviewStatus string `json:"review_status"`
		} `json:"delivery"`
		History []struct{ Status, Note string } `json:"review_history"`
	}
	if err := json.Unmarshal(body, &detail); err != nil || resp.StatusCode != 200 || detail.Delivery.ReviewStatus != "resolved" || len(detail.History) != 2 {
		t.Fatalf("review detail status=%d body=%s err=%v", resp.StatusCode, body, err)
	}
	resp, body = request(t, http.MethodGet, apiServer.URL+"/v1/deliveries?review_status=resolved", key, "")
	if err := json.Unmarshal(body, &queue); err != nil || resp.StatusCode != 200 || len(queue.Data) != 1 {
		t.Fatalf("resolved queue status=%d body=%s err=%v", resp.StatusCode, body, err)
	}
	resp, _ = request(t, http.MethodPatch, url+"/review", otherKey, `{"status":"resolved","note":"cross tenant"}`)
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant review status=%d", resp.StatusCode)
	}
	resp, _ = request(t, http.MethodGet, url, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant review detail status=%d", resp.StatusCode)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM delivery_review_history WHERE delivery_id=$1`, claim.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE target_id=$1 AND action='delivery.reviewed'`, claim.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("general review audit count=%d err=%v", count, err)
	}
	// A console session (owner) can review; history records the user, not a key.
	env := newConsoleEnv(t, pool, &api.Server{})
	uid, sessTok := env.sessionUser("reviewer@example.com", "Reviewer")
	memberID, _ := id.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO team_members (id,organization_id,name,email,role,user_id) VALUES ($1,$2,'Reviewer','reviewer@example.com','owner',$3)`, memberID, org, uid); err != nil {
		t.Fatal(err)
	}
	env.expect(http.MethodPatch, "/v1/deliveries/"+claim.ID+"/review", sessTok, org, `{"status":"investigating","note":"session check"}`, 200, "")
	var actorUser string
	var historyKey *string
	if err := pool.QueryRow(context.Background(), `SELECT actor_user_id::text, api_key_id::text FROM delivery_review_history WHERE delivery_id=$1 AND note='session check'`, claim.ID).Scan(&actorUser, &historyKey); err != nil || actorUser != uid || historyKey != nil {
		t.Fatalf("session history actor=%q key=%v err=%v", actorUser, historyKey, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM delivery_review_history WHERE delivery_id=$1 AND api_key_id IS NOT NULL AND actor_user_id IS NULL`, claim.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("key history rows=%d err=%v", count, err)
	}
	for _, bearer := range []string{key, sessTok} {
		got := env.expect(http.MethodGet, "/v1/deliveries/"+claim.ID, bearer, org, "", 200, "")
		hist, _ := got["review_history"].([]any)
		if len(hist) != 3 {
			t.Fatalf("history %v", got["review_history"])
		}
		last := hist[2].(map[string]any)
		if last["api_key_id"] != nil || last["actor_user_id"] != uid {
			t.Fatalf("session history entry %v", last)
		}
		if first := hist[0].(map[string]any); first["api_key_id"] == nil || first["actor_user_id"] != nil {
			t.Fatalf("key history entry %v", first)
		}
	}

	publishDeliveryEvent(t, apiServer, key)
	pending := dispatchAndClaim(t, pool, "second-worker")
	resp, _ = request(t, http.MethodPatch, apiServer.URL+"/v1/deliveries/"+pending.ID+"/review", key, `{"status":"investigating"}`)
	if resp.StatusCode != 409 {
		t.Fatalf("nonterminal review status=%d", resp.StatusCode)
	}
}
