package routing

import (
	"context"
	"sync"
	"sync/atomic"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Snapshot is an immutable, in-memory view of all routes. It is published
// atomically and read without locks on the hot path.
type Snapshot struct {
	ByPath map[string]Route
}

// Runtime holds the immutable route snapshot for the public frontend.
type Runtime struct {
	queries  *db.Queries
	reloadMu sync.Mutex
	snapshot atomic.Pointer[Snapshot]
}

// NewRuntime creates a runtime with no snapshot. Call Reload to publish the first snapshot.
// A nil snapshot means "not loaded yet"; a non-nil empty snapshot means "loaded, zero routes" and is authoritative.
func NewRuntime(queries *db.Queries) *Runtime {
	return &Runtime{queries: queries}
}

// Reload rebuilds the snapshot from the database and atomically publishes it.
// On error the previous snapshot remains active.
func (r *Runtime) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	rows, err := r.queries.ListRoutes(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]Route, len(rows))
	for _, row := range rows {
		rt := Route{
			ID:             row.ID,
			Path:           row.Path,
			EntryID:        row.EntryID,
			RouteType:      row.RouteType,
			ContentTypeID:  row.ContentTypeID,
			RedirectTo:     row.RedirectTo,
			RedirectStatus: row.RedirectStatus,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}
		m[row.Path] = rt
	}
	r.snapshot.Store(&Snapshot{ByPath: m})
	return nil
}

// Current returns the active snapshot, or nil if not yet loaded.
func (r *Runtime) Current() *Snapshot {
	return r.snapshot.Load()
}

// Loaded reports whether a snapshot has been published.
func (r *Runtime) Loaded() bool {
	return r.snapshot.Load() != nil
}

// Lookup returns the route for path without touching the database.
// It is lock-free on the read path (atomic snapshot load + map lookup).
func (r *Runtime) Lookup(path string) (Route, bool) {
	snap := r.snapshot.Load()
	if snap == nil {
		return Route{}, false
	}
	rt, ok := snap.ByPath[path]
	return rt, ok
}

// Count returns the number of routes in the snapshot (for observability).
func (r *Runtime) Count() int {
	snap := r.snapshot.Load()
	if snap == nil {
		return 0
	}
	return len(snap.ByPath)
}
