package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY)`); err != nil {
		log.Fatal(err)
	}
	files, err := filepath.Glob("db/migrations/*.up.sql")
	if err != nil || len(files) == 0 {
		log.Fatal("no migration files found")
	}
	for _, path := range files {
		version, err := strconv.Atoi(strings.SplitN(filepath.Base(path), "_", 2)[0])
		if err != nil {
			log.Fatal(err)
		}
		if err := apply(ctx, pool, version, path); err != nil {
			log.Fatal(err)
		}
	}
}

func apply(ctx context.Context, pool *pgxpool.Pool, version int, path string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var applied int
	err = tx.QueryRow(ctx, `SELECT version FROM schema_migrations WHERE version=$1`, version).Scan(&applied)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	sql, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("migration %d: %w", version, err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("migration %d applied", version)
	return nil
}
