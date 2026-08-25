package routing

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type Snapshot struct {
	ByPath map[string]Route
}

type Runtime struct {
	queries  *db.Queries
	reloadMu sync.Mutex
	snapshot atomic.Pointer[Snapshot]
}

func NewRuntime(queries *db.Queries) *Runtime {
	return &Runtime{queries: queries}
}

func (r *Runtime) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()
	rows, err := r.queries.ListRoutes(ctx)
	if err != nil {
		return err
	}
	// Load publication metadata for entry routes (visibility, published revision) to allow
	// password protection to bypass the shared page cache without a DB read on warm path.
	visMap := map[string]struct {
		visibility          string
		publishedRevisionID string
	}{}
	visRows, err := r.queries.ListEntryRouteVisibilities(ctx)
	if err != nil {
		return err
	}
	for _, v := range visRows {
		visMap[v.EntryID] = struct {
			visibility          string
			publishedRevisionID string
		}{visibility: v.Visibility, publishedRevisionID: v.PublishedRevisionID.String}
	}
	m := make(map[string]Route, len(rows))
	for _, row := range rows {
		rt := Route{
			ID:             row.ID,
			Path:           row.Path,
			EntryID:        row.EntryID,
			RouteType:      row.RouteType,
			ContentTypeID:  row.ContentTypeID,
			TaxonomyID:     row.TaxonomyID,
			TermID:         row.TermID,
			RedirectTo:     row.RedirectTo,
			RedirectStatus: row.RedirectStatus,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}
		if row.EntryID.Valid {
			if meta, ok := visMap[row.EntryID.String]; ok {
				rt.Visibility = meta.visibility
				if meta.publishedRevisionID != "" {
					rt.PublishedRevisionID = sql.NullString{String: meta.publishedRevisionID, Valid: true}
				}
			}
		}
		m[row.Path] = rt
	}
	r.snapshot.Store(&Snapshot{ByPath: m})
	return nil
}

func (r *Runtime) Current() *Snapshot {
	return r.snapshot.Load()
}

func (r *Runtime) Loaded() bool {
	return r.snapshot.Load() != nil
}

func (r *Runtime) Lookup(path string) (Route, bool) {
	snap := r.snapshot.Load()
	if snap == nil {
		return Route{}, false
	}
	rt, ok := snap.ByPath[path]
	return rt, ok
}

func (r *Runtime) Count() int {
	snap := r.snapshot.Load()
	if snap == nil {
		return 0
	}
	return len(snap.ByPath)
}
