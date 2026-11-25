package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
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

func TestTeamOwnerGuards(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	env := newConsoleEnv(t, pool, &api.Server{})
	uid, tok := env.sessionUser("owner@example.com", "Owner")
	org := env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"guarded"}`, 201, "")["id"].(string)
	var ownerRow string
	if err := pool.QueryRow(ctx, `SELECT id FROM team_members WHERE organization_id=$1 AND user_id=$2`, org, uid).Scan(&ownerRow); err != nil {
		t.Fatal(err)
	}
	keyID, _ := id.New()
	boot, _ := security.NewToken("whk_")
	prefix, _ := security.KeyPrefix(boot)
	if _, err := pool.Exec(ctx, `INSERT INTO api_keys (id,organization_id,name,key_prefix,key_hash,scopes) VALUES ($1,$2,'boot',$3,$4,$5)`, keyID, org, prefix, security.Hash(boot), []string{"team:read", "team:write"}); err != nil {
		t.Fatal(err)
	}
	if me := env.expect("GET", "/v1/me", boot, "", "", 200, ""); me["role"] != "owner" {
		t.Fatalf("bootstrap role %v", me)
	}
	admin := env.expect("POST", "/v1/team/members", boot, "", `{"name":"Admin","email":"admin@example.com","role":"admin"}`, 201, "")
	adminKey := admin["api_key"].(string)
	path := "/v1/team/members/" + ownerRow
	for _, body := range []string{`{"role":"viewer"}`, `{"status":"suspended"}`} {
		env.expect("PATCH", path, adminKey, "", body, 403, "forbidden")
	}
	env.expect("PATCH", "/v1/team/members/"+admin["id"].(string), adminKey, "", `{"role":"operator"}`, 200, "")

	env.expect("PATCH", "/v1/team/members/"+strings.ToUpper(ownerRow), boot, "", `{"role":"admin"}`, 409, "last_owner")
	for _, body := range []string{`{"status":"suspended"}`, `{"role":"admin"}`} {
		env.expect("PATCH", path, tok, org, body, 409, "last_owner")
		env.expect("PATCH", path, boot, "", body, 409, "last_owner")
	}
	var role, status string
	if err := pool.QueryRow(ctx, `SELECT role,status FROM team_members WHERE id=$1`, ownerRow).Scan(&role, &status); err != nil || role != "owner" || status != "active" {
		t.Fatalf("owner row changed: %s %s %v", role, status, err)
	}

	uid2, _ := env.sessionUser("owner2@example.com", "")
	second, _ := id.New()
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (id,organization_id,name,email,role,user_id) VALUES ($1,$2,'Owner2','owner2@example.com','owner',$3)`, second, org, uid2); err != nil {
		t.Fatal(err)
	}
	env.expect("PATCH", path, boot, "", `{"role":"admin"}`, 200, "")
	env.expect("PATCH", "/v1/team/members/"+second, boot, "", `{"status":"suspended"}`, 409, "last_owner")
}
