package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSetupRequiresCSRFToken(t *testing.T) {
	handler, service := newTestHandler(t)
	form := url.Values{
		"setup_code": {service.SetupCode()},
		"site_title": {"Example"},
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("setup without CSRF status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if hasAdmin, err := service.HasAdmin(context.Background()); err != nil || hasAdmin {
		t.Fatalf("administrator created without CSRF: hasAdmin=%v, err=%v", hasAdmin, err)
	}
}

func newTestHandler(t *testing.T) (*Handler, *auth.Service) {
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
	service, err := auth.NewService(database.DB, queries, false)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(database.DB, queries, service)
	if err != nil {
		t.Fatal(err)
	}
	return handler, service
}
