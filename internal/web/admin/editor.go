package admin

import (
	"encoding/json"
	"net/http"

	"github.com/kokosx/stratum/internal/document"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

// RenderInput mirrors publicweb.RenderInput and is what the editor preview
// renderer callback receives.
type RenderInput = publicweb.RenderInput

type editorBootstrap struct {
	Document    json.RawMessage `json:"document"`
	Catalog     any             `json:"catalog"`
	Definitions any             `json:"definitions"`
	PreviewURL  string          `json:"previewUrl"`
}

func (h *Handler) previewDocument(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	var payload struct {
		Document         json.RawMessage `json:"document"`
		Title            string          `json:"title"`
		Excerpt          string          `json:"excerpt"`
		Slug             string          `json:"slug"`
		EntryID          string          `json:"entry_id"`
		LayoutTemplateID string          `json:"layout_template_id"`
		ContentTypeID    string          `json:"content_type_id"`
		SEO              struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"seo"`
	}
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		payload.Document = json.RawMessage(r.FormValue("document_json"))
		payload.Title = r.FormValue("title")
		payload.Excerpt = r.FormValue("excerpt")
		payload.Slug = r.FormValue("slug")
		payload.EntryID = r.FormValue("entry_id")
		payload.LayoutTemplateID = r.FormValue("layout_template_id")
		payload.ContentTypeID = r.FormValue("content_type_id")
		payload.SEO.Title = r.FormValue("seo_title")
		payload.SEO.Description = r.FormValue("seo_description")
	}
	doc, err := document.Decode(payload.Document)
	if err == nil {
		err = h.blocks.ValidateDocument(doc)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if h.documentPreview == nil {
		http.Error(w, "Preview renderer is unavailable", http.StatusServiceUnavailable)
		return
	}
	path := "/"
	if payload.Slug != "" {
		path = "/" + trimSlashes(payload.Slug)
	}
	// If entry ID present but content type not provided, infer it.
	ct := payload.ContentTypeID
	if ct == "" && payload.EntryID != "" {
		if e, err := h.queries.GetEntry(r.Context(), payload.EntryID); err == nil {
			ct = e.ContentTypeID
		}
	}
	page, err := h.documentPreview(r.Context(), publicweb.RenderInput{
		Document:         doc,
		Title:            payload.Title,
		Excerpt:          payload.Excerpt,
		SEOTitle:         payload.SEO.Title,
		SEODescription:   payload.SEO.Description,
		Path:             path,
		EntryID:          payload.EntryID,
		LayoutTemplateID: payload.LayoutTemplateID,
		ContentTypeID:    ct,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	// Editor previews are never indexed (defense in depth: the admin handler
	// already sends this header for every /admin response).
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func trimSlashes(s string) string {
	for len(s) > 0 && (s[0] == '/' || s[len(s)-1] == '/') {
		s = s[1 : len(s)-1]
	}
	return s
}
