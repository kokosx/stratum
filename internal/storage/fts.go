package storage

import (
	"context"
	"fmt"
)

const nativeFTSIndexSQL = `CREATE INDEX fts_probe_idx ON fts_probe USING fts (title, body)`

// ProbeNativeFTS verifies the exact tursogo driver and native platform library
// used by Stratum with the driver's required experimental index-method setting.
// Native FTS is optional because the bundled native library is build-dependent.
func ProbeNativeFTS(ctx context.Context, databasePath string) error {
	database, err := OpenWithExperimentalIndexMethod(databasePath)
	if err != nil {
		return err
	}
	defer database.Close()

	for _, statement := range []string{
		`CREATE TABLE fts_probe (id INTEGER PRIMARY KEY, title TEXT NOT NULL, body TEXT NOT NULL)`,
		nativeFTSIndexSQL,
		`INSERT INTO fts_probe(title, body) VALUES ('Stratum CMS', 'Fast Go content management system')`,
	} {
		if _, err := database.DB.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("native FTS probe %q: %w", statement, err)
		}
	}

	var title string
	if err := database.DB.QueryRowContext(ctx, `SELECT title FROM fts_probe WHERE (title, body) MATCH 'stratum'`).Scan(&title); err != nil {
		return fmt.Errorf("native FTS match probe: %w", err)
	}
	if title != "Stratum CMS" {
		return fmt.Errorf("native FTS match probe returned %q", title)
	}
	var score float64
	if err := database.DB.QueryRowContext(ctx, `SELECT fts_score(title, body, 'stratum') FROM fts_probe WHERE (title, body) MATCH 'stratum'`).Scan(&score); err != nil {
		return fmt.Errorf("native FTS score probe: %w", err)
	}
	return nil
}

// NativeFTSAvailable is retained for callers that only need a boolean. New
// callers should use ProbeNativeFTS to preserve the native driver error.
func NativeFTSAvailable(ctx context.Context, databasePath string) (bool, error) {
	err := ProbeNativeFTS(ctx, databasePath)
	return err == nil, err
}
