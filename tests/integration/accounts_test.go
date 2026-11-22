package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/accounts"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/mail"
)

func TestSessionLifecycle(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	newUser := func(email, status string) string {
		uid, err := id.New()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1,$2,$3)`, uid, email, status); err != nil {
			t.Fatal(err)
		}
		return uid
	}
	now := time.Now().UTC().Truncate(time.Second)
	clock := now
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k"), Now: func() time.Time { return clock }}
	create := func(uid string) (string, accounts.Session) {
		t.Helper()
		var tok string
		var sess accounts.Session
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tok, sess, err = st.CreateSession(ctx, tx, uid, "1.2.3.4", "ua")
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return tok, sess
	}

	u1 := newUser("a@example.com", "active")
	tok, sess := create(u1)
	if len(tok) != 47 || tok[:4] != "rfs_" {
		t.Fatalf("bad token %q", tok)
	}
	gotS, gotU, err := st.LookupSession(ctx, tok)
	if err != nil || gotS.ID != sess.ID || gotU.ID != u1 || gotU.Email != "a@example.com" {
		t.Fatalf("lookup: %v %+v %+v", err, gotS, gotU)
	}
	if _, _, err := st.LookupSession(ctx, "rfs_nope"); !errors.Is(err, accounts.ErrSessionInvalid) {
		t.Fatalf("malformed token: %v", err)
	}
	var signedIn *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_sign_in_at FROM users WHERE id=$1`, u1).Scan(&signedIn); err != nil || signedIn == nil {
		t.Fatalf("last_sign_in_at not set: %v", err)
	}

	// idle timeout
	clock = now.Add(7*24*time.Hour + time.Minute)
	if _, _, err := st.LookupSession(ctx, tok); !errors.Is(err, accounts.ErrSessionInvalid) {
		t.Fatalf("idle: %v", err)
	}

	// absolute timeout despite daily activity
	clock = now
	tok2, _ := create(u1)
	for d := 1; d < 30; d++ {
		clock = now.Add(time.Duration(d) * 24 * time.Hour)
		if _, _, err := st.LookupSession(ctx, tok2); err != nil {
			t.Fatalf("day %d: %v", d, err)
		}
	}
	clock = now.Add(30*24*time.Hour + time.Minute)
	if _, _, err := st.LookupSession(ctx, tok2); !errors.Is(err, accounts.ErrSessionInvalid) {
		t.Fatalf("absolute: %v", err)
	}

	// list, revoke
	clock = now
	u1 = newUser("a2@example.com", "active")
	tok3, s3 := create(u1)
	list, err := st.ListSessions(ctx, u1)
	if err != nil || len(list) != 1 || list[0].ID != s3.ID || list[0].UserAgent != "ua" {
		t.Fatalf("list: %v %+v", err, list)
	}
	u2 := newUser("b@example.com", "active")
	if err := st.RevokeSession(ctx, u2, s3.ID); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatalf("other user revoke: %v", err)
	}
	if _, _, err := st.LookupSession(ctx, tok3); err != nil {
		t.Fatalf("still valid: %v", err)
	}
	if err := st.RevokeSession(ctx, u1, s3.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.LookupSession(ctx, tok3); !errors.Is(err, accounts.ErrSessionInvalid) {
		t.Fatalf("revoked: %v", err)
	}
	if list, _ := st.ListSessions(ctx, u1); len(list) != 0 {
		t.Fatalf("revoked listed: %+v", list)
	}

	// deleting user
	u3 := newUser("c@example.com", "deleting")
	tok4, _ := create(u3)
	if _, _, err := st.LookupSession(ctx, tok4); !errors.Is(err, accounts.ErrSessionInvalid) {
		t.Fatalf("deleting user: %v", err)
	}

	// long user agent is truncated, not rejected
	tx, _ := pool.Begin(ctx)
	long := string(make([]byte, 0))
	for i := 0; i < 400; i++ {
		long += "x"
	}
	if _, _, err := st.CreateSession(ctx, tx, u2, "", long); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
}

type fakeLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	calls  []string
}

func (f *fakeLimiter) AllowWindow(_ context.Context, subject, category string, limit int, _ time.Duration) (bool, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	f.calls = append(f.calls, category)
	k := category + ":" + subject
	if f.counts[k] >= limit {
		return false, 1234, nil
	}
	f.counts[k]++
	return true, 0, nil
}

func TestMagicLinkEmailNormalisation(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k")}
	mailer := &mail.LogMailer{}
	cfg := accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: &fakeLimiter{}, PerEmailPerHour: 5, PerIPPerHour: 100}
	for i := 0; i < 5; i++ {
		raw := "bo@example.com"
		if i%2 == 0 {
			raw = "  Bo@Example.COM "
		}
		if err := st.RequestMagicLink(ctx, cfg, raw, "1.2.3.4"); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	for _, m := range mailer.Sent {
		if m.To != "bo@example.com" {
			t.Fatalf("sent to %q", m.To)
		}
	}
	err := st.RequestMagicLink(ctx, cfg, "bo@example.com", "1.2.3.4")
	var rl accounts.ErrRateLimited
	if !errors.As(err, &rl) || rl.RetryAfter != 1234 {
		t.Fatalf("6th: %v", err)
	}
	if len(mailer.Sent) != 5 {
		t.Fatalf("limited request sent mail: %d", len(mailer.Sent))
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM magic_links`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("links stored: %d %v", n, err)
	}
	if err := st.RequestMagicLink(ctx, cfg, "not an email", "1.2.3.4"); !errors.Is(err, accounts.ErrInvalidEmail) {
		t.Fatalf("invalid: %v", err)
	}
	// per-IP limit is checked first
	fl := &fakeLimiter{}
	cfg2 := cfg
	cfg2.Limiter, cfg2.PerIPPerHour = fl, 1
	_ = st.RequestMagicLink(ctx, cfg2, "x@example.com", "9.9.9.9")
	if err := st.RequestMagicLink(ctx, cfg2, "y@example.com", "9.9.9.9"); !errors.As(err, &rl) {
		t.Fatalf("ip limit: %v", err)
	}
	if fl.calls[0] != "magic_link_ip" || fl.calls[1] != "magic_link_email" || fl.calls[2] != "magic_link_ip" || len(fl.calls) != 3 {
		t.Fatalf("order %v", fl.calls)
	}
}

func TestMagicLinkConsume(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	clock := time.Now().UTC().Truncate(time.Second)
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k"), Now: func() time.Time { return clock }}
	mailer := &mail.LogMailer{}
	cfg := accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: &fakeLimiter{}, PerEmailPerHour: 50, PerIPPerHour: 50}
	tokenOf := func(i int) string {
		t.Helper()
		m := mailer.Sent[i]
		_, tok, ok := strings.Cut(m.Text, "/auth/email?token=")
		if !ok || len(tok) < 47 {
			t.Fatalf("no link in %q", m.Text)
		}
		return tok[:47]
	}
	// unknown email: nil + mail sent
	if err := st.RequestMagicLink(ctx, cfg, "new@example.com", "1.1.1.1"); err != nil || len(mailer.Sent) != 1 {
		t.Fatalf("request: %v sent=%d", err, len(mailer.Sent))
	}
	tok := tokenOf(0)
	if tok[:4] != "rfm_" {
		t.Fatal(tok)
	}
	sess, _, u, err := st.ConsumeMagicLink(ctx, tok, "1.1.1.1", "ua")
	if err != nil || u.Email != "new@example.com" || !u.EmailVerified || u.Kind != "regular" || u.DisplayName != "new" || sess[:4] != "rfs_" {
		t.Fatalf("consume: %v %+v", err, u)
	}
	var users, sessions, idents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email='new@example.com'`).Scan(&users)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1`, u.ID).Scan(&sessions)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id=$1 AND provider='email' AND provider_subject='new@example.com'`, u.ID).Scan(&idents)
	if users != 1 || sessions != 1 || idents != 1 {
		t.Fatalf("users=%d sessions=%d idents=%d", users, sessions, idents)
	}
	if _, _, _, err := st.ConsumeMagicLink(ctx, tok, "1.1.1.1", "ua"); !errors.Is(err, accounts.ErrLinkInvalid) {
		t.Fatalf("reuse: %v", err)
	}
	if _, _, _, err := st.ConsumeMagicLink(ctx, "rfm_short", "", ""); !errors.Is(err, accounts.ErrLinkInvalid) {
		t.Fatalf("malformed: %v", err)
	}
	// expiry
	_ = st.RequestMagicLink(ctx, cfg, "new@example.com", "1.1.1.1")
	exp := tokenOf(1)
	clock = clock.Add(16 * time.Minute)
	if _, _, _, err := st.ConsumeMagicLink(ctx, exp, "", ""); !errors.Is(err, accounts.ErrLinkInvalid) {
		t.Fatalf("expired: %v", err)
	}
	// existing user signs in again: same user
	clock = clock.Add(-16 * time.Minute)
	_ = st.RequestMagicLink(ctx, cfg, "new@example.com", "1.1.1.1")
	_, _, u2, err := st.ConsumeMagicLink(ctx, tokenOf(2), "", "")
	if err != nil || u2.ID != u.ID {
		t.Fatalf("existing: %v %+v", err, u2)
	}
	// concurrent consumes
	_ = st.RequestMagicLink(ctx, cfg, "race@example.com", "1.1.1.1")
	race := tokenOf(3)
	var wg sync.WaitGroup
	var okCount, invalid atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, err := st.ConsumeMagicLink(ctx, race, "", "")
			switch {
			case err == nil:
				okCount.Add(1)
			case errors.Is(err, accounts.ErrLinkInvalid):
				invalid.Add(1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	var rs int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sessions s JOIN users u ON u.id=s.user_id WHERE u.email='race@example.com'`).Scan(&rs)
	if okCount.Load() != 1 || invalid.Load() != 7 || rs != 1 {
		t.Fatalf("ok=%d invalid=%d sessions=%d", okCount.Load(), invalid.Load(), rs)
	}
}

func TestMagicLinkEmptyIPFailsClosed(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	mailer := &mail.LogMailer{}
	cfg := accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: &fakeLimiter{}, PerEmailPerHour: 5, PerIPPerHour: 20}
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k")}
	for i := 0; i < 20; i++ {
		if err := st.RequestMagicLink(ctx, cfg, fmt.Sprintf("u%d@example.com", i), ""); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	var rl accounts.ErrRateLimited
	if err := st.RequestMagicLink(ctx, cfg, "u21@example.com", ""); !errors.As(err, &rl) {
		t.Fatalf("21st: %v", err)
	}
	if len(mailer.Sent) != 20 {
		t.Fatalf("sent %d", len(mailer.Sent))
	}
	nokey := &accounts.Store{DB: pool}
	if err := nokey.RequestMagicLink(ctx, cfg, "k@example.com", "1.2.3.4"); err == nil || errors.As(err, &rl) {
		t.Fatalf("expected refusal without key: %v", err)
	}
}

func TestGitHubLinksByVerifiedEmailOnly(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k")}
	identities := func(uid string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id=$1`, uid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// magic-link user, then GitHub with the same verified primary email
	mailer := &mail.LogMailer{}
	cfg := accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: &fakeLimiter{}, PerEmailPerHour: 5, PerIPPerHour: 100}
	if err := st.RequestMagicLink(ctx, cfg, "bo@x.io", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	link := mailer.Sent[0].Text[strings.Index(mailer.Sent[0].Text, "token=")+6:]
	if i := strings.IndexAny(link, " \n"); i >= 0 {
		link = link[:i]
	}
	_, _, mlUser, err := st.ConsumeMagicLink(ctx, link, "1.2.3.4", "ua")
	if err != nil {
		t.Fatal(err)
	}
	p := accounts.GitHubProfile{ID: 7, Login: "bo", Name: "Bo", AvatarURL: "https://a/1", VerifiedPrimaryEmail: "Bo@X.io"}
	tok, _, u, err := st.SignInWithGitHub(ctx, p, "1.2.3.4", "ua")
	if err != nil || u.ID != mlUser.ID || tok == "" {
		t.Fatalf("link: %v %+v", err, u)
	}
	if n := identities(u.ID); n != 2 {
		t.Fatalf("identities %d", n)
	}
	if u.DisplayName != "Bo" || u.AvatarURL != "https://a/1" {
		t.Fatalf("profile not refreshed: %+v", u)
	}

	// second sign-in: same user, refreshed profile, even without email
	p.Name, p.VerifiedPrimaryEmail = "", ""
	_, _, u2, err := st.SignInWithGitHub(ctx, p, "1.2.3.4", "ua")
	if err != nil || u2.ID != u.ID || u2.DisplayName != "bo" || identities(u.ID) != 2 {
		t.Fatalf("second: %v %+v", err, u2)
	}

	// no verified email on a new identity
	if _, _, _, err := st.SignInWithGitHub(ctx, accounts.GitHubProfile{ID: 8, Login: "z"}, "1.2.3.4", "ua"); !errors.Is(err, accounts.ErrGitHubEmailUnverified) {
		t.Fatalf("empty email: %v", err)
	}

	// unverified existing user is neither linked nor duplicated
	uid, _ := id.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1,'un@x.io')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.SignInWithGitHub(ctx, accounts.GitHubProfile{ID: 9, Login: "un", VerifiedPrimaryEmail: "un@x.io"}, "1.2.3.4", "ua"); !errors.Is(err, accounts.ErrGitHubEmailUnverified) {
		t.Fatalf("unverified: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email='un@x.io'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("users: %d %v", n, err)
	}
	if identities(uid) != 0 {
		t.Fatal("unverified user got linked")
	}

	// brand-new GitHub user is created verified
	_, _, nu, err := st.SignInWithGitHub(ctx, accounts.GitHubProfile{ID: 10, Login: "new", VerifiedPrimaryEmail: "new@x.io"}, "1.2.3.4", "ua")
	if err != nil || !nu.EmailVerified || nu.Email != "new@x.io" {
		t.Fatalf("create: %v %+v", err, nu)
	}
}

func TestGitHubLongProfileFieldsTruncated(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k")}
	long := strings.Repeat("é", 200)
	p := accounts.GitHubProfile{ID: 21, Login: "l", Name: long, AvatarURL: "https://a/" + strings.Repeat("x", 3000), VerifiedPrimaryEmail: "long@x.io"}
	for i := 0; i < 2; i++ { // create, then refresh
		_, _, u, err := st.SignInWithGitHub(ctx, p, "1.2.3.4", "ua")
		if err != nil {
			t.Fatalf("sign-in %d: %v", i, err)
		}
		var dn, av string
		if err := pool.QueryRow(ctx, `SELECT display_name, avatar_url FROM users WHERE id=$1`, u.ID).Scan(&dn, &av); err != nil {
			t.Fatal(err)
		}
		if len([]rune(dn)) != 120 || len([]rune(av)) != 2048 {
			t.Fatalf("lens %d %d", len([]rune(dn)), len([]rune(av)))
		}
	}
}

func TestMagicLinkIdentityConflict(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	st := &accounts.Store{DB: pool, IPHashKey: []byte("k")}
	a, _ := id.New()
	b, _ := id.New()
	for _, q := range [][]any{{a, "a@x.io"}, {b, "c@x.io"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, email_verified_at) VALUES ($1,$2,now())`, q...); err != nil {
			t.Fatal(err)
		}
	}
	iid, _ := id.New()
	if _, err := pool.Exec(ctx, `INSERT INTO user_identities (id, user_id, provider, provider_subject) VALUES ($1,$2,'email','c@x.io')`, iid, a); err != nil {
		t.Fatal(err)
	}
	mailer := &mail.LogMailer{}
	cfg := accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: &fakeLimiter{}, PerEmailPerHour: 5, PerIPPerHour: 100}
	if err := st.RequestMagicLink(ctx, cfg, "c@x.io", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	_, tok, _ := strings.Cut(mailer.Sent[0].Text, "/auth/email?token=")
	_, _, _, err := st.ConsumeMagicLink(ctx, tok[:47], "1.2.3.4", "ua")
	if !errors.Is(err, accounts.ErrIdentityConflict) {
		t.Fatalf("err %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sessions %d %v", n, err)
	}
}

type failingMailer struct{}

func (failingMailer) Send(context.Context, mail.Message) error {
	return errors.New("smtp refused recipient nobody@example.com")
}

func TestAuthRoutes(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	limiter := startRedisLimiter(t)
	const dashToken = "test-dashboard-token-0123456789abcdef"
	ipKey := bytes.Repeat([]byte{7}, 32)
	mailer := &mail.LogMailer{}
	store := &accounts.Store{DB: pool, IPHashKey: ipKey}
	magic := accounts.MagicLinkConfig{DashboardURL: "https://app.example", Mailer: mailer, Limiter: limiter, PerEmailPerHour: 5, PerIPPerHour: 20}
	newServer := func(s *api.Server) string {
		s.DB = pool
		s.EncryptionKey = bytes.Repeat([]byte{1}, 32)
		s.EncryptionKeyID = "test"
		srv := httptest.NewServer(s.Router())
		t.Cleanup(srv.Close)
		return srv.URL
	}
	base := newServer(&api.Server{Accounts: store, MagicLinks: magic, DashboardToken: dashToken})
	call := func(t *testing.T, url, method, path, token, ip, body string) (*http.Response, map[string]any) {
		t.Helper()
		r, err := http.NewRequest(method, url+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			r.Header.Set("X-RelayForge-Dashboard-Token", token)
		}
		if ip != "" {
			r.Header.Set("X-RelayForge-Client-IP", ip)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		raw, _ := io.ReadAll(resp.Body)
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatalf("%s %s: bad json %q", method, path, raw)
			}
		}
		return resp, out
	}
	expect := func(t *testing.T, resp *http.Response, body map[string]any, status int, code string) {
		t.Helper()
		if resp.StatusCode != status || (code != "" && body["code"] != code) {
			t.Fatalf("status=%d body=%v; want %d %s", resp.StatusCode, body, status, code)
		}
	}
	lastLink := func(t *testing.T, m *mail.LogMailer) string {
		t.Helper()
		msgs := m.Messages()
		if len(msgs) == 0 {
			t.Fatal("no mail sent")
		}
		_, tok, ok := strings.Cut(msgs[len(msgs)-1].Text, "/auth/email?token=")
		if !ok || len(tok) < 47 {
			t.Fatalf("no link in %q", msgs[len(msgs)-1].Text)
		}
		return tok[:47]
	}
	signIn := func(t *testing.T, email string) map[string]any {
		t.Helper()
		resp, body := call(t, base, "POST", "/v1/auth/email/start", dashToken, "", `{"email":"`+email+`"}`)
		expect(t, resp, body, 202, "")
		resp, body = call(t, base, "POST", "/v1/auth/email/consume", dashToken, "", `{"token":"`+lastLink(t, mailer)+`"}`)
		expect(t, resp, body, 200, "")
		return body
	}

	t.Run("dashboard token required", func(t *testing.T) {
		for _, tc := range []struct{ method, path, token string }{
			{"GET", "/v1/auth/providers", ""},
			{"GET", "/v1/auth/providers", dashToken + "x"},
			{"POST", "/v1/auth/email/start", ""},
			{"POST", "/v1/auth/email/consume", ""},
			{"POST", "/v1/auth/github/start", ""},
			{"POST", "/v1/auth/github/callback", ""},
		} {
			resp, body := call(t, base, tc.method, tc.path, tc.token, "", `{}`)
			expect(t, resp, body, 401, "dashboard_token_required")
		}
	})

	t.Run("providers and disabled github", func(t *testing.T) {
		resp, body := call(t, base, "GET", "/v1/auth/providers", dashToken, "", "")
		expect(t, resp, body, 200, "")
		if body["github"] != false || body["email"] != true || len(body) != 2 {
			t.Fatalf("providers %v", body)
		}
		resp, body = call(t, base, "POST", "/v1/auth/github/start", dashToken, "", "")
		expect(t, resp, body, 404, "provider_disabled")
	})

	t.Run("email sign-in, single use and audit", func(t *testing.T) {
		org, _ := tenantKey(t, pool, "auth-org")
		other, _ := tenantKey(t, pool, "auth-org-suspended")
		uid, _ := id.New()
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1,'member@example.com')`, uid); err != nil {
			t.Fatal(err)
		}
		for _, m := range [][]any{{org, "operator", "active"}, {other, "viewer", "suspended"}} {
			mid, _ := id.New()
			if _, err := pool.Exec(ctx, `INSERT INTO team_members (id, organization_id, name, email, role, status, user_id) VALUES ($1,$2,'M','member@example.com',$3,$4,$5)`, mid, m[0], m[1], m[2], uid); err != nil {
				t.Fatal(err)
			}
		}
		resp, body := call(t, base, "POST", "/v1/auth/email/start", dashToken, "", `{"email":"  Member@Example.com "}`)
		expect(t, resp, body, 202, "")
		if len(body) != 0 {
			t.Fatalf("start body %v", body)
		}
		link := lastLink(t, mailer)
		resp, body = call(t, base, "POST", "/v1/auth/email/consume", dashToken, "", `{"token":"`+link+`"}`)
		expect(t, resp, body, 200, "")
		tok, _ := body["session_token"].(string)
		if !strings.HasPrefix(tok, "rfs_") {
			t.Fatalf("session token %v", body)
		}
		exp, err := time.Parse(time.RFC3339, body["expires_at"].(string))
		if err != nil || exp.Sub(time.Now()) < 29*24*time.Hour || exp.Sub(time.Now()) > 31*24*time.Hour {
			t.Fatalf("expires_at %v %v", body["expires_at"], err)
		}
		user, _ := body["user"].(map[string]any)
		if user["id"] != uid || user["email"] != "member@example.com" || len(user) != 3 {
			t.Fatalf("user %v", user)
		}
		if _, u, err := store.LookupSession(ctx, tok); err != nil || u.ID != uid {
			t.Fatalf("lookup %v %+v", err, u)
		}
		var n int
		var role, target, targetType, method string
		if err := pool.QueryRow(ctx, `SELECT count(*) OVER (), actor_role, target_id::text, target_type, details->>'method' FROM audit_events
			WHERE action='user.signed_in' AND actor_user_id=$1 AND organization_id=$2`, uid, org).Scan(&n, &role, &target, &targetType, &method); err != nil {
			t.Fatal(err)
		}
		if n != 1 || role != "operator" || target != uid || targetType != "user" || method != "email" {
			t.Fatalf("audit n=%d role=%s target=%s type=%s method=%s", n, role, target, targetType, method)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='user.signed_in' AND organization_id=$1`, other).Scan(&n); err != nil || n != 0 {
			t.Fatalf("suspended membership audited: %d %v", n, err)
		}

		resp, body = call(t, base, "POST", "/v1/auth/email/consume", dashToken, "", `{"token":"`+link+`"}`)
		expect(t, resp, body, 400, "link_invalid")
		resp, body = call(t, base, "POST", "/v1/auth/email/consume", dashToken, "", `{"token":"rfm_nope"}`)
		expect(t, resp, body, 400, "link_invalid")
	})

	t.Run("org-less user writes no audit", func(t *testing.T) {
		body := signIn(t, "loner@example.com")
		uid := body["user"].(map[string]any)["id"]
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_user_id=$1`, uid).Scan(&n); err != nil || n != 0 {
			t.Fatalf("audit rows %d %v", n, err)
		}
	})

	t.Run("invalid email and json", func(t *testing.T) {
		resp, body := call(t, base, "POST", "/v1/auth/email/start", dashToken, "", `{"email":"not an email"}`)
		expect(t, resp, body, 422, "invalid_email")
		resp, body = call(t, base, "POST", "/v1/auth/email/start", dashToken, "", `{"email":"a@b.co","x":1}`)
		expect(t, resp, body, 400, "invalid_json")
	})

	t.Run("per-ip magic link limit", func(t *testing.T) {
		const ip = "203.0.113.50"
		for i := 1; i <= 20; i++ {
			resp, body := call(t, base, "POST", "/v1/auth/email/start", dashToken, ip, fmt.Sprintf(`{"email":"ip%d@example.com"}`, i))
			expect(t, resp, body, 202, "")
		}
		resp, body := call(t, base, "POST", "/v1/auth/email/start", dashToken, ip, `{"email":"ip21@example.com"}`)
		expect(t, resp, body, 429, "rate_limited")
		if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err != nil || ra <= 0 || ra > 3600 {
			t.Fatalf("Retry-After %q", resp.Header.Get("Retry-After"))
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM magic_links WHERE requested_ip_hash=$1`, store.HashIP(ip)).Scan(&n); err != nil || n != 20 {
			t.Fatalf("links for ip %d %v", n, err)
		}
		// Another client IP is unaffected.
		resp, body = call(t, base, "POST", "/v1/auth/email/start", dashToken, "203.0.113.51", `{"email":"ip21@example.com"}`)
		expect(t, resp, body, 202, "")
	})

	t.Run("mail failure still 202", func(t *testing.T) {
		m := magic
		m.Mailer = failingMailer{}
		u := newServer(&api.Server{Accounts: store, MagicLinks: m, DashboardToken: dashToken})
		resp, body := call(t, u, "POST", "/v1/auth/email/start", dashToken, "198.51.100.1", `{"email":"fails@example.com"}`)
		expect(t, resp, body, 202, "")
	})

	t.Run("disabled and no limiter", func(t *testing.T) {
		off := newServer(&api.Server{Accounts: &accounts.Store{DB: pool}, MagicLinks: magic, DashboardToken: dashToken})
		resp, body := call(t, off, "GET", "/v1/auth/providers", dashToken, "", "")
		expect(t, resp, body, 200, "")
		if body["github"] != false || body["email"] != false {
			t.Fatalf("providers %v", body)
		}
		for _, p := range []string{"/v1/auth/email/start", "/v1/auth/email/consume", "/v1/auth/github/start", "/v1/auth/github/callback"} {
			resp, body := call(t, off, "POST", p, dashToken, "", `{}`)
			expect(t, resp, body, 404, "provider_disabled")
		}
		m := magic
		m.Limiter = nil
		gh := &accounts.GitHubConfig{ClientID: "cid", ClientSecret: "sec", RedirectURL: "https://app.example/auth/github/callback"}
		nolim := newServer(&api.Server{Accounts: store, MagicLinks: m, GitHub: gh, DashboardToken: dashToken})
		resp, body = call(t, nolim, "POST", "/v1/auth/email/start", dashToken, "", `{"email":"nolim@example.com"}`)
		expect(t, resp, body, 503, "rate_limit_unavailable")
		resp, body = call(t, nolim, "POST", "/v1/auth/github/start", dashToken, "", "")
		expect(t, resp, body, 503, "rate_limit_unavailable")
	})

	t.Run("github", func(t *testing.T) {
		var ghID atomic.Int64
		var unverified atomic.Bool
		ghID.Store(777)
		mux := http.NewServeMux()
		mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			if r.PostForm.Get("code") != "good" {
				_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok"}`))
		})
		mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `{"id":%d,"login":"ghuser","name":"GH User"}`, ghID.Load())
		})
		mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `[{"email":"gh%d@example.com","primary":true,"verified":%t}]`, ghID.Load(), !unverified.Load())
		})
		fake := httptest.NewServer(mux)
		t.Cleanup(fake.Close)
		gh := &accounts.GitHubConfig{ClientID: "cid", ClientSecret: "sec", RedirectURL: "https://app.example/auth/github/callback", AuthBase: fake.URL, APIBase: fake.URL}
		u := newServer(&api.Server{Accounts: store, MagicLinks: magic, GitHub: gh, DashboardToken: dashToken, OAuthStartPerIPPerHour: 2})
		resp, body := call(t, u, "GET", "/v1/auth/providers", dashToken, "", "")
		if body["github"] != true || body["email"] != true {
			t.Fatalf("providers %v", body)
		}
		resp, body = call(t, u, "POST", "/v1/auth/github/start", dashToken, "198.51.100.7", "")
		expect(t, resp, body, 200, "")
		authURL, _ := body["authorize_url"].(string)
		if !strings.HasPrefix(authURL, fake.URL+"/login/oauth/authorize?") || body["state"] == "" || body["verifier"] == "" {
			t.Fatalf("start %v", body)
		}
		resp, body = call(t, u, "POST", "/v1/auth/github/start", dashToken, "198.51.100.7", "")
		expect(t, resp, body, 200, "")
		resp, body = call(t, u, "POST", "/v1/auth/github/start", dashToken, "198.51.100.7", "")
		expect(t, resp, body, 429, "rate_limited")
		if resp.Header.Get("Retry-After") == "" {
			t.Fatal("missing Retry-After")
		}

		resp, body = call(t, u, "POST", "/v1/auth/github/callback", dashToken, "", `{"code":"bad","verifier":"v"}`)
		expect(t, resp, body, 400, "github_failed")
		resp, body = call(t, u, "POST", "/v1/auth/github/callback", dashToken, "", `{"code":"","verifier":""}`)
		expect(t, resp, body, 400, "github_failed")
		resp, body = call(t, u, "POST", "/v1/auth/github/callback", dashToken, "", `{"code":"good","verifier":"v"}`)
		expect(t, resp, body, 200, "")
		if tok, _ := body["session_token"].(string); !strings.HasPrefix(tok, "rfs_") {
			t.Fatalf("callback %v", body)
		}
		if user := body["user"].(map[string]any); user["email"] != "gh777@example.com" || user["display_name"] != "GH User" {
			t.Fatalf("user %v", user)
		}

		ghID.Store(778)
		unverified.Store(true)
		resp, body = call(t, u, "POST", "/v1/auth/github/callback", dashToken, "", `{"code":"good","verifier":"v"}`)
		expect(t, resp, body, 403, "github_email_unverified")
	})
}

// consoleEnv is an API server with accounts configured, plus helpers to create
// session users and call the API with an org header.
type consoleEnv struct {
	t     *testing.T
	url   string
	store *accounts.Store
}

func newConsoleEnv(t *testing.T, pool *pgxpool.Pool, s *api.Server) *consoleEnv {
	t.Helper()
	store := &accounts.Store{DB: pool, IPHashKey: bytes.Repeat([]byte{7}, 32)}
	s.DB = pool
	s.Accounts = store
	s.EncryptionKey = bytes.Repeat([]byte{1}, 32)
	s.EncryptionKeyID = "test"
	srv := httptest.NewServer(s.Router())
	t.Cleanup(srv.Close)
	return &consoleEnv{t: t, url: srv.URL, store: store}
}

// sessionUser inserts a user and returns its id and a live session token.
func (e *consoleEnv) sessionUser(email, displayName string) (string, string) {
	e.t.Helper()
	ctx := context.Background()
	uid, _ := id.New()
	var dn any
	if displayName != "" {
		dn = displayName
	}
	if _, err := e.store.DB.Exec(ctx, `INSERT INTO users (id, email, email_verified_at, display_name) VALUES ($1,$2,now(),$3)`, uid, email, dn); err != nil {
		e.t.Fatal(err)
	}
	tx, err := e.store.DB.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	tok, _, err := e.store.CreateSession(ctx, tx, uid, "1.2.3.4", "test-agent")
	if err != nil {
		_ = tx.Rollback(ctx)
		e.t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		e.t.Fatal(err)
	}
	return uid, tok
}

func (e *consoleEnv) do(method, path, bearer, org, body string) (int, map[string]any) {
	e.t.Helper()
	r, err := http.NewRequest(method, e.url+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if org != "" {
		r.Header.Set("X-RelayForge-Org", org)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(raw) > 0 {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			e.t.Fatalf("%s %s: bad json %q", method, path, raw)
		}
		if m, ok := v.(map[string]any); ok {
			out = m
		} else {
			out["_"] = v
		}
	}
	return resp.StatusCode, out
}

func (e *consoleEnv) expect(method, path, bearer, org, body string, status int, code string) map[string]any {
	e.t.Helper()
	got, out := e.do(method, path, bearer, org, body)
	if got != status || (code != "" && out["code"] != code) {
		e.t.Fatalf("%s %s: status=%d body=%v; want %d %s", method, path, got, out, status, code)
	}
	return out
}

func TestSessionOrgHeaderRequiresActiveMembership(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	env := newConsoleEnv(t, pool, &api.Server{})
	uid, tok := env.sessionUser("ada@example.com", "")

	env.expect("GET", "/v1/endpoints", tok, "", "", 400, "org_required")
	me := env.expect("GET", "/v1/me", tok, "", "", 200, "")
	if me["user_id"] != uid || me["role"] != "" || len(me["scopes"].([]any)) != 0 {
		t.Fatalf("org-less me %v", me)
	}

	foreign, key := tenantKey(t, pool, "not-mine")
	env.expect("GET", "/v1/endpoints", tok, foreign, "", 403, "not_a_member")
	env.expect("GET", "/v1/endpoints", tok, "not-a-uuid", "", 400, "invalid_org")

	ws := env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"  Ada's workspace  "}`, 201, "")
	if ws["name"] != "Ada's workspace" || ws["role"] != "owner" || ws["plan"] != "free" {
		t.Fatalf("workspace %v", ws)
	}
	org := ws["id"].(string)
	env.expect("GET", "/v1/endpoints", tok, org, "", 200, "")
	me = env.expect("GET", "/v1/me", tok, org, "", 200, "")
	if me["user_id"] != uid || me["role"] != "owner" || len(me["scopes"].([]any)) != 11 {
		t.Fatalf("owner me %v", me)
	}

	acct := env.expect("GET", "/v1/account", tok, "", "", 200, "")
	user := acct["user"].(map[string]any)
	wss := acct["workspaces"].([]any)
	if user["id"] != uid || user["email"] != "ada@example.com" || len(wss) != 1 || wss[0].(map[string]any)["id"] != org || wss[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("account %v", acct)
	}

	if _, err := pool.Exec(ctx, `UPDATE team_members SET status='suspended' WHERE user_id=$1 AND organization_id=$2`, uid, org); err != nil {
		t.Fatal(err)
	}
	env.expect("GET", "/v1/endpoints", tok, org, "", 403, "not_a_member")
	if _, err := pool.Exec(ctx, `UPDATE team_members SET status='active' WHERE user_id=$1 AND organization_id=$2`, uid, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organizations SET status='suspended' WHERE id=$1`, org); err != nil {
		t.Fatal(err)
	}
	env.expect("GET", "/v1/endpoints", tok, org, "", 403, "not_a_member")

	// A stale org header never blocks account routes or /v1/me.
	for _, stale := range []string{org, foreign} {
		if a := env.expect("GET", "/v1/account", tok, stale, "", 200, ""); len(a["workspaces"].([]any)) != 0 {
			t.Fatalf("suspended org listed %v", a)
		}
		if m := env.expect("GET", "/v1/me", tok, stale, "", 200, ""); m["role"] != "" || m["user_id"] != uid {
			t.Fatalf("stale me %v", m)
		}
		env.expect("GET", "/v1/account/sessions", tok, stale, "", 200, "")
	}

	env.expect("GET", "/v1/account", key, "", "", 403, "session_required")
	env.expect("GET", "/v1/endpoints", key, "", "", 200, "")

	// An API key's org header is ignored: it only ever sees its own org.
	ep := env.expect("POST", "/v1/endpoints", key, "", `{"url":"https://8.8.8.8/webhooks","event_types":["order.created"]}`, 201, "")
	epID := ep["endpoint"].(map[string]any)["id"]
	other, _ := tenantKey(t, pool, "other-with-endpoint")
	for _, hdr := range []string{org, other} {
		list := env.expect("GET", "/v1/endpoints", key, hdr, "", 200, "")
		data, _ := list["data"].([]any)
		if len(data) != 1 || data[0].(map[string]any)["id"] != epID {
			t.Fatalf("key with header %s saw %v", hdr, list)
		}
	}
	var otherEndpoints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM endpoints WHERE organization_id IN ($1,$2)`, org, other).Scan(&otherEndpoints); err != nil || otherEndpoints != 0 {
		t.Fatalf("endpoint created in wrong org: %d %v", otherEndpoints, err)
	}
	env.expect("GET", "/v1/account", "rfs_"+strings.Repeat("x", 43), "", "", 401, "unauthorized")

	// accounts not configured: session bearers are rejected
	// Sign-out still works with a stale header.
	env.expect("POST", "/v1/account/sign-out", tok, org, "", 204, "")
	env.expect("GET", "/v1/account", tok, "", "", 401, "unauthorized")

	off := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: bytes.Repeat([]byte{1}, 32), EncryptionKeyID: "test"}).Router())
	t.Cleanup(off.Close)
	resp, _ := request(t, "GET", off.URL+"/v1/me", tok, "")
	if resp.StatusCode != 401 {
		t.Fatalf("accounts off: %d", resp.StatusCode)
	}
}

func TestCreateWorkspaceLimit(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	env := newConsoleEnv(t, pool, &api.Server{})
	uid, tok := env.sessionUser("grace.hopper@example.com", "")

	env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"   "}`, 422, "invalid_workspace")
	env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"`+strings.Repeat("a", 81)+`"}`, 422, "invalid_workspace")
	for i := 1; i <= 3; i++ {
		ws := env.expect("POST", "/v1/account/workspaces", tok, "", fmt.Sprintf(`{"name":"ws %d"}`, i), 201, "")
		org := ws["id"].(string)
		var name, email, role, status string
		if err := pool.QueryRow(ctx, `SELECT name,email,role,status FROM team_members WHERE organization_id=$1 AND user_id=$2`, org, uid).Scan(&name, &email, &role, &status); err != nil {
			t.Fatal(err)
		}
		if name != "grace.hopper" || email != "grace.hopper@example.com" || role != "owner" || status != "active" {
			t.Fatalf("member %s %s %s %s", name, email, role, status)
		}
		var n int
		var actorRole, targetType, target string
		if err := pool.QueryRow(ctx, `SELECT count(*) OVER (), actor_role, target_type, target_id::text FROM audit_events WHERE organization_id=$1 AND action='organization.created_by_user' AND actor_user_id=$2 AND actor_api_key_id IS NULL`, org, uid).Scan(&n, &actorRole, &targetType, &target); err != nil {
			t.Fatal(err)
		}
		if n != 1 || actorRole != "owner" || targetType != "organization" || target != org {
			t.Fatalf("audit %d %s %s %s", n, actorRole, targetType, target)
		}
		var quota, maxEndpoints int
		var createdBy string
		if err := pool.QueryRow(ctx, `SELECT monthly_event_quota, max_endpoints, created_by_user_id::text FROM organizations WHERE id=$1`, org).Scan(&quota, &maxEndpoints, &createdBy); err != nil {
			t.Fatal(err)
		}
		if quota != 10000 || maxEndpoints != 10 || createdBy != uid {
			t.Fatalf("org %d %d %s", quota, maxEndpoints, createdBy)
		}
	}
	env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"ws 4"}`, 429, "workspace_limit")
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE created_by_user_id=$1`, uid).Scan(&n); err != nil || n != 3 {
		t.Fatalf("orgs %d %v", n, err)
	}

	// concurrent creations never exceed the limit
	uid2, tok2 := env.sessionUser("race@example.com", "Racer")
	var wg sync.WaitGroup
	var created atomic.Int32
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, _ := env.do("POST", "/v1/account/workspaces", tok2, "", fmt.Sprintf(`{"name":"r%d"}`, i))
			if got == 201 {
				created.Add(1)
			} else if got != 429 {
				t.Errorf("concurrent status %d", got)
			}
		}(i)
	}
	wg.Wait()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE created_by_user_id=$1`, uid2).Scan(&n); err != nil || n != 3 || created.Load() != 3 {
		t.Fatalf("concurrent orgs=%d created=%d %v", n, created.Load(), err)
	}
	var memberName string
	if err := pool.QueryRow(ctx, `SELECT name FROM team_members WHERE user_id=$1 LIMIT 1`, uid2).Scan(&memberName); err != nil || memberName != "Racer" {
		t.Fatalf("display name member %q %v", memberName, err)
	}

	// configured limit
	env2 := newConsoleEnv(t, pool, &api.Server{MaxWorkspacesPerUser: 1})
	_, tok3 := env2.sessionUser("one@example.com", "")
	env2.expect("POST", "/v1/account/workspaces", tok3, "", `{"name":"only"}`, 201, "")
	env2.expect("POST", "/v1/account/workspaces", tok3, "", `{"name":"second"}`, 429, "workspace_limit")
}

func TestSignOutRevokesSession(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	env := newConsoleEnv(t, pool, &api.Server{})
	uid, tok := env.sessionUser("out@example.com", "")
	_, tokOther := env.sessionUser("other@example.com", "")
	ws := env.expect("POST", "/v1/account/workspaces", tok, "", `{"name":"w"}`, 201, "")

	// a second session for the same user
	tx, _ := pool.Begin(ctx)
	tok2, sess2, err := env.store.CreateSession(ctx, tx, uid, "", "second-agent")
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)

	_, list := env.do("GET", "/v1/account/sessions", tok, "", "")
	items, _ := list["data"].([]any)
	if len(items) != 2 {
		t.Fatalf("sessions %v", list)
	}
	currents := 0
	for _, it := range items {
		m := it.(map[string]any)
		for _, k := range []string{"id", "created_at", "last_seen_at", "expires_at", "user_agent", "current"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("missing %s in %v", k, m)
			}
		}
		if m["current"] == true {
			currents++
			if m["id"] == sess2.ID || m["user_agent"] != "test-agent" {
				t.Fatalf("wrong current %v", m)
			}
		}
	}
	if currents != 1 {
		t.Fatalf("current count %d", currents)
	}

	// another user's session id is not found; unknown and malformed ids too
	env.expect("DELETE", "/v1/account/sessions/"+sess2.ID, tokOther, "", "", 404, "not_found")
	unknown, _ := id.New()
	env.expect("DELETE", "/v1/account/sessions/"+unknown, tok, "", "", 404, "not_found")
	env.expect("DELETE", "/v1/account/sessions/nope", tok, "", "", 404, "not_found")
	env.expect("DELETE", "/v1/account/sessions/"+sess2.ID, tok, "", "", 204, "")
	env.expect("GET", "/v1/account", tok2, "", "", 401, "unauthorized")
	var revoked int
	var revokedRole string
	if err := pool.QueryRow(ctx, `SELECT count(*) OVER (), actor_role FROM audit_events WHERE action='session.revoked' AND target_type='session' AND target_id=$1 AND actor_user_id=$2 AND organization_id=$3`, sess2.ID, uid, ws["id"]).Scan(&revoked, &revokedRole); err != nil || revoked != 1 || revokedRole != "owner" {
		t.Fatalf("session.revoked audit %d %s %v", revoked, revokedRole, err)
	}

	env.expect("POST", "/v1/account/sign-out", tok, "", "", 204, "")
	env.expect("GET", "/v1/account", tok, "", "", 401, "unauthorized")
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='user.signed_out' AND actor_user_id=$1 AND organization_id=$2 AND actor_role='owner'`, uid, ws["id"]).Scan(&n); err != nil || n != 1 {
		t.Fatalf("sign-out audit %d %v", n, err)
	}
	env.expect("GET", "/v1/account", tokOther, "", "", 200, "")
}
