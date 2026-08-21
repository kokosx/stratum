package public

import (
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
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

	Content template.HTML
}

func NewHandler(
	queries *db.Queries,
	blocks *blocks.Registry,
	templatePath string,
) (*Handler, error) {

	tmpl, err := template.ParseFiles(templatePath)
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

	data := PageData{
		Title:          entry.Title,
		SEOTitle:       stringValue(entry.SeoTitle),
		SEODescription: stringValue(entry.SeoDescription),

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
