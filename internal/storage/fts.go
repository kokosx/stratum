package storage

import (
	"context"
	"fmt"
)

const nativeFTSIndexSQL = `CREATE INDEX fts_probe_idx ON fts_probe USING fts (title, body)`

// NativeFTSAvailable probes the exact tursogo driver and native platform library
// used by Stratum. Native FTS is optional because it is build-dependent.
func NativeFTSAvailable(ctx context.Context, databasePath string) (bool, error) {
	database, err := Open(databasePath)
	if err != nil {
		return false, err
	}
	defer database.Close()

	for _, statement := range []string{
		`CREATE TABLE fts_probe (id INTEGER PRIMARY KEY, title TEXT NOT NULL, body TEXT NOT NULL)`,
		nativeFTSIndexSQL,
		`INSERT INTO fts_probe(title, body) VALUES ('Stratum CMS', 'Fast Go content management system')`,
	} {
		if _, err := database.DB.ExecContext(ctx, statement); err != nil {
			if statement == nativeFTSIndexSQL {
				return false, nil
			}
			return false, fmt.Errorf("native FTS probe %q: %w", statement, err)
		}
	}

	var title string
	if err := database.DB.QueryRowContext(ctx, `SELECT title FROM fts_probe WHERE fts_match(title, body, 'stratum')`).Scan(&title); err != nil {
		return false, fmt.Errorf("native FTS match probe: %w", err)
	}
	var score float64
	if err := database.DB.QueryRowContext(ctx, `SELECT fts_score(fts_probe_idx) FROM fts_probe WHERE fts_match(title, body, 'stratum')`).Scan(&score); err != nil {
		return false, fmt.Errorf("native FTS score probe: %w", err)
	}
	return title == "Stratum CMS", nil
}
