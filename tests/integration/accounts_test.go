package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"webhook_delivery/internal/mail"

	"webhook_delivery/internal/accounts"
	"webhook_delivery/internal/id"
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
	sess, u, err := st.ConsumeMagicLink(ctx, tok, "1.1.1.1", "ua")
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
	if _, _, err := st.ConsumeMagicLink(ctx, tok, "1.1.1.1", "ua"); !errors.Is(err, accounts.ErrLinkInvalid) {
		t.Fatalf("reuse: %v", err)
	}
	if _, _, err := st.ConsumeMagicLink(ctx, "rfm_short", "", ""); !errors.Is(err, accounts.ErrLinkInvalid) {
		t.Fatalf("malformed: %v", err)
	}
	// expiry
	_ = st.RequestMagicLink(ctx, cfg, "new@example.com", "1.1.1.1")
	exp := tokenOf(1)
	clock = clock.Add(16 * time.Minute)
	if _, _, err := st.ConsumeMagicLink(ctx, exp, "", ""); !errors.Is(err, accounts.ErrLinkInvalid) {
		t.Fatalf("expired: %v", err)
	}
	// existing user signs in again: same user
	clock = clock.Add(-16 * time.Minute)
	_ = st.RequestMagicLink(ctx, cfg, "new@example.com", "1.1.1.1")
	_, u2, err := st.ConsumeMagicLink(ctx, tokenOf(2), "", "")
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
			_, _, err := st.ConsumeMagicLink(ctx, race, "", "")
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
