package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/database/sqlc"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

func main() {
	name := flag.String("org", "", "organization name")
	existingOrgID := flag.String("org-id", "", "issue a key for an existing organization")
	keyName := flag.String("key-name", "bootstrap", "API key name")
	scopeArg := flag.String("scopes", "events:write,events:read,endpoints:write,endpoints:read,deliveries:read,deliveries:write,metrics:read,team:read,team:write", "comma-separated scopes")
	flag.Parse()
	if (*existingOrgID == "") == (strings.TrimSpace(*name) == "") {
		log.Fatal("provide exactly one of -org or -org-id")
	}
	if *existingOrgID != "" && !id.Valid(*existingOrgID) {
		log.Fatal("invalid -org-id")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	orgID := *existingOrgID
	if orgID == "" {
		orgID, err = id.New()
		if err != nil {
			log.Fatal(err)
		}
	}
	keyID, err := id.New()
	if err != nil {
		log.Fatal(err)
	}
	raw, err := security.NewToken("whk_")
	if err != nil {
		log.Fatal(err)
	}
	prefix, err := security.KeyPrefix(raw)
	if err != nil {
		log.Fatal(err)
	}
	scopes := strings.Split(*scopeArg, ",")
	allowed := map[string]bool{"events:write": true, "events:read": true, "endpoints:write": true, "endpoints:read": true, "deliveries:read": true, "deliveries:write": true, "metrics:read": true, "team:read": true, "team:write": true}
	for _, s := range scopes {
		if !allowed[s] {
			log.Fatalf("invalid scope: %q", s)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if *existingOrgID == "" {
		err = sqlc.New(tx).CreateOrganization(ctx, sqlc.CreateOrganizationParams{ID: orgID, Name: *name})
		if err != nil {
			log.Fatal(err)
		}
	} else {
		var status string
		if err = tx.QueryRow(ctx, `SELECT status FROM organizations WHERE id=$1`, orgID).Scan(&status); err != nil || status != "active" {
			log.Fatal("organization not found or inactive")
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO api_keys (id,organization_id,name,key_prefix,key_hash,scopes) VALUES ($1,$2,$3,$4,$5,$6)`, keyID, orgID, *keyName, prefix, security.Hash(raw), scopes)
	if err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Organization ID: %s\nAPI key (shown once): %s\n", orgID, raw)
}
