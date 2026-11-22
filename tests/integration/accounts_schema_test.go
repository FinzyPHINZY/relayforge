package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"webhook_delivery/internal/id"
)

func pgCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestAccountsSchemaConstraints(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	newID := func() string {
		v, err := id.New()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email) VALUES ($1,'Bo@x.io')`, newID()); pgCode(err) != "23514" {
		t.Fatalf("mixed-case email: want check violation, got %v", err)
	}
	user := newID()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email) VALUES ($1,'bo@x.io')`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_identities (id,user_id,provider,provider_subject) VALUES ($1,$2,'github','123')`, newID(), user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_identities (id,user_id,provider,provider_subject) VALUES ($1,$2,'github','123')`, newID(), user); pgCode(err) != "23505" {
		t.Fatalf("duplicate identity: want unique violation, got %v", err)
	}

	org := newID()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name) VALUES ($1,'acme')`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (id,organization_id,user_id,name,email,role) VALUES ($1,$2,$3,'Bo','bo@x.io','owner')`, newID(), org, user); err != nil {
		t.Fatalf("owner member: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events (id,organization_id,actor_user_id,actor_role,action,target_type,target_id) VALUES ($1,$2,$3,'admin','user.signed_in','user',$3)`, newID(), org, user); err != nil {
		t.Fatalf("user actor audit: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events (id,organization_id,actor_role,action,target_type,target_id) VALUES ($1,$2,'admin','user.signed_in','user',$3)`, newID(), org, user); pgCode(err) != "23514" {
		t.Fatalf("no actor: want check violation, got %v", err)
	}
	// Review history has exactly one actor: a key or a user.
	for name, args := range map[string][2]any{"both": {newID(), user}, "neither": {nil, nil}} {
		if _, err := pool.Exec(ctx, `INSERT INTO delivery_review_history (id,delivery_id,organization_id,api_key_id,actor_user_id,status) VALUES ($1,$2,$3,$4,$5,'investigating')`, newID(), newID(), org, args[0], args[1]); pgCode(err) != "23514" {
			t.Fatalf("review actor %s: want check violation, got %v", name, err)
		}
	}
}
