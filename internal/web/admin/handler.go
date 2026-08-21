package admin

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
	webassets "github.com/kokosx/stratum/internal/web"
)

type Handler struct {
	queries           *db.Queries
	dashboardTemplate *template.Template
	entriesTemplate   *template.Template
}

type LayoutData struct {
	Title      string
	ActiveMenu string
	Content    any
}

func NewHandler(queries *db.Queries) (*Handler, error) {
	templateFS, err := fs.Sub(webassets.Assets, "templates/admin")
	if err != nil {
		return nil, fmt.Errorf("admin templates: %w", err)
	}

	dashboardTemplate, err := template.ParseFS(templateFS, "layout.html", "dashboard.html")
	if err != nil {
		return nil, err
	}

	entriesTemplate, err := template.ParseFS(templateFS, "layout.html", "entries.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		queries:           queries,
		dashboardTemplate: dashboardTemplate,
		entriesTemplate:   entriesTemplate,
	}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", h.dashboard)
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /admin/pages", h.listPages)
	mux.HandleFunc("GET /admin/posts", h.listPosts)
	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(fmt.Sprintf("admin static files: %v", err))
	}
	mux.Handle("GET /admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.FS(staticFS))))

	return mux
}
