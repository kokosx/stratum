package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/patterns"
	"github.com/kokosx/stratum/internal/rendering"
)

// RenderInput is the shared preview contract (canonical in rendering). Admin
// depends on rendering, not on the public HTTP package.
type RenderInput = rendering.RenderInput

type editorBootstrap struct {
	Document         json.RawMessage                         `json:"document"`
	Catalog          any                                     `json:"catalog"`
	Definitions      any                                     `json:"definitions"`
	PreviewURL       string                                  `json:"previewUrl"`
	ContentTypeID    string                                  `json:"contentTypeId,omitempty"`
	ContentTypes     []editorOption                          `json:"contentTypes,omitempty"`
	FieldCatalogs    map[string][]content.FieldCatalogOption `json:"fieldCatalogs,omitempty"`
	TaxonomyCatalogs map[string][]taxonomyCatalogEntry       `json:"taxonomyCatalogs,omitempty"`
	Patterns         []patterns.Pattern                      `json:"patterns,omitempty"`
	ContextKind      string                                  `json:"contextKind,omitempty"`
	SiteParts        any                                     `json:"siteParts,omitempty"`
	TemplateKind     string                                  `json:"templateKind,omitempty"`
	SitePartID       string                                  `json:"sitePartId,omitempty"`
}

type taxonomyCatalogEntry struct {
	ID    string              `json:"id"`
	Label string              `json:"label"`
	Terms []taxonomyTermEntry `json:"terms"`
}

type taxonomyTermEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Slug  string `json:"slug"`
}

type editorOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func (h *Handler) editorOptions(ctx context.Context) ([]editorOption, map[string][]content.FieldCatalogOption) {
	definitions, err := content.NewCatalog(h.queries).ListDefinitions(ctx)
	if err != nil {
		return nil, nil
	}
	types := make([]editorOption, 0, len(definitions))
	catalogs := make(map[string][]content.FieldCatalogOption, len(definitions))
	for _, definition := range definitions {
		types = append(types, editorOption{Value: string(definition.ID), Label: definition.Label()})
		catalogs[string(definition.ID)] = content.FieldCatalog(definition)
	}
	return types, catalogs
}

func (h *Handler) taxonomyCatalogs(ctx context.Context) map[string][]taxonomyCatalogEntry {
	definitions, err := content.NewCatalog(h.queries).ListDefinitions(ctx)
	if err != nil {
		return nil
	}
	out := make(map[string][]taxonomyCatalogEntry, len(definitions))
	for _, def := range definitions {
		taxRows, err := h.queries.ListTaxonomiesByContentType(ctx, string(def.ID))
		if err != nil || len(taxRows) == 0 {
			continue
		}
		entries := make([]taxonomyCatalogEntry, 0, len(taxRows))
		for _, tax := range taxRows {
			terms, _ := h.queries.ListTermsByTaxonomy(ctx, tax.ID)
			termEntries := make([]taxonomyTermEntry, 0, len(terms))
			for _, t := range terms {
				termEntries = append(termEntries, taxonomyTermEntry{ID: t.ID, Label: t.Name, Slug: t.Slug})
			}
			entries = append(entries, taxonomyCatalogEntry{ID: tax.ID, Label: tax.PluralName, Terms: termEntries})
		}
		if len(entries) > 0 {
			out[string(def.ID)] = entries
		}
	}
	return out
}

func (h *Handler) patternsForContext(ctx string) []patterns.Pattern {
	cat := patterns.NewCatalog()
	return cat.List(ctx)
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
		Fields           map[string]any  `json:"fields"`
		FeaturedMediaID  string          `json:"featured_media_id"`
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
		payload.FeaturedMediaID = r.FormValue("featured_media_id")
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
	// Preview must validate against the effective DB-backed definition: custom
	// fields are revision data, and the fallback DefinitionFor intentionally has
	// no schema for arbitrary custom types.
	definition, definitionErr := content.NewCatalog(h.queries).GetDefinition(r.Context(), ct)
	if definitionErr != nil {
		// Older preview callers did not send a type. Keep their harmless
		// no-schema behavior while refusing an unknown explicit custom type.
		if ct == "" || ct == string(content.ContentTypePage) || ct == string(content.ContentTypePost) {
			definition = content.DefinitionFor(ct)
		} else {
			http.Error(w, "Unknown content type", http.StatusUnprocessableEntity)
			return
		}
	}
	if r.Header.Get("Content-Type") != "application/json" {
		payload.Fields = rawFieldValues(r, definition)
	}
	fields, err := content.ValidateFields(definition, payload.Fields, content.FieldValidationOptions{
		MediaExists: func(id string) bool {
			_, err := h.queries.GetMedia(r.Context(), id)
			return err == nil
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	page, err := h.documentPreview(r.Context(), rendering.RenderInput{
		Document:         doc,
		Title:            payload.Title,
		Slug:             payload.Slug,
		Excerpt:          payload.Excerpt,
		SEOTitle:         payload.SEO.Title,
		SEODescription:   payload.SEO.Description,
		Path:             path,
		EntryID:          payload.EntryID,
		LayoutTemplateID: payload.LayoutTemplateID,
		ContentTypeID:    ct,
		Fields:           fields,
		FeaturedMediaID:  payload.FeaturedMediaID,
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
