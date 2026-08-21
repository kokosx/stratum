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
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("turso", path)
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
