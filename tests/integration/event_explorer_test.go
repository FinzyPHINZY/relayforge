package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"webhook_delivery/internal/delivery"
)

func TestEventExplorerFiltersPaginationAndPayloadAccess(t *testing.T) {
	pool, server := setup(t)
	org, ownerKey := tenantKey(t, pool, "explorer-a")
	_, otherKey := tenantKey(t, pool, "explorer-b")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE api_keys SET scopes=array_append(scopes,'team:write') WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	var billingID string
	for i := range 55 {
		typeName := "order.created"
		data := fmt.Sprintf(`{"index":%d}`, i)
		if i == 54 {
			typeName = "billing.invoice"
			data = `{"order_id":"safe","password":"private","nested":{"api_key":"whk_secret","email":"person@example.test"}}`
		}
		resp, body := request(t, http.MethodPost, server.URL+"/v1/events", ownerKey, fmt.Sprintf(`{"type":%q,"data":%s}`, typeName, data))
		if resp.StatusCode != 202 {
			t.Fatalf("publish %d status=%d body=%s", i, resp.StatusCode, body)
		}
		if i == 54 {
			var event struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(body, &event); err != nil {
				t.Fatal(err)
			}
			billingID = event.ID
		}
	}
	if count, err := delivery.DispatchBatch(ctx, pool, 100); err != nil || count != 55 {
		t.Fatalf("dispatch empty subscriptions=%d err=%v", count, err)
	}
	var status atomic.Int32
	status.Store(204)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(int(status.Load())) }))
	defer receiver.Close()
	endpointID, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
	publishDeliveryEvent(t, server, ownerKey)
	claim := dispatchAndClaim(t, pool, "explorer-success")
	if applied, err := delivery.Complete(ctx, pool, *claim, testSender(pool).Send(ctx, *claim)); err != nil || !applied {
		t.Fatalf("success complete=%v err=%v", applied, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE endpoints SET max_attempts=1 WHERE id=$1`, endpointID); err != nil {
		t.Fatal(err)
	}
	status.Store(400)
	publishDeliveryEvent(t, server, ownerKey)
	claim = dispatchAndClaim(t, pool, "explorer-exhausted")
	if applied, err := delivery.Complete(ctx, pool, *claim, testSender(pool).Send(ctx, *claim)); err != nil || !applied {
		t.Fatalf("exhausted complete=%v err=%v", applied, err)
	}

	type page struct {
		Data []struct {
			ID              string `json:"id"`
			DeliverySummary struct {
				Total     int64 `json:"total"`
				Succeeded int64 `json:"succeeded"`
				Exhausted int64 `json:"exhausted"`
			} `json:"delivery_summary"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
	}
	readPage := func(key, query string) page {
		t.Helper()
		resp, body := request(t, http.MethodGet, server.URL+"/v1/events?"+query, key, "")
		if resp.StatusCode != 200 {
			t.Fatalf("query %s status=%d body=%s", query, resp.StatusCode, body)
		}
		var out page
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	cursor := ""
	count := 0
	for {
		q := "limit=20"
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		result := readPage(ownerKey, q)
		count += len(result.Data)
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	if count != 57 || len(readPage(otherKey, "limit=100").Data) != 0 {
		t.Fatalf("tenant pagination count=%d", count)
	}
	for _, item := range []struct {
		filter string
		want   int
	}{{"type=billing.", 1}, {"outcome=no_delivery", 55}, {"outcome=succeeded", 1}, {"outcome=exhausted", 1}, {"event_id=" + billingID, 1}} {
		if got := len(readPage(ownerKey, item.filter+"&limit=100").Data); got != item.want {
			t.Fatalf("filter %s got=%d want=%d", item.filter, got, item.want)
		}
	}
	after := url.QueryEscape(time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
	before := url.QueryEscape(time.Now().UTC().Add(time.Minute).Format(time.RFC3339))
	if got := len(readPage(ownerKey, "created_after="+after+"&created_before="+before+"&limit=100").Data); got != 57 {
		t.Fatalf("time filter got=%d", got)
	}
	for _, filter := range []string{"outcome=unknown", "event_id=bad", "created_after=invalid", "created_after=" + before + "&created_before=" + after} {
		resp, _ := request(t, http.MethodGet, server.URL+"/v1/events?"+filter, ownerKey, "")
		if resp.StatusCode != 400 {
			t.Fatalf("invalid filter %s status=%d", filter, resp.StatusCode)
		}
	}
	resp, body := request(t, http.MethodGet, server.URL+"/v1/events/"+billingID, ownerKey, "")
	if resp.StatusCode != 200 || strings.Contains(string(body), "password") || strings.Contains(string(body), "private") {
		t.Fatalf("metadata exposed payload status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/team/members", ownerKey, `{"name":"Reader","email":"explorer-reader@example.test","role":"viewer"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create viewer status=%d body=%s", resp.StatusCode, body)
	}
	var viewer struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &viewer); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodPost, server.URL+"/v1/events/"+billingID+"/reveal", viewer.APIKey, "")
	if resp.StatusCode != 403 {
		t.Fatalf("viewer reveal status=%d", resp.StatusCode)
	}
	resp, _ = request(t, http.MethodPost, server.URL+"/v1/events/"+billingID+"/reveal", otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant reveal status=%d", resp.StatusCode)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/events/"+billingID+"/reveal", ownerKey, "")
	if resp.StatusCode != 200 || !strings.Contains(string(body), "safe") || strings.Contains(string(body), "private") || strings.Contains(string(body), "whk_secret") || strings.Contains(string(body), "person@example.test") {
		t.Fatalf("redacted reveal status=%d body=%s", resp.StatusCode, body)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE organization_id=$1 AND action='event.payload_viewed' AND target_id=$2`, org, billingID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("payload audit count=%d err=%v", auditCount, err)
	}
}
