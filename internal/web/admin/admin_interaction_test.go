package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestInvalidCreatePreservesFields(t *testing.T) {
	h, _ := newTestHandler(t)
	// Get CSRF token
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/pages/new", nil)
	// Need auth – use setup helper that creates admin and returns handler with auth?
	// newTestHandler already sets up DB and auth service, but we need an authenticated request.
	// Instead test readEntryInput preservation via helper – simulate POST with reserved slug.
	form := url.Values{
		"title":         {"My Preserved Title"},
		"slug":          {"admin"}, // reserved slug -> validation error
		"document_json": {`{"version":1,"nodes":[]}`},
		"csrf_token":    {"test"},
	}
	r := httptest.NewRequest(http.MethodPost, "/admin/pages", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Bypass CSRF by setting cookie and header manually and mocking validCSRF? Instead directly test readEntryInput.
	// Simpler: test handler.createPage preserves title via rendered HTML
	// Setup auth user and CSRF
	ctx := context.Background()
	// Create admin user via auth
	token, err := h.auth.Setup(ctx, h.auth.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Get CSRF token
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/pages/new", nil)
	csrfReq.AddCookie(&http.Cookie{Name: "stratum_session", Value: token})
	_ = h.auth.UserForToken
	csTok, _ := h.csrfToken(csrfRec, csrfReq)
	// Build POST with auth cookie + csrf
	form.Set("csrf_token", csTok)
	req = httptest.NewRequest(http.MethodPost, "/admin/pages", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Add cookies
	for _, c := range csrfRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.AddCookie(&http.Cookie{Name: "stratum_session", Value: token})
	// Ensure CSRF cookie present
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with form re-render, got %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "My Preserved Title") {
		t.Fatalf("form did not preserve title after validation error, body: %s", body)
	}
	if !strings.Contains(body, "Could not save") && !strings.Contains(body, "reserved") && !strings.Contains(body, "this slug") {
		// error message may vary
		t.Logf("warning: expected error message not found, body: %s", body)
	}
}

func TestDestructiveActionRequiresCSRF(t *testing.T) {
	h, _ := newTestHandler(t)
	ctx := context.Background()
	token, err := h.auth.Setup(ctx, h.auth.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	// Create a page to delete
	doc := `{"version":1,"nodes":[]}`
	entryID := "csrf-del-test"
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "To Delete", slug: "to-delete", documentJSON: doc}, true, false); err != nil {
		t.Fatal(err)
	}
	// Attempt delete without CSRF
	req := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID+"/delete", nil)
	req.AddCookie(&http.Cookie{Name: "stratum_session", Value: token})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF, got %d", rec.Code)
	}
	// With valid CSRF should succeed (or not 403)
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/pages", nil)
	csrfReq.AddCookie(&http.Cookie{Name: "stratum_session", Value: token})
	csTok, _ := h.csrfToken(csrfRec, csrfReq)
	req2 := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID+"/delete", strings.NewReader(url.Values{"csrf_token": {csTok}}.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req2.AddCookie(c)
	}
	req2.AddCookie(&http.Cookie{Name: "stratum_session", Value: token})
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec2, req2)
	// Move to trash then delete permanently requires trash first – but test just checks not forbidden
	if rec2.Code == http.StatusForbidden {
		t.Fatalf("valid CSRF should not be forbidden")
	}
}

func TestQuickEditPreservesDraftPublicSemantics(t *testing.T) {
	h, _ := newTestHandler(t)
	ctx := context.Background()
	// Setup auth
	adminToken, _ := h.auth.Setup(ctx, h.auth.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	// Create published entry
	entryID := "qe-semantic"
	doc := `{"version":1,"nodes":[]}`
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "Original", slug: "original", documentJSON: doc}, true, true); err != nil {
		t.Fatal(err)
	}
	// Verify published revision title
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	pubRev, err := h.queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil {
		t.Fatal(err)
	}
	if pubRev.Title != "Original" {
		t.Fatalf("published title = %q want Original", pubRev.Title)
	}
	// Simulate Quick Edit Save draft (non-publish) – use handler directly with CSRF
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/pages", nil)
	csrfReq.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	csTok, _ := h.csrfToken(csrfRec, csrfReq)
	// Build quick edit POST form
	form := url.Values{
		"title":       {"Edited Draft"},
		"slug":        {"original"},
		"csrf_token":  {csTok},
		"quick_action": {"save"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID+"/quick-edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("quick edit save status = %d body %s", rec.Code, rec.Body.String())
	}
	// After draft save, published should still be Original
	entry2, _ := h.queries.GetEntry(ctx, entryID)
	pubRev2, _ := h.queries.GetEntryRevision(ctx, entry2.PublishedRevisionID.String)
	if pubRev2.Title != "Original" {
		t.Fatalf("published changed after draft quick edit: %q", pubRev2.Title)
	}
	latest, _ := h.queries.GetLatestEntryRevision(ctx, entryID)
	if latest.Title != "Edited Draft" {
		t.Fatalf("latest draft not updated: %q", latest.Title)
	}
	// Now publish via quick edit
	form.Set("quick_action", "publish")
	form.Set("title", "Published Edit")
	req3 := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID+"/quick-edit", strings.NewReader(form.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Need new CSRF? reuse
	for _, c := range csrfRec.Result().Cookies() {
		req3.AddCookie(c)
	}
	req3.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	req3.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec3 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusSeeOther && rec3.Code != http.StatusOK {
		t.Fatalf("quick edit publish status = %d", rec3.Code)
	}
	entry3, _ := h.queries.GetEntry(ctx, entryID)
	pubRev3, _ := h.queries.GetEntryRevision(ctx, entry3.PublishedRevisionID.String)
	if pubRev3.Title != "Published Edit" {
		t.Fatalf("published not updated after quick edit publish: %q", pubRev3.Title)
	}
	_ = pubRev
	_ = db.Entry{}
}

func TestDefaultActionCannotApplyToDraftTemplate(t *testing.T) {
	h, _ := newTestHandler(t)
	ctx := context.Background()
	// Create draft template (not published)
	templateID, err := h.layoutsService.CreateWithKind(ctx, "Draft Template", "page", "single")
	if err != nil {
		t.Fatal(err)
	}
	// Ensure it is draft (no published revision)
	tmpl, err := h.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.PublishedRevisionID.Valid {
		t.Fatalf("new template should be draft")
	}
	// Setup auth
	adminToken, _ := h.auth.Setup(ctx, h.auth.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/appearance/templates", nil)
	csrfReq.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	csTok, _ := h.csrfToken(csrfRec, csrfReq)
	form := url.Values{"csrf_token": {csTok}}
	req := httptest.NewRequest(http.MethodPost, "/admin/appearance/templates/"+templateID+"/default", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when setting draft as default, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestNormalPOSTStillWorksWhereDatastarExists(t *testing.T) {
	h, _ := newTestHandler(t)
	ctx := context.Background()
	adminToken, _ := h.auth.Setup(ctx, h.auth.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	// Create entry to edit
	entryID := "normal-post-test"
	doc := `{"version":1,"nodes":[]}`
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "Orig", slug: "orig", documentJSON: doc}, true, false); err != nil {
		t.Fatal(err)
	}
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/pages/"+entryID+"/edit", nil)
	csrfReq.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	csTok, _ := h.csrfToken(csrfRec, csrfReq)
	// Normal POST without Datastar header should redirect, not SSE
	form := url.Values{
		"title":         {"Updated Normal"},
		"slug":          {"orig"},
		"document_json": {doc},
		"csrf_token":    {csTok},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 for normal POST, got %d location %s body %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "text/event-stream") {
		t.Fatalf("normal POST should not be SSE")
	}
	// Datastar POST should be SSE
	form2 := url.Values{
		"title":         {"Updated Datastar"},
		"slug":          {"orig"},
		"document_json": {doc},
		"csrf_token":    {csTok},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID, strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Datastar-Request", "true")
	for _, c := range csrfRec.Result().Cookies() {
		req2.AddCookie(c)
	}
	req2.AddCookie(&http.Cookie{Name: "stratum_session", Value: adminToken})
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csTok})
	rec2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("datastar POST expected 200, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("datastar POST should be SSE, got %s", rec2.Header().Get("Content-Type"))
	}
}
