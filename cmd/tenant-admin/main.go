package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/tenant"
)

func main() {
	action := flag.String("action", "", "suspend, export, purge, or link-owner")
	orgID := flag.String("org-id", "", "organization UUID")
	email := flag.String("email", "", "owner email address (required for link-owner)")
	file := flag.String("file", "", "export file path (required for export and purge)")
	confirm := flag.String("confirm-org-id", "", "repeat organization UUID to confirm purge")
	flag.Parse()
	if *action != "suspend" && *action != "export" && *action != "purge" && *action != "link-owner" {
		log.Fatal("action must be suspend, export, purge, or link-owner")
	}
	if *action == "link-owner" && (!id.Valid(*orgID) || *email == "") {
		log.Fatal("link-owner requires a valid -org-id and -email")
	}
	if *action == "purge" && *confirm != *orgID {
		log.Fatal("purge requires -confirm-org-id matching -org-id")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	switch *action {
	case "link-owner":
		var userID string
		if userID, err = tenant.LinkOwner(ctx, pool, *orgID, *email); err == nil {
			where := os.Getenv("DASHBOARD_PUBLIC_URL")
			if where == "" {
				where = "the relayforge console"
			}
			log.Printf("user %s is now an owner of %s", userID, *orgID)
			log.Printf("Sign in at %s with this email to get a sign-in link.", where)
		}
	case "suspend":
		err = tenant.Suspend(ctx, pool, *orgID)
	case "export":
		err = tenant.Export(ctx, pool, *orgID, *file)
	case "purge":
		err = tenant.Purge(ctx, pool, *orgID, *file)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("tenant %s: %s complete", *orgID, *action)
}
