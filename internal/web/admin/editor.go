package admin

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"

	"github.com/kokosx/stratum/internal/document"
)

type editorBootstrap struct {
	Document    json.RawMessage `json:"document"`
	Catalog     any             `json:"catalog"`
	Definitions any             `json:"definitions"`
	PreviewURL  string          `json:"previewUrl"`
}

func (h *Handler) renderPageForm(w http.ResponseWriter, data pageFormData) {
	token, err := h.csrfToken(w)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if h.blocks == nil {
		http.Error(w, "Block registry is not configured", http.StatusInternalServerError)
		return
	}
	if data.DocumentJSON == "" {
		data.DocumentJSON = `{"version":1,"nodes":[]}`
	}
	doc, err := document.Decode([]byte(data.DocumentJSON))
	if err != nil {
		log.Printf("prepare editor document: %v", err)
		http.Error(w, "Invalid stored document", http.StatusInternalServerError)
		return
	}
	bootstrap, err := json.Marshal(editorBootstrap{
		Document: json.RawMessage(data.DocumentJSON), Catalog: h.blocks.EditorCatalog(), Definitions: h.blocks.EditorDefinitions(doc), PreviewURL: "/admin/editor/preview",
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// encoding/json escapes '<', '>' and '&', so this cannot terminate the script element.
	data.EditorJSON = template.JS(bootstrap)
	data.CSRFToken = token
	if err := h.pageTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: data.Heading, ActiveMenu: "pages", CSRFToken: token, Content: data}); err != nil {
		log.Printf("render page form: %v", err)
	}
}

func (h *Handler) previewDocument(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	doc, err := document.Decode([]byte(postedDocument(r)))
	if err == nil {
		err = h.blocks.ValidateDocument(doc)
	}
	var rendered template.HTML
	if err == nil {
		rendered, err = h.blocks.RenderDocument(doc)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("<style>" + h.blocks.Styles() + "</style>" + string(rendered)))
}
