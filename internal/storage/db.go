package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
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

	// modernc applies _pragma values to every connection it opens. This matters
	// with a reader pool because foreign_keys is otherwise connection-local.
	dsn := sqliteDSN(path, false)
	return open(dsn)
}

// OpenReadOnly opens an existing database without changing its journal or
// header. It is used for integrity checks of immutable backup snapshots.
func OpenReadOnly(path string) (*Database, error) {
	return open(sqliteDSN(path, true))
}

func sqliteDSN(path string, readOnly bool) string {
	query := url.Values{
		"_pragma": {
			"foreign_keys(ON)",
			"busy_timeout(5000)",
		},
	}
	if readOnly {
		query.Set("mode", "ro")
	} else {
		query["_pragma"] = append(query["_pragma"], "journal_mode(WAL)", "synchronous(NORMAL)")
	}
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: query.Encode(),
	}).String()
}

func open(dsn string) (*Database, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite WAL supports concurrent readers while serializing writers. Keep
	// the pool deliberately small rather than constraining all access to one
	// connection as the previous embedded driver required.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

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
