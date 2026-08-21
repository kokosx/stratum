package admin

import (
	"html/template"
	"net/http"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
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

func NewHandler(queries *db.Queries, templateDir string) (*Handler, error) {
	dashboardTemplate, err := template.ParseFiles(
		templateDir+"/layout.html",
		templateDir+"/dashboard.html",
	)
	if err != nil {
		return nil, err
	}

	entriesTemplate, err := template.ParseFiles(
		templateDir+"/layout.html",
		templateDir+"/entries.html",
	)
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
	mux.HandleFunc("GET /admin/pages", h.listPages)
	mux.HandleFunc("GET /admin/posts", h.listPosts)
	mux.Handle("GET /admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.Dir("internal/web/static"))))

	return mux
}
