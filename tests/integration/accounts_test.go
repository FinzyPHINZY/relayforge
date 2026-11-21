package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
