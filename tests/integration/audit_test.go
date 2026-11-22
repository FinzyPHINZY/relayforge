package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/id"
)

func TestAuditTrailIsTenantScopedRedactedAndAtomic(t *testing.T) {
	pool, server := setup(t)
	org, ownerKey := tenantKey(t, pool, "audit owner")
	otherOrg, otherKey := tenantKey(t, pool, "audit outsider")
	for _, tenant := range []string{org, otherOrg} {
		if _, err := pool.Exec(context.Background(), `UPDATE api_keys SET scopes=array_append(array_append(scopes,'team:read'),'team:write') WHERE organization_id=$1`, tenant); err != nil {
			t.Fatal(err)
		}
	}
	url := "https://8.8.8.8/webhooks"
	resp, body := request(t, http.MethodPost, server.URL+"/v1/endpoints", ownerKey, `{"url":"`+url+`","event_types":["order.created"]}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create endpoint status=%d body=%s", resp.StatusCode, body)
	}
	var created struct {
		Endpoint struct {
			ID string `json:"id"`
		} `json:"endpoint"`
		Secret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	endpointID := created.Endpoint.ID
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, ownerKey, `{"description":"private customer context","status":"paused","rate_limit_per_minute":30}`)
	if resp.StatusCode != 200 {
		t.Fatalf("patch endpoint status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/endpoints/"+endpointID+"/rotate-secret", ownerKey, `{"overlap_seconds":300}`)
	if resp.StatusCode != 201 {
		t.Fatalf("rotate status=%d body=%s", resp.StatusCode, body)
	}
	var rotated struct {
		Secret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(body, &rotated); err != nil {
		t.Fatal(err)
	}
	resp, body = request(t, http.MethodPost, server.URL+"/v1/team/members", ownerKey, `{"name":"Reader","email":"reader@example.test","role":"viewer"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create member status=%d body=%s", resp.StatusCode, body)
	}
	var member struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &member); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/audit", member.APIKey, "")
	if resp.StatusCode != 403 {
		t.Fatalf("viewer audit status=%d", resp.StatusCode)
	}
	// Role changes go straight to SQL so the audit event counts asserted below stay stable.
	for role, want := range map[string]int{"operator": 403, "admin": 200} {
		if _, err := pool.Exec(context.Background(), `UPDATE team_members SET role=$2 WHERE id=$1`, member.ID, role); err != nil {
			t.Fatal(err)
		}
		resp, body = request(t, http.MethodGet, server.URL+"/v1/audit", member.APIKey, "")
		if resp.StatusCode != want {
			t.Fatalf("%s audit status=%d want=%d body=%s", role, resp.StatusCode, want, body)
		}
	}
	resp, body = request(t, http.MethodPatch, server.URL+"/v1/team/members/"+member.ID, ownerKey, `{"role":"operator","status":"suspended"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("update member status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = request(t, http.MethodGet, server.URL+"/v1/audit?limit=20", ownerKey, "")
	if resp.StatusCode != 200 {
		t.Fatalf("owner audit status=%d body=%s", resp.StatusCode, body)
	}
	for _, secret := range []string{created.Secret, rotated.Secret, ownerKey, member.APIKey, url, "private customer context", "reader@example.test"} {
		if secret != "" && strings.Contains(string(body), secret) {
			t.Fatalf("audit leaked sensitive content %q", secret)
		}
	}
	var page struct {
		Data []struct {
			ID            string         `json:"id"`
			ActorAPIKeyID string         `json:"actor_api_key_id"`
			ActorRole     string         `json:"actor_role"`
			Action        string         `json:"action"`
			TargetType    string         `json:"target_type"`
			TargetID      string         `json:"target_id"`
			Details       map[string]any `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"endpoint.created": 1, "endpoint.updated": 1, "endpoint.secret_rotated": 1, "team.member_created": 1, "team.member_updated": 1, "api_key.issued": 1}
	if len(page.Data) != 6 {
		t.Fatalf("audit actions=%d body=%s", len(page.Data), body)
	}
	for _, entry := range page.Data {
		want[entry.Action]--
		if entry.ActorRole != "owner" || entry.ActorAPIKeyID == "" {
			t.Fatalf("missing actor: %+v", entry)
		}
	}
	for action, count := range want {
		if count != 0 {
			t.Fatalf("audit action %s count difference=%d", action, count)
		}
	}
	resp, body = request(t, http.MethodGet, server.URL+"/v1/audit", otherKey, "")
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"data":[]`) {
		t.Fatalf("cross-tenant audit status=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = request(t, http.MethodGet, server.URL+"/v1/audit", member.APIKey, "")
	if resp.StatusCode != 401 { // The member was suspended before the audit read.
		t.Fatalf("suspended member audit status=%d", resp.StatusCode)
	}
	resp, body = request(t, http.MethodGet, server.URL+"/v1/audit?limit=2", ownerKey, "")
	var firstPage struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &firstPage); err != nil || resp.StatusCode != 200 || len(firstPage.Data) != 2 || firstPage.NextCursor == "" {
		t.Fatalf("first audit page status=%d body=%s err=%v", resp.StatusCode, body, err)
	}
	resp, body = request(t, http.MethodGet, server.URL+"/v1/audit?limit=2&cursor="+firstPage.NextCursor, ownerKey, "")
	if resp.StatusCode != 200 || strings.Contains(string(body), firstPage.Data[0].ID) || strings.Contains(string(body), firstPage.Data[1].ID) {
		t.Fatalf("second audit page status=%d body=%s", resp.StatusCode, body)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE audit_events SET details='{}' WHERE id=$1`, page.Data[0].ID); err == nil {
		t.Fatal("audit update was permitted")
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM audit_events WHERE id=$1`, page.Data[0].ID); err == nil {
		t.Fatal("audit delete was permitted")
	}
	if _, err := pool.Exec(context.Background(), `CREATE FUNCTION reject_endpoint_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='endpoint.updated' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END; $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `CREATE TRIGGER reject_endpoint_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_endpoint_audit()`); err != nil {
		t.Fatal(err)
	}
	resp, _ = request(t, http.MethodPatch, server.URL+"/v1/endpoints/"+endpointID, ownerKey, `{"rate_limit_per_minute":60}`)
	if resp.StatusCode != 503 {
		t.Fatalf("audit failure patch status=%d", resp.StatusCode)
	}
	var rate, count int
	if err := pool.QueryRow(context.Background(), `SELECT rate_limit_per_minute FROM endpoints WHERE id=$1`, endpointID).Scan(&rate); err != nil || rate != 30 {
		t.Fatalf("endpoint changed without audit: rate=%d err=%v", rate, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE organization_id=$1`, org).Scan(&count); err != nil || count != 6 {
		t.Fatalf("failed patch audit count=%d err=%v", count, err)
	}

	// User actors: actor_user_id is returned and the label is the display name,
	// else the email local part, never the full email.
	env := newConsoleEnv(t, pool, &api.Server{})
	ownerID, ownerTok := env.sessionUser("ada.owner@example.com", "Ada Owner")
	ws := env.expect(http.MethodPost, "/v1/account/workspaces", ownerTok, "", `{"name":"audited"}`, 201, "")
	wsID := ws["id"].(string)
	plainID, plainTok := env.sessionUser("plain.user@example.com", "")
	memberID, _ := id.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO team_members (id,organization_id,name,email,role,user_id) VALUES ($1,$2,'plain','plain.user@example.com','viewer',$3)`, memberID, wsID, plainID); err != nil {
		t.Fatal(err)
	}
	env.expect(http.MethodPost, "/v1/account/sign-out", plainTok, "", "", 204, "")
	resp, body = request(t, http.MethodGet, server.URL+"/v1/audit", ownerKey, "")
	if resp.StatusCode != 200 || strings.Contains(string(body), "ada.owner") {
		t.Fatalf("key audit read status=%d body=%s", resp.StatusCode, body)
	}
	r, _ := http.NewRequest(http.MethodGet, env.url+"/v1/audit", nil)
	r.Header.Set("Authorization", "Bearer "+ownerTok)
	r.Header.Set("X-RelayForge-Org", wsID)
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || strings.Contains(string(raw), "@example.com") {
		t.Fatalf("user audit status=%d body=%s", res.StatusCode, raw)
	}
	var userPage struct {
		Data []struct {
			ActorAPIKeyID *string `json:"actor_api_key_id"`
			ActorUserID   string  `json:"actor_user_id"`
			ActorKeyName  string  `json:"actor_key_name"`
			ActorRole     string  `json:"actor_role"`
			Action        string  `json:"action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &userPage); err != nil || len(userPage.Data) != 2 {
		t.Fatalf("user audit page %s err=%v", raw, err)
	}
	for _, e := range userPage.Data {
		switch e.Action {
		case "organization.created_by_user":
			if e.ActorUserID != ownerID || e.ActorKeyName != "Ada Owner" || e.ActorRole != "owner" || e.ActorAPIKeyID != nil {
				t.Fatalf("created_by_user entry %+v", e)
			}
		case "user.signed_out":
			if e.ActorUserID != plainID || e.ActorKeyName != "plain.user" || e.ActorRole != "viewer" {
				t.Fatalf("signed_out entry %+v", e)
			}
		default:
			t.Fatalf("unexpected entry %+v", e)
		}
	}
}
