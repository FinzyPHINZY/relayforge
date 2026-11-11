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
	resp, body = request(t, http.MethodGet, server.URL+"/v1/team/members", created.APIKey, "")
	if resp.StatusCode != 200 {
		t.Fatalf("member read=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/events", created.APIKey, `{"type":"order.created","data":{"id":1}}`)
	if resp.StatusCode != 403 {
		t.Fatalf("viewer write=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/team/members/"+created.ID, ownerKey, `{"status":"suspended"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("suspend=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/team/members", created.APIKey, "")
	if resp.StatusCode != 401 {
		t.Fatalf("suspended key status=%d", resp.StatusCode)
	}
}
