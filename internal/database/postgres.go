package database

import (
	"context"
	"embed"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgres(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	if err := runMigrations(pool); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	return pool, nil
}

//go:embed schema.sql migrate_v3.sql
var schemaFS embed.FS

func runMigrations(pool *pgxpool.Pool) error {

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("gagal baca schema.sql: %w", err)
	}

	schemaV3, err := schemaFS.ReadFile("migrate_v3.sql")
	if err != nil {
		return fmt.Errorf("gagal baca migrate_v3.sql: %w", err)
	}

	_, err = pool.Exec(context.Background(), string(schema))
	if err != nil {
		return fmt.Errorf("gagal eksekusi schema.sql (tabel induk): %w", err)
	}

	_, err = pool.Exec(context.Background(), string(schemaV3))
	if err != nil {
		return fmt.Errorf("gagal eksekusi migrate_v3.sql: %w", err)
	}

	log.Println("Database migration executed successfully!")
	return nil
}
