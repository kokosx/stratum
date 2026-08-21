package public

import (
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"

	db "github.com/kokosx/stratum/internal/storage/sqlc"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/rendering"
)

type Handler struct {
	queries  *db.Queries
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
	templatePath string,
) (*Handler, error) {

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return nil, err
	}

	return &Handler{
		queries:  queries,
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

	blockDefinitions, err := h.queries.ListBlockDefinitions(r.Context())
	if err != nil {
		log.Printf("load block definitions: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	definitions := make([]rendering.Definition, 0, len(blockDefinitions))
	for _, definition := range blockDefinitions {
		if !definition.Template.Valid {
			log.Printf("block %s/%s@%d has no template", definition.Namespace, definition.Name, definition.Version)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		definitions = append(definitions, rendering.Definition{
			Namespace:    definition.Namespace,
			Name:         definition.Name,
			Version:      definition.Version,
			RendererType: definition.RendererType,
			Template:     definition.Template.String,
		})
	}

	renderer, err := rendering.NewRenderer(definitions)
	if err != nil {
		log.Printf("build block renderer: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	content, err := renderer.RenderDocument(doc)

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
