package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	b, err := os.ReadFile("db/migrations/000001_foundation.up.sql")
	if err != nil {
		log.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY)`)
	if err != nil {
		log.Fatal(err)
	}
	var version int
	err = tx.QueryRow(ctx, `SELECT version FROM schema_migrations WHERE version = 1`).Scan(&version)
	if err == nil {
		log.Println("migration already applied")
		return
	}
	if err != pgx.ErrNoRows {
		log.Fatal(err)
	}
	_, err = tx.Exec(ctx, string(b), pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		log.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (1)`)
	if err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("migration 1 applied")
}
