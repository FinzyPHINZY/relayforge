package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTeamRoleAndSuspension(t *testing.T) {
	pool, server := setup(t)
	org, ownerKey := tenantKey(t, pool, "team-owner")
	if _, err := pool.Exec(t.Context(), `UPDATE api_keys SET scopes=array_append(array_append(scopes,'team:read'),'team:write') WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	resp, body := request(t, http.MethodPost, server.URL+"/v1/team/members", ownerKey, `{"name":"Reader","email":"reader@example.test","role":"viewer"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create=%d body=%s", resp.StatusCode, body)
	}
	var created struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	resp, body = request(t, http.MethodGet, server.URL+"/v1/me", created.APIKey, "")
	if resp.StatusCode != 200 {
		t.Fatalf("viewer identity=%d body=%s", resp.StatusCode, body)
	}
	var identity struct {
		Role   string   `json:"role"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(body, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Role != "viewer" {
		t.Fatalf("viewer role=%q", identity.Role)
	}
	for _, scope := range identity.Scopes {
		if scope == "events:write" || scope == "team:write" {
			t.Fatalf("viewer exposed write scope=%q", scope)
		}
	}
	resp, body = request(t, http.MethodGet, server.URL+"/v1/team/members", created.APIKey, "")
	if resp.StatusCode != 200 {
		t.Fatalf("member read=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/events", created.APIKey, `{"type":"order.created","data":{"id":1}}`)
	if resp.StatusCode != 403 {
		t.Fatalf("viewer write=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/team/members/"+created.ID, ownerKey, `{"role":"operator"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("promote=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/events", created.APIKey, `{"type":"order.created","data":{"id":2}}`)
	if resp.StatusCode != 202 {
		t.Fatalf("operator write=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/team/members/"+created.ID, ownerKey, `{"role":"viewer"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("demote=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/events", created.APIKey, `{"type":"order.created","data":{"id":3}}`)
	if resp.StatusCode != 403 {
		t.Fatalf("demoted viewer write=%d body=%s", resp.StatusCode, body)
	}
	var memberKeyID string
	if err := pool.QueryRow(t.Context(), `SELECT id FROM api_keys WHERE member_id=$1 AND organization_id=$2`, created.ID, org).Scan(&memberKeyID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE api_keys SET scopes=ARRAY['team:read']::text[] WHERE id=$1`, memberKeyID); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/events", created.APIKey, "")
	if resp.StatusCode != 403 {
		t.Fatalf("missing events:read scope status=%d", resp.StatusCode)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/team/members", created.APIKey, "")
	if resp.StatusCode != 200 {
		t.Fatalf("allowed team:read scope status=%d", resp.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE api_keys SET expires_at=now()-interval '1 second' WHERE id=$1`, memberKeyID); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/me", created.APIKey, "")
	if resp.StatusCode != 401 {
		t.Fatalf("expired key status=%d", resp.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE api_keys SET expires_at=NULL WHERE id=$1`, memberKeyID); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/me", created.APIKey, "")
	if resp.StatusCode != 200 {
		t.Fatalf("restored key status=%d", resp.StatusCode)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/team/members/"+created.ID, ownerKey, `{"status":"suspended"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("suspend=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/team/members", created.APIKey, "")
	if resp.StatusCode != 401 {
		t.Fatalf("suspended key status=%d", resp.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE api_keys SET revoked_at=now() WHERE id=$1`, memberKeyID); err != nil {
		t.Fatal(err)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/team/members/"+created.ID, ownerKey, `{"status":"active"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("reactivate=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/me", created.APIKey, "")
	if resp.StatusCode != 401 {
		t.Fatalf("revoked key status=%d", resp.StatusCode)
	}
}
