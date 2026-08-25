package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "turso.tech/database/tursogo"
)

type Database struct {
	DB *sql.DB
}

func Open(path string) (*Database, error) {
	return open(path, path)
}

// OpenWithExperimentalIndexMethod opens a separate connection with Turso's
// experimental native index methods enabled. Search capability probing uses it
// so the normal CMS connection remains on the stable driver configuration.
func OpenWithExperimentalIndexMethod(path string) (*Database, error) {
	return open(path, path+"?experimental=index_method")
}

func open(path, dsn string) (*Database, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("turso", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite foreign-key enforcement is connection-local. Keep this embedded
	// database on one connection so every query uses the configured pragma.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{
		DB: db,
	}, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}

func (d *Database) Ping(ctx context.Context) error {
	return d.DB.PingContext(ctx)
}
