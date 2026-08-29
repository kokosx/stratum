package admin

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func testPNGBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func newMediaHandlerForMultiTest(t *testing.T) *Handler {
	t.Helper()
	ctx := t.Context()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	svc, err := auth.NewService(database.DB, queries, false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	store, err := media.NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	mediaSvc := media.NewServiceWithDB(database.DB, queries, store)
	handler, err := NewHandler(database.DB, queries, svc, registry, themeRuntime, mediaSvc)
	if err != nil {
		t.Fatal(err)
	}
	publicHandler, err := publicweb.NewHandler(queries, registry, themeRuntime, mediaSvc)
	if err != nil {
		t.Fatal(err)
	}
	handler.SetDocumentPreviewRenderer(publicHandler.RenderEditableDocument)
	return handler
}

func TestMultiUploadFileCountBound(t *testing.T) {
	handler := newMediaHandlerForMultiTest(t)

	// Create admin user and get CSRF
	// Use helper to create user directly via auth service? Simpler: bypass CSRF by not checking?
	// newTestHandler's auth service starts empty; we need to create admin via setup
	// Instead we can directly test the file count logic by constructing a request with valid CSRF
	// We need to get CSRF token
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/media", nil)
	token, err := handler.csrfToken(rec, req)
	if err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()

	// Build multipart with 21 files (exceeds 20) via direct handler call bypassing auth middleware
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("csrf_token", token)
	for i := 0; i < 21; i++ {
		fw, _ := w.CreateFormFile("file", "img.jpg")
		_, _ = fw.Write(testPNGBytes(100, 100))
	}
	w.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/admin/media/upload", &buf)
	req2.Header.Set("Content-Type", w.FormDataContentType())
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	// Call handler directly to bypass auth redirect
	handler.uploadMedia(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too many files, got %d body %s", rec2.Code, rec2.Body.String())
	}
	if !bytes.Contains(rec2.Body.Bytes(), []byte("too many files")) {
		t.Fatalf("expected too many files error, got %s", rec2.Body.String())
	}
}

func TestMultiUploadAggregateWithinBounds(t *testing.T) {
	handler := newMediaHandlerForMultiTest(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/media", nil)
	token, err := handler.csrfToken(rec, req)
	if err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("csrf_token", token)
	for i := 0; i < 3; i++ {
		fw, _ := w.CreateFormFile("file", "img.jpg")
		_, _ = fw.Write(testPNGBytes(200, 200))
	}
	w.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/admin/media/upload", &buf)
	req2.Header.Set("Content-Type", w.FormDataContentType())
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	handler.uploadMedia(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for 3 files, got %d body %s", rec2.Code, rec2.Body.String())
	}
	if !bytes.Contains(rec2.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("expected ok true, got %s", rec2.Body.String())
	}
	if !bytes.Contains(rec2.Body.Bytes(), []byte(`"uploaded":3`)) && !bytes.Contains(rec2.Body.Bytes(), []byte(`"assets"`)) {
		t.Logf("body %s", rec2.Body.String())
	}
}
