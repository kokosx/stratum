package media

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func benchmarkMediaService(b *testing.B) *Service {
	b.Helper()
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(b.TempDir(), "bench.db"))
	_ = database.Migrate(ctx)
	queries := db.New(database.DB)
	store, _ := NewLocalStorage(filepath.Join(b.TempDir(), "media"))
	svc := NewService(queries, store)
	// Seed 10 media with variants
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	data := buf.Bytes()
	for i := 0; i < 10; i++ {
		asset, _ := svc.Upload(ctx, "test.png", "", bytes.NewReader(data))
		if asset != nil {
			// touch MediaView to cache
			_, _ = svc.MediaView(ctx, asset.ID)
		}
	}
	return svc
}

func BenchmarkMediaMetadataResolve(b *testing.B) {
	b.ReportAllocs()
	svc := benchmarkMediaService(b)
	// Get an ID
	ctx := context.Background()
	rows, _ := svc.queries.ListAllMedia(ctx)
	if len(rows) == 0 {
		b.Skip("no media")
	}
	id := rows[0].ID
	// Ensure cached
	_, _ = svc.MediaView(ctx, id)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := svc.MediaView(ctx, id); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkMediaViews10Warm(b *testing.B) {
	b.ReportAllocs()
	svc := benchmarkMediaService(b)
	ctx := context.Background()
	rows, _ := svc.queries.ListAllMedia(ctx)
	ids := make([]string, 0, 10)
	for _, r := range rows {
		ids = append(ids, r.ID)
		if len(ids) >= 10 {
			break
		}
	}
	// Warm cache
	_ = svc.MediaViews(ctx, ids)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.MediaViews(ctx, ids)
	}
}

// BenchmarkMediaViews10 is kept for backward compat (warm).
func BenchmarkMediaViews10(b *testing.B) { BenchmarkMediaViews10Warm(b) }

func BenchmarkMediaViews10Cold(b *testing.B) {
	b.ReportAllocs()
	svc := benchmarkMediaService(b)
	ctx := context.Background()
	rows, _ := svc.queries.ListAllMedia(ctx)
	ids := make([]string, 0, 10)
	for _, r := range rows {
		ids = append(ids, r.ID)
		if len(ids) >= 10 {
			break
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.InvalidateAllViews()
		_ = svc.MediaViews(ctx, ids)
	}
}

func BenchmarkMediaServeMetadata(b *testing.B) {
	b.ReportAllocs()
	svc := benchmarkMediaService(b)
	ctx := context.Background()
	rows, _ := svc.queries.ListAllMedia(ctx)
	if len(rows) == 0 {
		b.Skip("no media")
	}
	id := rows[0].ID
	// warm serve cache
	_, _, _, _ = svc.OpenVariant(ctx, id, "original")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, _, _, err := svc.OpenVariant(ctx, id, "original")
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

// countingDB wraps *sql.DB and counts QueryContext calls for MediaViews batch verification.
type countingDB struct {
	db    *countingRawDB
	count *int
}

type countingRawDB struct {
	inner *sql.DB
	count *int
}

func (c *countingRawDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	*c.count++
	return c.inner.ExecContext(ctx, query, args...)
}
func (c *countingRawDB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	*c.count++
	return c.inner.PrepareContext(ctx, query)
}
func (c *countingRawDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	*c.count++
	return c.inner.QueryContext(ctx, query, args...)
}
func (c *countingRawDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	*c.count++
	return c.inner.QueryRowContext(ctx, query, args...)
}

func TestMediaViewsBatchQueryCount(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "bench.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Wrap DB with counting
	var qcount int
	wrapped := &countingRawDB{inner: database.DB, count: &qcount}
	queries := db.New(wrapped)
	store, _ := NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	svc := NewService(queries, store)
	// Seed 10 media
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	data := buf.Bytes()
	ids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		asset, err := svc.Upload(ctx, "test.png", "", bytes.NewReader(data))
		if err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
		ids = append(ids, asset.ID)
	}
	// Ensure cache cleared for cold test
	svc.InvalidateAllViews()
	qcount = 0
	_ = svc.MediaViews(ctx, ids)
	if qcount > 2 {
		t.Fatalf("cold MediaViews for 10 IDs used %d queries, want <=2", qcount)
	}
	// Warm should be 0
	qcount = 0
	_ = svc.MediaViews(ctx, ids)
	if qcount != 0 {
		t.Fatalf("warm MediaViews should be 0 queries, got %d", qcount)
	}
}
