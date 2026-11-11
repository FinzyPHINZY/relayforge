package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/tenant"
)

func main() {
	action := flag.String("action", "", "suspend, export, or purge")
	orgID := flag.String("org-id", "", "organization UUID")
	file := flag.String("file", "", "export file path (required for export and purge)")
	confirm := flag.String("confirm-org-id", "", "repeat organization UUID to confirm purge")
	flag.Parse()
	if *action != "suspend" && *action != "export" && *action != "purge" {
		log.Fatal("action must be suspend, export, or purge")
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
