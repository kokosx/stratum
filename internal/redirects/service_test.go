package redirects

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	return New(database.DB, queries)
}

func TestManual301And302(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	route, err := svc.Create(ctx, "/old", "/new", 301, now)
	if err != nil {
		t.Fatalf("create 301: %v", err)
	}
	if route.Path != "/old" || route.RedirectTo.String != "/new" || route.RedirectStatus.Int64 != 301 {
		t.Fatalf("route mismatch: %#v", route)
	}
	route2, err := svc.Create(ctx, "/old2", "/new2", 302, now)
	if err != nil {
		t.Fatalf("create 302: %v", err)
	}
	if route2.RedirectStatus.Int64 != 302 {
		t.Fatalf("expected 302")
	}
}

func TestInvalidSource(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	_, err := svc.Create(ctx, "old", "/new", 301, now)
	if err == nil {
		t.Fatalf("expected invalid source")
	}
	_, err = svc.Create(ctx, "https://example.com/x", "/new", 301, now)
	if err == nil {
		t.Fatalf("expected invalid source for external")
	}
}

func TestInvalidTarget(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	_, err := svc.Create(ctx, "/old", "javascript:alert(1)", 301, now)
	if err == nil {
		t.Fatalf("expected invalid target for javascript")
	}
	_, err = svc.Create(ctx, "/old2", "//evil.com", 301, now)
	if err == nil {
		t.Fatalf("expected invalid for protocol-relative")
	}
}

func TestReservedSource(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	_, err := svc.Create(ctx, "/admin", "/new", 301, now)
	if err == nil {
		t.Fatalf("expected reserved")
	}
	_, err = svc.Create(ctx, "/media/foo", "/new", 301, now)
	if err == nil {
		t.Fatalf("expected reserved media")
	}
}

func TestLiveRouteConflict(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	// Create a live entry route manually
	svc.queries.CreateRoute(ctx, db.CreateRouteParams{ID: "live1", Path: "/about", RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	_, err := svc.Create(ctx, "/about", "/contact", 301, now)
	if err == nil {
		t.Fatalf("expected conflict for live route")
	}
	if !contains(err.Error(), "already uses") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRedirectConflict(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	_, err := svc.Create(ctx, "/old", "/new", 301, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, "/old", "/other", 301, now)
	if err == nil {
		t.Fatalf("expected redirect already exists")
	}
}

func TestSelfRedirect(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	_, err := svc.Create(ctx, "/a", "/a", 301, now)
	if err == nil {
		t.Fatalf("expected self redirect error")
	}
}

func TestLoopTwoNodes(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	if _, err := svc.Create(ctx, "/a", "/b", 301, now); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(ctx, "/b", "/a", 301, now)
	if err == nil {
		t.Fatalf("expected loop error")
	}
}

func TestLoopMultiNode(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	svc.Create(ctx, "/a", "/b", 301, now)
	svc.Create(ctx, "/b", "/c", 301, now)
	_, err := svc.Create(ctx, "/c", "/a", 301, now)
	if err == nil {
		t.Fatalf("expected multi-node loop")
	}
}

func TestChainDetection(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	now := time.Now().Unix()
	svc.Create(ctx, "/a", "/b", 301, now)
	svc.Create(ctx, "/b", "/c", 301, now)
	chains, err := svc.DetectChains(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(chains) == 0 {
		t.Fatalf("expected chain detection")
	}
	// Ensure chain contains a->b->c
	found := false
	for _, ch := range chains {
		s := ""
		for _, p := range ch {
			s += p + " "
		}
		if contains(s, "/a") && contains(s, "/b") && contains(s, "/c") {
			found = true
		}
	}
	if !found {
		t.Fatalf("chain not found: %#v", chains)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
