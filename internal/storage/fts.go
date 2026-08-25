package storage

import (
	"context"
	"fmt"
)

// ProbeFTS5 verifies the required standard SQLite FTS5 capability through the
// same storage opener used by the application.
func ProbeFTS5(ctx context.Context, databasePath string) error {
	database, err := Open(databasePath)
	if err != nil {
		return err
	}
	defer database.Close()

	for _, statement := range []string{
		`CREATE VIRTUAL TABLE fts_probe USING fts5(title, body, tokenize='unicode61')`,
		`INSERT INTO fts_probe(title, body) VALUES ('Stratum CMS', 'Żółty żółw buduje szybki CMS w Go')`,
	} {
		if _, err := database.DB.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("FTS5 probe %q: %w", statement, err)
		}
	}

	var title string
	if err := database.DB.QueryRowContext(ctx, `SELECT title FROM fts_probe WHERE fts_probe MATCH 'stratum'`).Scan(&title); err != nil {
		return fmt.Errorf("FTS5 match probe: %w", err)
	}
	if title != "Stratum CMS" {
		return fmt.Errorf("FTS5 match probe returned %q", title)
	}
	var score float64
	if err := database.DB.QueryRowContext(ctx, `SELECT bm25(fts_probe) FROM fts_probe WHERE fts_probe MATCH 'żółw'`).Scan(&score); err != nil {
		return fmt.Errorf("FTS5 Polish/bm25 probe: %w", err)
	}
	return nil
}
