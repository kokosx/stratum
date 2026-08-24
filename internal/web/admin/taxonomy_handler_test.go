package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

func seededTaxonomyHandler(t *testing.T) *Handler {
	t.Helper()
	h, _ := newTestHandler(t)
	if err := (&storage.Database{DB: h.database}).Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h
}

func postTermRequest(path string, form url.Values) *http.Request {
	form.Set("csrf_token", "csrf")
	r := httptest.NewRequest(http.MethodPost, path, nil)
	r.Form = form
	r.PostForm = form
	r.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	return r
}

func TestCreateTermHandlerMapsDescriptionAndParent(t *testing.T) {
	h := seededTaxonomyHandler(t)
	ctx := context.Background()
	svc := taxonomy.New(h.database, h.queries)
	parent, err := svc.CreateTerm(ctx, "category", "Parent", "parent", "", "")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.createCategory(rec, postTermRequest("/admin/posts/categories", url.Values{
		"name": {"Child"}, "slug": {"child"}, "parent_id": {parent.ID}, "description": {"Child description"},
	}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	child, err := h.queries.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "category", Slug: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if !child.ParentID.Valid || child.ParentID.String != parent.ID || child.Description != "Child description" {
		t.Fatalf("child stored incorrectly: %+v", child)
	}
}

func TestCreateTagHandlerAcceptsDescription(t *testing.T) {
	h := seededTaxonomyHandler(t)
	rec := httptest.NewRecorder()
	h.createTag(rec, postTermRequest("/admin/posts/tags", url.Values{
		"name": {"Go"}, "slug": {"go"}, "description": {"Go tag description"},
	}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	term, err := h.queries.GetTermByTaxonomyAndSlug(context.Background(), db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "tag", Slug: "go"})
	if err != nil || term.Description != "Go tag description" || term.ParentID.Valid {
		t.Fatalf("tag stored incorrectly: term=%+v err=%v", term, err)
	}
}

func TestSingleLifecycleEndpointsRejectWrongContentType(t *testing.T) {
	h := seededTaxonomyHandler(t)
	ctx := context.Background()
	if err := h.queries.CreateEntry(ctx, db.CreateEntryParams{ID: "post-entry", ContentTypeID: "post", Slug: "post-entry", Status: "active", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []func(http.ResponseWriter, *http.Request, string, string){h.trashEntry, h.restoreEntry, h.deleteEntryPermanently} {
		req := postTermRequest("/admin/pages/post-entry/action", url.Values{})
		req.SetPathValue("id", "post-entry")
		rec := httptest.NewRecorder()
		endpoint(rec, req, "page", "/admin/pages")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("wrong-type lifecycle status = %d, want 404", rec.Code)
		}
	}
}
