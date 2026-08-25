package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNativeFTSProbeReturnsCapabilityResult ensures optional capability
// detection always retains a concrete native-driver reason.
func TestNativeFTSProbeReturnsCapabilityResult(t *testing.T) {
	err := ProbeNativeFTS(context.Background(), filepath.Join(t.TempDir(), "fts-probe.db"))
	if err != nil && err.Error() == "" {
		t.Fatal("FTS probe returned an empty error")
	}
}
