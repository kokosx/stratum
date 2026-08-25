package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSQLiteFTS5 verifies the required FTS5 capability through storage.Open.
func TestSQLiteFTS5(t *testing.T) {
	if err := ProbeFTS5(context.Background(), filepath.Join(t.TempDir(), "fts-probe.db")); err != nil {
		t.Fatal(err)
	}
}
