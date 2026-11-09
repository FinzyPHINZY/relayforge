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
	keyName := flag.String("key-name", "bootstrap", "API key name")
	scopeArg := flag.String("scopes", "events:write,events:read,endpoints:write,endpoints:read", "comma-separated scopes")
	flag.Parse()
	if strings.TrimSpace(*name) == "" {
		log.Fatal("-org is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	orgID, err := id.New()
	if err != nil {
		log.Fatal(err)
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
	allowed := map[string]bool{"events:write": true, "events:read": true, "endpoints:write": true, "endpoints:read": true, "deliveries:read": true, "deliveries:write": true, "metrics:read": true}
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
	err = sqlc.New(tx).CreateOrganization(ctx, sqlc.CreateOrganizationParams{ID: orgID, Name: *name})
	if err != nil {
		log.Fatal(err)
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
