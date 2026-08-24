package routing

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func benchmarkRoutes(b *testing.B, nRoutes int) *Runtime {
	b.Helper()
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(b.TempDir(), "bench.db"))
	_ = database.Migrate(ctx)
	queries := db.New(database.DB)
	rt := NewRuntime(queries)
	// Seed nRoutes routes directly
	for i := 0; i < nRoutes; i++ {
		path := "/page-" + string(rune('a'+i%26)) + string(rune('0'+i%10))
		_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r" + path, Path: path, RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "e" + path, Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	}
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-blog", Path: "/blog", RouteType: RouteTypeArchive, ContentTypeID: sql.NullString{String: "post", Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-old", Path: "/old", RouteType: RouteTypeRedirect, RedirectTo: sql.NullString{String: "/new", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	_ = rt.Reload(ctx)
	return rt
}

func BenchmarkRouteLookup(b *testing.B) {
	b.ReportAllocs()
	rt := benchmarkRoutes(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := rt.Lookup("/blog"); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkRouteNotFound(b *testing.B) {
	b.ReportAllocs()
	rt := benchmarkRoutes(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := rt.Lookup("/not-found-" + string(rune(i%10))); ok {
			b.Fatal("hit")
		}
	}
}

func BenchmarkRedirectLookup(b *testing.B) {
	b.ReportAllocs()
	rt := benchmarkRoutes(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Lookup("/old")
	}
}
