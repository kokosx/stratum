package preview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stratum.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// seed minimal site and content type
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO site_settings (id, site_title, site_url, language, timezone, homepage_mode, indexing_enabled, sitemap_enabled, robots_mode, site_represents, posts_base_path, active_theme) VALUES (1, 'Test', 'https://example.com', 'en', 'UTC', 'latest_posts', 1, 1, 'managed', 'organization', '/blog', 'default')`)
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO content_types (id, display_name, plural_name, hierarchical, public, config_json, created_at, updated_at) VALUES ('page', 'Page', 'Pages', 0, 1, '{}', unixepoch(), unixepoch())`)
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO users (id, email, password_hash, role, created_at, updated_at) VALUES ('user1', 'test@example.com', 'hash', 'admin', unixepoch(), unixepoch())`)
	// create entry and revisions
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO entries (id, content_type_id, slug, status, published_revision_id, created_at, updated_at) VALUES ('entry1', 'page', 'test-page', 'active', NULL, unixepoch(), unixepoch())`)
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, created_at) VALUES ('rev1', 'entry1', 1, 'Draft Title', '{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Hello draft"}}]}', unixepoch())`)
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, created_at) VALUES ('rev2', 'entry1', 2, 'Updated Title', '{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Updated draft"}}]}', unixepoch())`)
	queries := db.New(database.DB)
	svc := NewService(database.DB, queries)
	cleanup := func() { database.Close() }
	return svc, cleanup
}

func TestCreateAndGet(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	token, link, err := svc.Create(ctx, "entry1", "rev1", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token == "" || len(token) < 20 {
		t.Fatalf("token too short: %q", token)
	}
	// Check stored as hash, not plaintext
	var storedHash string
	if err := svc.db.QueryRowContext(ctx, `SELECT token_hash FROM preview_links WHERE id=?`, link.ID).Scan(&storedHash); err != nil {
		t.Fatalf("query hash: %v", err)
	}
	if storedHash == token {
		t.Fatalf("token stored plaintext!")
	}
	h := sha256.Sum256([]byte(token))
	expected := hex.EncodeToString(h[:])
	if storedHash != expected {
		t.Fatalf("hash mismatch: got %s want %s", storedHash, expected)
	}
	// Get by token
	got, err := svc.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RevisionID != "rev1" || got.EntryID != "entry1" {
		t.Fatalf("got wrong link: %+v", got)
	}
}

func TestSpecificRevisionSemantics(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	token, _, err := svc.Create(ctx, "entry1", "rev1", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Create new revision for same entry
	_, _ = svc.db.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, created_at) VALUES ('rev3', 'entry1', 3, 'Even newer', '{"version":1,"nodes":[]}', unixepoch())`)
	// Old token should still point to rev1
	got, err := svc.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("get old token: %v", err)
	}
	if got.RevisionID != "rev1" {
		t.Fatalf("expected rev1, got %s", got.RevisionID)
	}
	// New token for rev3 should point to rev3
	token2, _, _ := svc.Create(ctx, "entry1", "rev3", "user1", 24*time.Hour)
	got2, _ := svc.GetByToken(ctx, token2)
	if got2.RevisionID != "rev3" {
		t.Fatalf("expected rev3, got %s", got2.RevisionID)
	}
}

func TestExpiry(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	token, link, _ := svc.Create(ctx, "entry1", "rev1", "user1", time.Hour)
	// Manually expire
	_, _ = svc.db.ExecContext(ctx, `UPDATE preview_links SET expires_at=? WHERE id=?`, time.Now().Add(-2*time.Hour).Unix(), link.ID)
	if _, err := svc.GetByToken(ctx, token); err == nil {
		t.Fatalf("expected expired to be 404")
	}
}

func TestRevocation(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	token, link, _ := svc.Create(ctx, "entry1", "rev1", "user1", 24*time.Hour)
	if err := svc.Revoke(ctx, link.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.GetByToken(ctx, token); err == nil {
		t.Fatalf("expected revoked to be 404")
	}
}

func TestWrongToken(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := svc.GetByToken(ctx, "invalid-token-xyz"); err == nil {
		t.Fatalf("expected invalid token to fail")
	}
}

func TestTokenEntropy(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	token, _, _ := svc.Create(ctx, "entry1", "rev1", "user1", 24*time.Hour)
	// Token should be base64url without padding, 32 bytes => 43 chars
	if len(token) < 43 {
		t.Fatalf("token too short for 256 bits: %d", len(token))
	}
}

func TestMaxActivePerEntry(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	for i := 0; i < MaxActivePerEntry; i++ {
		revID := "rev1"
		if i%2 == 0 {
			revID = "rev2"
		}
		if _, _, err := svc.Create(ctx, "entry1", revID, "user1", 24*time.Hour); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	// Next should fail
	if _, _, err := svc.Create(ctx, "entry1", "rev1", "user1", 24*time.Hour); err == nil {
		t.Fatalf("expected max active limit")
	}
}

func TestListActive(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	svc.Create(ctx, "entry1", "rev1", "user1", 24*time.Hour)
	svc.Create(ctx, "entry1", "rev2", "user1", 24*time.Hour)
	links, err := svc.ListActiveByEntry(ctx, "entry1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2, got %d", len(links))
	}
}
