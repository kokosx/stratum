package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// TestTursoNativeFTSCapability exercises the exact tursogo driver and native
// platform library used by Stratum. It intentionally does not use SQLite FTS5.
func TestTursoNativeFTSCapability(t *testing.T) {
	available, err := NativeFTSAvailable(context.Background(), filepath.Join(t.TempDir(), "fts-probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skip("native Turso FTS unavailable in this tursogo/platform build")
	}
}
