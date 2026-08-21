package public

import (
	"database/sql"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	webassets "github.com/kokosx/stratum/internal/web"
)

type Handler struct {
	queries  *db.Queries
	blocks   *blocks.Registry
	template *template.Template
}

type PageData struct {
	Title          string
	SEOTitle       string
	SEODescription string
	SiteTitle      string
	Language       string

	Content template.HTML
}

func NewHandler(
	queries *db.Queries,
	blocks *blocks.Registry,
) (*Handler, error) {

	templateFS, err := fs.Sub(webassets.Assets, "templates/public")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.ParseFS(templateFS, "layout.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		queries:  queries,
		blocks:   blocks,
		template: tmpl,
	}, nil
}

func (h *Handler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/stratum/blocks.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(h.blocks.Styles()))
		return
	}

	path := r.URL.Path

	entry, err := h.queries.GetPublishedEntryByPath(
		r.Context(),
		path,
	)

	if err != nil {

		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}

		log.Printf("get entry: %v", err)

		http.Error(
			w,
			"Internal Server Error",
			http.StatusInternalServerError,
		)

		return
	}

	doc, err := document.Decode(
		[]byte(entry.DocumentJson),
	)

	if err != nil {

		log.Printf("decode document: %v", err)

		http.Error(
			w,
			"Internal Server Error",
			http.StatusInternalServerError,
		)

		return
	}

	content, err := h.blocks.RenderDocument(doc)

	if err != nil {

		log.Printf("render document: %v", err)

		http.Error(
			w,
			"Internal Server Error",
			http.StatusInternalServerError,
		)

		return
	}
	settings, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		log.Printf("get site settings: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Title:          entry.Title,
		SEOTitle:       stringValue(entry.SeoTitle),
		SEODescription: stringValue(entry.SeoDescription),
		SiteTitle:      settings.SiteTitle,
		Language:       settings.Language,

		Content: content,
	}

	if err := h.template.ExecuteTemplate(
		w,
		"layout.html",
		data,
	); err != nil {

		log.Printf("render page template: %v", err)
	}
}

func stringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}

	return value.String
}
