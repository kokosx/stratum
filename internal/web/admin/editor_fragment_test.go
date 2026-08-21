package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditorSaveFragmentEmitsSSEStatusPatchWithoutRedirect(t *testing.T) {
	handler, _ := newTestHandler(t)
	ctx := context.Background()
	entryID := "frag-entry"
	doc := `{"version":1,"nodes":[]}`
	if err := handler.writeEntry(ctx, "page", "author", entryID, entryInput{title: "T", slug: "t", documentJSON: doc}, true, false); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID, nil)
	request.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	handler.editorSaveFragment(rec, request, "page", "pages", entryID, false, entryInput{title: "T", slug: "t", documentJSON: doc}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no redirect)", rec.Code)
	}
	if rec.Result().Header.Get("Location") != "" {
		t.Fatalf("save returned a redirect Location, which would reload the document: %s", rec.Result().Header.Get("Location"))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "datastar-patch-elements") || !strings.Contains(body, "#editor-status-region") || !strings.Contains(body, "Saved") {
		t.Fatalf("status patch missing: %s", body)
	}
	if !strings.Contains(body, "toast-success") {
		t.Fatalf("success toast missing: %s", body)
	}
}

func TestEditorPublishFragmentShowsPublishedStateAndPublicURL(t *testing.T) {
	handler, _ := newTestHandler(t)
	ctx := context.Background()
	entryID := "publish-entry"
	doc := `{"version":1,"nodes":[]}`
	if err := handler.writeEntry(ctx, "page", "author", entryID, entryInput{title: "T", slug: "published", documentJSON: doc}, true, true); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID+"/publish", nil)
	request.Header.Set("Datastar-Request", "true")
	request.Host = "example.test"
	rec := httptest.NewRecorder()
	handler.editorSaveFragment(rec, request, "page", "pages", entryID, true, entryInput{title: "T", slug: "published", documentJSON: doc}, nil)

	body := rec.Body.String()
	if !strings.Contains(body, "Published") {
		t.Fatalf("published state missing: %s", body)
	}
	if !strings.Contains(body, `id="editor-public-url"`) || !strings.Contains(body, "http://example.test/published") {
		t.Fatalf("public URL link missing: %s", body)
	}
}

func TestEditorSaveFragmentShowsInlineErrorAndErrorToast(t *testing.T) {
	handler, _ := newTestHandler(t)
	ctx := context.Background()
	entryID := "error-entry"
	doc := `{"version":1,"nodes":[]}`
	if err := handler.writeEntry(ctx, "page", "author", entryID, entryInput{title: "T", slug: "t", documentJSON: doc}, true, false); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/admin/pages/"+entryID, nil)
	request.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	handler.editorSaveFragment(rec, request, "page", "pages", entryID, false, entryInput{}, errors.New("slug is already in use"))

	body := rec.Body.String()
	if !strings.Contains(body, "toast-error") || !strings.Contains(body, `id="editor-error"`) || !strings.Contains(body, "Could not save the entry.") {
		t.Fatalf("inline error + toast missing: %s", body)
	}
}
