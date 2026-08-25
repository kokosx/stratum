package public

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newPasswordHarness(t *testing.T) (*Handler, *storage.Database, *db.Queries, *publishing.Service) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	queries := db.New(database.DB)
	// Seed content types
	_ = database.Seed(ctx)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	svc := publishing.New(database.DB, queries)
	// Ensure routes and site loaded
	_ = handler.Hub().Routes.Reload(ctx)
	_ = handler.Hub().Site.Reload(ctx)
	return handler, database, queries, svc
}

func createPasswordPost(t *testing.T, queries *db.Queries, svc *publishing.Service, id, slug, password string) string {
	t.Helper()
	ctx := context.Background()
	now := int64(1000)
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "post", Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	hash, _ := publishing.HashPassword(password)
	revID := id + "-r1"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Protected " + id, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "password", PasswordHash: sql.NullString{String: hash, Valid: true}}); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if err := svc.PublishRevision(ctx, id, revID, now+1); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return revID
}

func TestPasswordGateWithoutCookie(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	revID := createPasswordPost(t, queries, svc, "pwd1", "protected-post", "secret")
	_ = revID
	_ = handler.Hub().Routes.Reload(ctx)
	// Ensure no cache
	handler.Hub().Pages.InvalidateAll()

	req := httptest.NewRequest(http.MethodGet, "/blog/protected-post", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET gate = %d want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Protected Content") || !strings.Contains(body, "password protected") {
		t.Fatalf("gate should contain protection text, got %s", body)
	}
	if strings.Contains(body, "Protected Body") {
		t.Fatalf("gate should not contain protected body")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("gate Cache-Control = %q want no-store", cc)
	}
}

func TestPasswordWrongPasswordRemainsLocked(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	createPasswordPost(t, queries, svc, "pwd2", "protected-wrong", "secret")
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()

	form := url.Values{}
	form.Set("stratum_password", "wrong")
	req := httptest.NewRequest(http.MethodPost, "/blog/protected-wrong", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("wrong password POST = %d want 200 gate", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid password") {
		t.Fatalf("should show invalid password")
	}
	// Ensure no unlock cookie set
	for _, c := range rec.Result().Cookies() {
		if strings.Contains(c.Name, "stratum_unlock") {
			t.Fatalf("wrong password should not set unlock cookie")
		}
	}
}

func TestPasswordCorrectPasswordSetsCookieAndUnlocks(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	createPasswordPost(t, queries, svc, "pwd3", "protected-ok", "secret")
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()

	form := url.Values{}
	form.Set("stratum_password", "secret")
	req := httptest.NewRequest(http.MethodPost, "/blog/protected-ok", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("correct password POST = %d want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()
	var unlockCookie *http.Cookie
	for _, c := range cookies {
		if strings.Contains(c.Name, "stratum_unlock") {
			unlockCookie = c
			break
		}
	}
	if unlockCookie == nil {
		t.Fatalf("should set unlock cookie")
	}
	if !unlockCookie.HttpOnly || unlockCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie should be HttpOnly and SameSite Lax")
	}
	if strings.Contains(unlockCookie.Value, "secret") {
		t.Fatalf("cookie must not contain password")
	}
	// Check cookie value is opaque token (base64 url, not bcrypt hash)
	if strings.HasPrefix(unlockCookie.Value, "$2a$") {
		t.Fatalf("cookie must not contain bcrypt hash")
	}
	// Unlocked GET should show content
	req2 := httptest.NewRequest(http.MethodGet, "/blog/protected-ok", nil)
	req2.AddCookie(unlockCookie)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("unlocked GET = %d want 200", rec2.Code)
	}
	if cc := rec2.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("unlocked Cache-Control = %q want private, no-store", cc)
	}
}

func TestPasswordOldCookieFailsAfterRepublish(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createPasswordPost(t, queries, svc, "pwd4", "protected-repub", "secret")
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()

	// Unlock with old revision
	form := url.Values{}
	form.Set("stratum_password", "secret")
	req := httptest.NewRequest(http.MethodPost, "/blog/protected-repub", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var oldCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if strings.Contains(c.Name, "stratum_unlock") {
			oldCookie = c
			break
		}
	}
	if oldCookie == nil {
		t.Fatalf("no cookie")
	}
	// Publish new protected revision with same password but new hash/revision
	now := int64(2000)
	hash, _ := publishing.HashPassword("secret")
	rev2ID := "pwd4-r2"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "pwd4", RevisionNumber: 2, Slug: "protected-repub", Title: "Protected new", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "password", PasswordHash: sql.NullString{String: hash, Valid: true}}); err != nil {
		t.Fatalf("rev2: %v", err)
	}
	if err := svc.PublishRevision(ctx, "pwd4", rev2ID, now); err != nil {
		t.Fatalf("publish rev2: %v", err)
	}
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()
	_ = rev1 // keep

	// Old cookie should no longer be valid (revision mismatch)
	req2 := httptest.NewRequest(http.MethodGet, "/blog/protected-repub", nil)
	req2.AddCookie(oldCookie)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "Protected Content") {
		t.Fatalf("old cookie should not unlock new revision, should show gate")
	}
}

func TestPasswordNotInSitemapOrFeedOrCollection(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	createPasswordPost(t, queries, svc, "pwd5", "protected-sitemap", "secret")
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()
	// Also create a public post for control
	pubRev := func() string {
		id := "pub1"
		now := int64(1000)
		queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "post", Slug: "public-post", Status: "active", CreatedAt: now, UpdatedAt: now})
		rev := id + "-r1"
		queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev, EntryID: id, RevisionNumber: 1, Slug: "public-post", Title: "Public", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
		svc.PublishRevision(ctx, id, rev, now+1)
		return rev
	}()
	_ = pubRev
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Sitemap.Invalidate()
	handler.Hub().Feed.Invalidate()

	// Sitemap should not contain password route
	rows, err := queries.ListSitemapEntries(ctx)
	if err != nil {
		t.Fatalf("sitemap query: %v", err)
	}
	for _, r := range rows {
		if strings.Contains(r.RoutePath, "protected-sitemap") {
			t.Fatalf("password route should not be in sitemap")
		}
	}
	// Feed should not contain password
	// Use ListPublishedEntriesByContentType which filters public only
	list, err := queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: "post", Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("list published: %v", err)
	}
	for _, e := range list {
		if e.Slug == "protected-sitemap" {
			t.Fatalf("password should not be in published list")
		}
	}
	foundPublic := false
	for _, e := range list {
		if e.Slug == "public-post" {
			foundPublic = true
		}
	}
	if !foundPublic {
		t.Fatalf("public post should be in list")
	}
	_ = handler // avoid unused
}

func TestPrivateNotRoutable(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	// Create private entry
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "priv1", ContentTypeID: "page", Slug: "private-page", Status: "active", CreatedAt: now, UpdatedAt: now})
	revID := "priv1-r1"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: "priv1", RevisionNumber: 1, Slug: "private-page", Title: "Private", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "private"})
	svc.PublishRevision(ctx, "priv1", revID, now+1)
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()

	req := httptest.NewRequest(http.MethodGet, "/private-page", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("private GET = %d want 404", rec.Code)
	}
}

func TestPublicToPasswordDoesNotLeakCache(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	// Create public entry
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "cache1", ContentTypeID: "page", Slug: "cache-test", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev1 := "cache1-r1"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev1, EntryID: "cache1", RevisionNumber: 1, Slug: "cache-test", Title: "Public Cache", DocumentJson: `{"version":1,"nodes":[{"id":"a","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Public Content"}]}}}]}`, CreatedAt: now})
	svc.PublishRevision(ctx, "cache1", rev1, now+1)
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()
	// Warm public cache
	req := httptest.NewRequest(http.MethodGet, "/cache-test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Public Content") {
		t.Fatalf("public warm failed")
	}
	// Now publish as password
	now2 := int64(2000)
	hash, _ := publishing.HashPassword("secret")
	rev2 := "cache1-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2, EntryID: "cache1", RevisionNumber: 2, Slug: "cache-test", Title: "Password", DocumentJson: `{"version":1,"nodes":[{"id":"a","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Secret Content"}]}}}]}`, CreatedAt: now2, Visibility: "password", PasswordHash: sql.NullString{String: hash, Valid: true}})
	svc.PublishRevision(ctx, "cache1", rev2, now2)
	_ = handler.Hub().Routes.Reload(ctx)
	handler.Hub().Pages.InvalidateAll()
	// GET without cookie should show gate, not old public cache
	req2 := httptest.NewRequest(http.MethodGet, "/cache-test", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if strings.Contains(rec2.Body.String(), "Public Content") {
		t.Fatalf("should not leak old public cache after password transition")
	}
	if !strings.Contains(rec2.Body.String(), "Protected Content") {
		t.Fatalf("should show gate")
	}
}

func TestUnknownRouteVisibilityNeverServesSharedCache(t *testing.T) {
	handler, database, queries, svc := newPasswordHarness(t)
	defer database.Close()
	ctx := context.Background()
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "unknown-vis", ContentTypeID: "page", Slug: "unknown-vis", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "unknown-vis-r1", EntryID: "unknown-vis", RevisionNumber: 1, Slug: "unknown-vis", Title: "Public", DocumentJson: `{"version":1,"nodes":[{"id":"a","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Cached Public Content"}]}}}]}`, CreatedAt: now})
	if err := svc.PublishRevision(ctx, "unknown-vis", "unknown-vis-r1", now+1); err != nil {
		t.Fatal(err)
	}
	if err := handler.Hub().Routes.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/unknown-vis", nil))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "Cached Public Content") {
		t.Fatalf("failed to warm public cache: %d %s", first.Code, first.Body.String())
	}

	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "unknown-vis-r2", EntryID: "unknown-vis", RevisionNumber: 2, Slug: "unknown-vis", Title: "Private", DocumentJson: `{"version":1,"nodes":[]}`, Visibility: "private", CreatedAt: now + 2})
	if err := svc.PublishRevision(ctx, "unknown-vis", "unknown-vis-r2", now+3); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/unknown-vis"); err == sql.ErrNoRows {
		if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "unknown-vis-route", Path: "/unknown-vis", EntryID: sql.NullString{String: "unknown-vis", Valid: true}, RouteType: "entry", CreatedAt: now + 3, UpdatedAt: now + 3}); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if err := handler.Hub().Routes.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	// Simulate a malformed/stale route snapshot without visibility metadata.
	snapshot := handler.Hub().Routes.Current()
	route := snapshot.ByPath["/unknown-vis"]
	route.Visibility = ""
	snapshot.ByPath["/unknown-vis"] = route

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown-vis", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown visibility served cached private content: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Cached Public Content") {
		t.Fatal("shared cache leaked when route visibility was unknown")
	}
}
