package public

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/navigation"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

type Handler struct {
	queries    *db.Queries
	blocks     *blocks.Registry
	navigation *navigation.Loader
	themes     *themes.Runtime
}

func NewHandler(queries *db.Queries, blocks *blocks.Registry, runtimes ...*themes.Runtime) (*Handler, error) {
	var runtime *themes.Runtime
	if len(runtimes) > 0 {
		runtime = runtimes[0]
	} else {
		var err error
		runtime, err = themes.NewRuntime(context.Background(), queries)
		if err != nil {
			return nil, err
		}
	}
	return &Handler{queries: queries, blocks: blocks, navigation: navigation.NewLoader(queries), themes: runtime}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/stratum/blocks.css":
		serveAsset(w, "text/css; charset=utf-8", h.blocks.Styles())
		return
	case "/stratum/theme.css":
		serveAsset(w, "text/css; charset=utf-8", h.themes.Styles())
		return
	case "/stratum/theme.js":
		serveAsset(w, "text/javascript; charset=utf-8", h.themes.JavaScript())
		return
	}

	page, err := h.RenderPath(r.Context(), r.URL.Path, nil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("render public page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func serveAsset(w http.ResponseWriter, contentType, value string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(value))
}

// RenderPath is the single public and preview rendering pipeline. Passing
// temporary settings changes only this render and never the runtime snapshot.
func (h *Handler) RenderPath(ctx context.Context, path string, temporary map[string]any) ([]byte, error) {
	return h.renderPath(ctx, path, temporary, nil)
}

func (h *Handler) RenderPreview(ctx context.Context, path string, temporary map[string]any, customCSS string) ([]byte, error) {
	return h.renderPath(ctx, path, temporary, &customCSS)
}

func (h *Handler) renderPath(ctx context.Context, path string, temporary map[string]any, customCSS *string) ([]byte, error) {
	entry, err := h.queries.GetPublishedEntryByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	content, err := h.blocks.RenderDocument(doc)
	if err != nil {
		return nil, fmt.Errorf("render document: %w", err)
	}
	settings, err := h.queries.GetSiteSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get site settings: %w", err)
	}
	menus, err := h.navigation.LoadLocationsForPath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("load navigation: %w", err)
	}
	view := themes.PageView{
		Site:       themes.SiteView{Title: settings.SiteTitle, Tagline: settings.SiteTagline, Language: settings.Language},
		Entry:      themes.EntryView{Title: entry.Title, SEOTitle: stringValue(entry.SeoTitle), SEODescription: stringValue(entry.SeoDescription)},
		Navigation: menus,
		Content:    content,
	}
	if customCSS != nil {
		return h.themes.Preview(view, temporary, *customCSS)
	}
	return h.themes.Render(view, temporary)
}

func stringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
