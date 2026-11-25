package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

func keyWorkspace(t *testing.T, pool *pgxpool.Pool) (*consoleEnv, string, string, string) {
	t.Helper()
	env := newConsoleEnv(t, pool, &api.Server{})
	uid, tok := env.sessionUser("owner@example.com", "Owner One")
	ws := env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"keys"}`, 201, "")
	return env, uid, tok, ws["id"].(string)
}

func (e *consoleEnv) member(pool *pgxpool.Pool, org, email, displayName, role string) (string, string) {
	e.t.Helper()
	uid, tok := e.sessionUser(email, displayName)
	mid, _ := id.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO team_members (id,organization_id,name,email,role,user_id) VALUES ($1,$2,$3,$4,$5,$6)`, mid, org, email, email, role, uid); err != nil {
		e.t.Fatal(err)
	}
	return uid, tok
}

func TestConsoleKeyRoleCeiling(t *testing.T) {
	pool, _ := setup(t)
	env, _, ownerTok, org := keyWorkspace(t, pool)
	adminID, adminTok := env.member(pool, org, "admin@example.com", "", "admin")
	_, opTok := env.member(pool, org, "op@example.com", "", "operator")
	_, viewerTok := env.member(pool, org, "viewer@example.com", "", "viewer")

	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"too high","role":"owner","scopes":["events:read"]}`, 403, "scope_exceeds_role")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"viewer write","role":"viewer","scopes":["endpoints:write"]}`, 422, "invalid_scopes")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"op keys","role":"operator","scopes":["keys:write"]}`, 422, "invalid_scopes")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"bogus","role":"admin","scopes":["root:all"]}`, 422, "invalid_scopes")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"none","role":"admin","scopes":[]}`, 422, "invalid_scopes")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"   ","role":"admin","scopes":["events:read"]}`, 422, "invalid_key")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"`+strings.Repeat("a", 81)+`","role":"admin","scopes":["events:read"]}`, 422, "invalid_key")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"x","role":"root","scopes":["events:read"]}`, 422, "invalid_key")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"x","role":"admin","scopes":["events:read"],"expires_in_days":0}`, 422, "invalid_key")
	env.expect("POST", "/v1/keys", adminTok, org, `{"name":"x","role":"admin","scopes":["events:read"],"expires_in_days":366}`, 422, "invalid_key")

	created := env.expect("POST", "/v1/keys", adminTok, org, `{"name":"  `+strings.Repeat("é", 80)+`  ","role":"operator","scopes":["endpoints:write","endpoints:read","endpoints:write"],"expires_in_days":30}`, 201, "")
	raw, _ := created["key"].(string)
	if !strings.HasPrefix(raw, "whk_") || created["name"] != strings.Repeat("é", 80) || created["role"] != "operator" || created["prefix"] != raw[:16] {
		t.Fatalf("created %v", created)
	}
	if sc := created["scopes"].([]any); len(sc) != 2 {
		t.Fatalf("scopes not de-duplicated %v", sc)
	}
	if cb := created["created_by"].(map[string]any); cb["id"] != adminID || cb["display_name"] != nil {
		t.Fatalf("created_by %v", cb)
	}
	exp, err := time.Parse(time.RFC3339Nano, created["expires_at"].(string))
	if err != nil || exp.Before(time.Now().Add(29*24*time.Hour)) || exp.After(time.Now().Add(31*24*time.Hour)) {
		t.Fatalf("expires_at %v %v", created["expires_at"], err)
	}
	me := env.expect("GET", "/v1/me", raw, "", "", 200, "")
	if me["role"] != "operator" {
		t.Fatalf("console key me %v", me)
	}
	var memberID *string
	var createdBy string
	if err := pool.QueryRow(context.Background(), `SELECT member_id::text, created_by_user_id::text FROM api_keys WHERE key_prefix=$1`, raw[:16]).Scan(&memberID, &createdBy); err != nil || memberID != nil || createdBy != adminID {
		t.Fatalf("stored key member=%v by=%s %v", memberID, createdBy, err)
	}

	env.expect("POST", "/v1/keys", opTok, org, `{"name":"op","role":"operator","scopes":["endpoints:write"]}`, 403, "forbidden")
	env.expect("POST", "/v1/keys", viewerTok, org, `{"name":"v","role":"viewer","scopes":["events:read"]}`, 403, "forbidden")
	env.expect("GET", "/v1/keys", opTok, org, "", 200, "")
	env.expect("GET", "/v1/keys", viewerTok, org, "", 403, "forbidden")

	env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"owner key","role":"owner","scopes":["keys:read","keys:write"]}`, 201, "")
	list := env.expect("GET", "/v1/keys", ownerTok, org, "", 200, "")
	data := list["data"].([]any)
	if len(data) < 2 || data[0].(map[string]any)["name"] != "owner key" {
		t.Fatalf("list %v", list)
	}
	for _, item := range data {
		k := item.(map[string]any)
		if _, leaked := k["key"]; leaked {
			t.Fatalf("listing leaked raw key %v", k)
		}
		if k["name"] == "owner key" && k["created_by"].(map[string]any)["display_name"] != "Owner One" {
			t.Fatalf("owner created_by %v", k)
		}
	}
	foreign, foreignKey := tenantKey(t, pool, "foreign")
	if _, err := pool.Exec(context.Background(), `UPDATE api_keys SET scopes=array_append(scopes,'keys:read') WHERE organization_id=$1`, foreign); err != nil {
		t.Fatal(err)
	}
	fl := env.expect("GET", "/v1/keys", foreignKey, "", "", 200, "")
	if fd := fl["data"].([]any); len(fd) != 1 || fd[0].(map[string]any)["created_by"] != nil {
		t.Fatalf("foreign list %v", fl)
	}
}

func TestKeyRevokeAndLastUsed(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	env, ownerID, ownerTok, org := keyWorkspace(t, pool)

	created := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"ci","role":"operator","scopes":["endpoints:read"]}`, 201, "")
	raw, keyID := created["key"].(string), created["id"].(string)
	if created["last_used_at"] != nil || created["revoked_at"] != nil || created["expires_at"] != nil {
		t.Fatalf("fresh key %v", created)
	}
	env.expect("GET", "/v1/endpoints", raw, "", "", 200, "")
	var lastUsed *time.Time
	deadline := time.Now().Add(5 * time.Second)
	for lastUsed == nil && time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, `SELECT last_used_at FROM api_keys WHERE id=$1`, keyID).Scan(&lastUsed); err != nil {
			t.Fatal(err)
		}
		if lastUsed == nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if lastUsed == nil {
		t.Fatal("last_used_at not set after use")
	}
	for i := 0; i < 5; i++ {
		env.expect("GET", "/v1/endpoints", raw, "", "", 200, "")
	}
	time.Sleep(200 * time.Millisecond)
	var again *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_used_at FROM api_keys WHERE id=$1`, keyID).Scan(&again); err != nil || again == nil || !again.Equal(*lastUsed) {
		t.Fatalf("last_used_at moved within a minute: %v -> %v (%v)", lastUsed, again, err)
	}

	env.expect("DELETE", "/v1/keys/"+keyID, ownerTok, org, "", 204, "")
	env.expect("GET", "/v1/endpoints", raw, "", "", 401, "unauthorized")
	env.expect("DELETE", "/v1/keys/"+strings.ToUpper(keyID), ownerTok, org, "", 204, "")
	revoked := env.expect("GET", "/v1/keys", ownerTok, org, "", 200, "")["data"].([]any)
	if len(revoked) != 1 || revoked[0].(map[string]any)["revoked_at"] == nil || revoked[0].(map[string]any)["last_used_at"] == nil {
		t.Fatalf("revoked listing %v", revoked)
	}
	env.expect("DELETE", "/v1/keys/not-a-uuid", ownerTok, org, "", 404, "not_found")
	missing, _ := id.New()
	env.expect("DELETE", "/v1/keys/"+missing, ownerTok, org, "", 404, "not_found")
	foreign, foreignKey := tenantKey(t, pool, "foreign")
	var foreignKeyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM api_keys WHERE organization_id=$1`, foreign).Scan(&foreignKeyID); err != nil {
		t.Fatal(err)
	}
	env.expect("DELETE", "/v1/keys/"+foreignKeyID, ownerTok, org, "", 404, "not_found")
	env.expect("GET", "/v1/endpoints", foreignKey, "", "", 200, "")

	self := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"manager","role":"admin","scopes":["keys:read","keys:write","team:read"]}`, 201, "")
	selfRaw, selfID := self["key"].(string), self["id"].(string)
	env.expect("DELETE", "/v1/keys/"+selfID, selfRaw, "", "", 409, "cannot_revoke_self")

	audit := env.expect("GET", "/v1/audit", ownerTok, org, "", 200, "")["data"].([]any)
	counts := map[string]int{}
	for _, item := range audit {
		a := item.(map[string]any)
		if a["target_id"] != keyID {
			continue
		}
		counts[a["action"].(string)]++
		if a["actor_user_id"] != ownerID || a["actor_api_key_id"] != nil || a["target_type"] != "api_key" {
			t.Fatalf("audit row %v", a)
		}
		details, _ := a["details"].(map[string]any)
		for _, v := range details {
			if s, ok := v.(string); ok && (strings.Contains(s, raw) || strings.Contains(s, raw[:16])) {
				t.Fatalf("audit leaked key material %v", a)
			}
		}
	}
	if counts["api_key.issued"] != 1 || counts["api_key.revoked"] != 1 {
		t.Fatalf("audit counts %v", counts)
	}
}

func TestKeyOutlivesCreator(t *testing.T) {
	pool, _ := setup(t)
	env, _, ownerTok, org := keyWorkspace(t, pool)
	member := env.expect("POST", "/v1/team/members", ownerTok, org, `{"name":"`+strings.Repeat("m", 100)+`","email":"m@example.com","role":"viewer"}`, 201, "")
	var memberKeyRole, memberKeyName string
	if err := pool.QueryRow(context.Background(), `SELECT role,name FROM api_keys WHERE member_id=$1`, member["id"]).Scan(&memberKeyRole, &memberKeyName); err != nil || memberKeyRole != "viewer" || memberKeyName != strings.Repeat("m", 80) {
		t.Fatalf("member key role=%q name=%q %v", memberKeyRole, memberKeyName, err)
	}
	adminID, adminTok := env.member(pool, org, "admin@example.com", "Admin", "admin")
	created := env.expect("POST", "/v1/keys", adminTok, org, `{"name":"deploy","role":"admin","scopes":["endpoints:read"]}`, 201, "")
	raw := created["key"].(string)
	if _, err := pool.Exec(context.Background(), `UPDATE team_members SET status='suspended' WHERE user_id=$1 AND organization_id=$2`, adminID, org); err != nil {
		t.Fatal(err)
	}
	env.expect("GET", "/v1/endpoints", adminTok, org, "", 403, "not_a_member")
	env.expect("GET", "/v1/endpoints", raw, "", "", 200, "")
	if me := env.expect("GET", "/v1/me", raw, "", "", 200, ""); me["role"] != "admin" {
		t.Fatalf("me %v", me)
	}
}

func TestKeyLegacyNameMigration(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	down, err := os.ReadFile("../../db/migrations/000021_api_key_name_validate.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../db/migrations/000021_api_key_name_validate.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	org, _ := tenantKey(t, pool, "legacy")
	names := map[string]string{"": "Legacy key", "   ": "Legacy key", "  padded  ": "padded", strings.Repeat("b", 100): strings.Repeat("b", 80)}
	ids := map[string]string{}
	for name := range names {
		kid, _ := id.New()
		raw, _ := security.NewToken("whk_")
		prefix, _ := security.KeyPrefix(raw)
		// write the row as it could exist before 000019, then restore the NOT VALID check.
		if _, err = pool.Exec(ctx, `ALTER TABLE api_keys DROP CONSTRAINT api_keys_name_length_check`); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO api_keys (id,organization_id,name,key_prefix,key_hash,scopes) VALUES ($1,$2,$3,$4,$5,'{events:read}')`, kid, org, name, prefix, security.Hash(raw)); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `ALTER TABLE api_keys ADD CONSTRAINT api_keys_name_length_check CHECK (length(name) BETWEEN 1 AND 80) NOT VALID`); err != nil {
			t.Fatal(err)
		}
		ids[name] = kid
	}
	// without the migration, postgresql re-checks the NOT VALID constraint on update.
	if _, err = pool.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1`, ids[""]); pgCode(err) != "23514" {
		t.Fatalf("expected check violation before migration, got %v", err)
	}
	if _, err = pool.Exec(ctx, string(up), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	for name, want := range names {
		var got string
		if err = pool.QueryRow(ctx, `SELECT name FROM api_keys WHERE id=$1`, ids[name]).Scan(&got); err != nil || got != want {
			t.Fatalf("name %q -> %q want %q (%v)", name, got, want, err)
		}
	}
	var validated bool
	if err = pool.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conname='api_keys_name_length_check'`).Scan(&validated); err != nil || !validated {
		t.Fatalf("constraint validated=%v %v", validated, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1`, ids[""]); err != nil {
		t.Fatal(err)
	}
}

func legacyKey(t *testing.T, pool *pgxpool.Pool, org string, scopes []string) (string, string) {
	t.Helper()
	kid, _ := id.New()
	raw, _ := security.NewToken("whk_")
	prefix, _ := security.KeyPrefix(raw)
	if _, err := pool.Exec(context.Background(), `INSERT INTO api_keys (id,organization_id,name,key_prefix,key_hash,scopes) VALUES ($1,$2,'legacy',$3,$4,$5)`, kid, org, prefix, security.Hash(raw), scopes); err != nil {
		t.Fatal(err)
	}
	return kid, raw
}

func TestKeyRevokeRoleCeiling(t *testing.T) {
	pool, _ := setup(t)
	env, _, ownerTok, org := keyWorkspace(t, pool)
	_, adminTok := env.member(pool, org, "admin@example.com", "", "admin")
	ownerKey := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"owner","role":"owner","scopes":["events:read"]}`, 201, "")["id"].(string)
	revokedOwnerKey := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"gone","role":"owner","scopes":["events:read"]}`, 201, "")["id"].(string)
	env.expect("DELETE", "/v1/keys/"+revokedOwnerKey, ownerTok, org, "", 204, "")
	legacyID, _ := legacyKey(t, pool, org, []string{"events:read"})
	adminKey := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"admin","role":"admin","scopes":["events:read"]}`, 201, "")["id"].(string)

	for _, target := range []string{ownerKey, legacyID, revokedOwnerKey} {
		env.expect("DELETE", "/v1/keys/"+target, adminTok, org, "", 403, "scope_exceeds_role")
	}
	env.expect("DELETE", "/v1/keys/"+adminKey, adminTok, org, "", 204, "")
	env.expect("DELETE", "/v1/keys/"+ownerKey, ownerTok, org, "", 204, "")
	env.expect("DELETE", "/v1/keys/"+legacyID, ownerTok, org, "", 204, "")
}

func TestKeyScopesWithinCaller(t *testing.T) {
	pool, _ := setup(t)
	env, _, ownerTok, org := keyWorkspace(t, pool)
	narrow := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"narrow","role":"admin","scopes":["keys:read","keys:write"]}`, 201, "")["key"].(string)
	env.expect("POST", "/v1/keys", narrow, "", `{"name":"wide","role":"operator","scopes":["events:write"]}`, 403, "scope_exceeds_role")
	env.expect("POST", "/v1/keys", narrow, "", `{"name":"same","role":"admin","scopes":["keys:read"]}`, 201, "")

	_, legacy := legacyKey(t, pool, org, []string{"keys:write"})
	env.expect("POST", "/v1/keys", legacy, "", `{"name":"wide","role":"owner","scopes":["events:write"]}`, 403, "scope_exceeds_role")
	env.expect("POST", "/v1/keys", legacy, "", `{"name":"wide","role":"owner","scopes":["keys:read"]}`, 403, "scope_exceeds_role")
	env.expect("POST", "/v1/keys", legacy, "", `{"name":"same","role":"owner","scopes":["keys:write"]}`, 201, "")
}

func TestAccountsRollbackRevokesConsoleRoleKeys(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	env, _, ownerTok, org := keyWorkspace(t, pool)
	viewer := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"viewer","role":"viewer","scopes":["events:read"]}`, 201, "")
	owner := env.expect("POST", "/v1/keys", ownerTok, org, `{"name":"owner","role":"owner","scopes":["events:read"]}`, 201, "")
	legacyID, _ := legacyKey(t, pool, org, []string{"events:read"})
	for _, name := range []string{"000021_api_key_name_validate", "000020_review_user_actor", "000019_accounts"} {
		down, err := os.ReadFile("../../db/migrations/" + name + ".down.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(down), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("%s down: %v", name, err)
		}
	}
	revoked := func(keyID string) bool {
		t.Helper()
		var r bool
		if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM api_keys WHERE id=$1`, keyID).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if !revoked(viewer["id"].(string)) {
		t.Fatal("console viewer key survived rollback unrevoked; the old binary would treat it as owner")
	}
	if revoked(owner["id"].(string)) || revoked(legacyID) {
		t.Fatal("owner-level keys must stay usable after rollback")
	}
}
