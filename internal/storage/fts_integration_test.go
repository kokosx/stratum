//go:build tursofts

package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNativeFTSProbeWithIndexMethod is the explicit required integration path.
// It must fail if this tursogo/native-library build cannot provide native FTS.
func TestNativeFTSProbeWithIndexMethod(t *testing.T) {
	if err := ProbeNativeFTS(context.Background(), filepath.Join(t.TempDir(), "fts-probe.db")); err != nil {
		t.Fatal(err)
	}
}
