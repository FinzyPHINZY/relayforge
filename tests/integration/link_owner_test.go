package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webhook_delivery/internal/accounts"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/mail"
	"webhook_delivery/internal/tenant"
)

func TestLinkOwnerIdempotent(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	org, _ := tenantKey(t, pool, "bootstrap")

	uid, err := tenant.LinkOwner(ctx, pool, org, "  Owner@Example.COM ")
	if err != nil || !id.Valid(uid) {
		t.Fatalf("first link: %q %v", uid, err)
	}
	uid2, err := tenant.LinkOwner(ctx, pool, org, "owner@example.com")
	if err != nil || uid2 != uid {
		t.Fatalf("second link: %q %v (want %q)", uid2, err, uid)
	}

	var users, members, verified int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(email_verified_at) FROM users WHERE email='owner@example.com'`).Scan(&users, &verified); err != nil || users != 1 || verified != 1 {
		t.Fatalf("users=%d verified=%d %v", users, verified, err)
	}
	var role, status string
	if err := pool.QueryRow(ctx, `SELECT count(*) OVER (), role, status FROM team_members WHERE organization_id=$1 AND user_id=$2`, org, uid).Scan(&members, &role, &status); err != nil || members != 1 || role != "owner" || status != "active" {
		t.Fatalf("members=%d role=%s status=%s %v", members, role, status, err)
	}
	var created, updated int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action='team.member_created'), count(*) FILTER (WHERE action='team.member_updated') FROM audit_events WHERE organization_id=$1 AND actor_role='system' AND target_type='team_member'`, org).Scan(&created, &updated); err != nil || created != 1 || updated != 0 {
		t.Fatalf("audit created=%d updated=%d %v", created, updated, err)
	}

	// An existing row for the same email (no user yet) is upgraded, not duplicated.
	mid, _ := id.New()
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (id, organization_id, name, email, role, status) VALUES ($1,$2,'Pre','pre@example.com','viewer','suspended')`, mid, org); err != nil {
		t.Fatal(err)
	}
	puid, err := tenant.LinkOwner(ctx, pool, org, "pre@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var gotUser, gotRole, gotStatus string
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) OVER (), user_id::text, role, status FROM team_members WHERE organization_id=$1 AND email='pre@example.com'`, org).Scan(&n, &gotUser, &gotRole, &gotStatus); err != nil || n != 1 || gotUser != puid || gotRole != "owner" || gotStatus != "active" {
		t.Fatalf("pre n=%d user=%s role=%s status=%s %v", n, gotUser, gotRole, gotStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE organization_id=$1 AND action='team.member_updated' AND target_id=$2`, org, mid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("update audit %d %v", n, err)
	}

	// Errors: unknown org, suspended org, bad email, inactive user.
	missing, _ := id.New()
	if _, err := tenant.LinkOwner(ctx, pool, missing, "a@example.com"); err == nil || err.Error() != "organization not found or not active" {
		t.Fatalf("missing org: %v", err)
	}
	if _, err := tenant.LinkOwner(ctx, pool, org, "not an email"); err == nil {
		t.Fatal("bad email accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET status='deleting' WHERE id=$1`, puid); err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.LinkOwner(ctx, pool, org, "pre@example.com"); err == nil {
		t.Fatal("inactive user linked")
	}
	if _, err := pool.Exec(ctx, `UPDATE organizations SET status='suspended' WHERE id=$1`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.LinkOwner(ctx, pool, org, "owner@example.com"); err == nil {
		t.Fatal("suspended org linked")
	}
}

func TestLinkOwnerSignsIn(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	org, _ := tenantKey(t, pool, "bootstrap-signin")
	if _, err := tenant.LinkOwner(ctx, pool, org, "Boss@Example.com"); err != nil {
		t.Fatal(err)
	}

	limiter := startRedisLimiter(t)
	const dashToken = "test-dashboard-token-0123456789abcdef"
	mailer := &mail.LogMailer{}
	store := &accounts.Store{DB: pool, IPHashKey: bytes.Repeat([]byte{7}, 32)}
	s := &api.Server{DB: pool, Accounts: store, DashboardToken: dashToken, EncryptionKey: bytes.Repeat([]byte{1}, 32), EncryptionKeyID: "test",
		MagicLinks: accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: limiter, PerEmailPerHour: 5, PerIPPerHour: 20}}
	srv := httptest.NewServer(s.Router())
	t.Cleanup(srv.Close)

	do := func(method, path, bearer, body string, hdr map[string]string) map[string]any {
		t.Helper()
		r, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
		}
		out := map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	dash := map[string]string{"X-RelayForge-Dashboard-Token": dashToken}
	do("POST", "/v1/auth/email/start", "", `{"email":"boss@example.com"}`, dash)
	msgs := mailer.Messages()
	if len(msgs) != 1 {
		t.Fatalf("mails: %d", len(msgs))
	}
	_, tok, ok := strings.Cut(msgs[0].Text, "/auth/email?token=")
	if !ok || len(tok) < 47 {
		t.Fatalf("no link in %q", msgs[0].Text)
	}
	sess := do("POST", "/v1/auth/email/consume", "", `{"token":"`+tok[:47]+`"}`, dash)
	st, _ := sess["session_token"].(string)
	acct := do("GET", "/v1/account", st, "", nil)
	wss, _ := acct["workspaces"].([]any)
	if len(wss) != 1 || wss[0].(map[string]any)["id"] != org || wss[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("account %v", acct)
	}
}

func TestLinkOwnerRefusesOtherMembersRow(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	for _, role := range []string{"owner", "viewer"} {
		org, _ := tenantKey(t, pool, "conflict-"+role)
		vid, _ := id.New()
		email := role + "@example.com"
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, email_verified_at) VALUES ($1,$2,now())`, vid, "v-"+email); err != nil {
			t.Fatal(err)
		}
		mid, _ := id.New()
		// V's row carries the target email but is linked to V.
		if _, err := pool.Exec(ctx, `INSERT INTO team_members (id, organization_id, name, email, role, status, user_id) VALUES ($1,$2,'V',$3,$4,'active',$5)`, mid, org, email, role, vid); err != nil {
			t.Fatal(err)
		}
		_, err := tenant.LinkOwner(ctx, pool, org, email)
		if err == nil || err.Error() != "email already belongs to a different member of this workspace" {
			t.Fatalf("%s: err=%v", role, err)
		}
		var gotUser, gotRole, gotStatus string
		if err := pool.QueryRow(ctx, `SELECT user_id::text, role, status FROM team_members WHERE id=$1`, mid).Scan(&gotUser, &gotRole, &gotStatus); err != nil || gotUser != vid || gotRole != role || gotStatus != "active" {
			t.Fatalf("%s: row changed %s %s %s %v", role, gotUser, gotRole, gotStatus, err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE organization_id=$1`, org).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: audit rows %d %v", role, n, err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM team_members WHERE organization_id=$1 AND user_id<>$2`, org, vid).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: extra members %d", role, n)
		}
	}
}
