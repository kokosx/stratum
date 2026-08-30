package admin

import (
	"fmt"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/patterns"
	"github.com/kokosx/stratum/internal/rendering"
)

// RenderInput is the shared preview contract (canonical in rendering). Admin
// depends on rendering, not on the public HTTP package.
type RenderInput = rendering.RenderInput

type EditorResource struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Kind          string `json:"kind,omitempty"`
	Label         string `json:"label"`
	ContentTypeID string `json:"contentTypeId,omitempty"`
	Location      string `json:"location,omitempty"`
}

type EditorCapabilities struct {
	SaveDraft          bool `json:"saveDraft"`
	Publish            bool `json:"publish"`
	Preview            bool `json:"preview"`
	SEO                bool `json:"seo"`
	Slug               bool `json:"slug"`
	FeaturedMedia      bool `json:"featuredMedia"`
	CustomFields       bool `json:"customFields"`
	Taxonomies         bool `json:"taxonomies"`
	TemplateAssignment bool `json:"templateAssignment"`
	Scheduling         bool `json:"scheduling"`
	SitePartLocation   bool `json:"sitePartLocation"`
	DynamicContent     bool `json:"dynamicContent"`
}

type EditorActions struct {
	PreviewURL       string `json:"previewUrl,omitempty"`
	SaveURL          string `json:"saveUrl,omitempty"`
	PublishURL       string `json:"publishUrl,omitempty"`
	BackURL          string `json:"backUrl,omitempty"`
	PublicPreviewURL string `json:"publicPreviewUrl,omitempty"`
}

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
	Forms            any                                     `json:"forms,omitempty"`
	TemplateKind     string                                  `json:"templateKind,omitempty"`
	SitePartID       string                                  `json:"sitePartId,omitempty"`
	Resource         EditorResource                          `json:"resource"`
	Capabilities     EditorCapabilities                      `json:"capabilities"`
	Actions          EditorActions                           `json:"actions"`
}

func editorCapabilitiesForEntry(def content.ContentTypeDefinition) EditorCapabilities {
	return EditorCapabilities{
		SaveDraft:          true,
		Publish:            true,
		Preview:            true,
		SEO:                def.Capabilities.HasSEO && def.Routing.Single,
		Slug:               def.Routing.Single,
		FeaturedMedia:      def.Capabilities.HasFeatured,
		CustomFields:       len(def.Fields) > 0,
		Taxonomies:         true,
		TemplateAssignment: def.Routing.Single,
		Scheduling:         true,
		SitePartLocation:   false,
		DynamicContent:     def.Capabilities.HasContent,
	}
}

func editorCapabilitiesForLayoutTemplate(kind string) EditorCapabilities {
	return EditorCapabilities{
		SaveDraft:          true,
		Publish:            true,
		Preview:            true,
		SEO:                false,
		Slug:               false,
		FeaturedMedia:      false,
		CustomFields:       false,
		Taxonomies:         false,
		TemplateAssignment: false,
		Scheduling:         false,
		SitePartLocation:   false,
		DynamicContent:     true,
	}
}

func editorCapabilitiesForSitePart() EditorCapabilities {
	return EditorCapabilities{
		SaveDraft:        true,
		Publish:          true,
		Preview:          true,
		SEO:              false,
		Slug:             false,
		FeaturedMedia:    false,
		CustomFields:     false,
		Taxonomies:       false,
		Scheduling:       false,
		SitePartLocation: true,
		DynamicContent:   true,
	}
}

func buildEditorBootstrap(ctx context.Context, h *Handler, resource EditorResource, doc *document.Document, contextKind string, previewURL string, actions EditorActions) (editorBootstrap, error) {
	catalog := h.blocks.EditorCatalogFor(contextKind)
	defs := h.blocks.EditorDefinitions(doc)
	contentTypes, fieldCatalogs := h.editorOptions(ctx)
	taxonomyCatalogs := h.taxonomyCatalogs(ctx)
	patterns := h.patternsForContext(contextKind)
	var caps EditorCapabilities
	switch resource.Type {
	case "entry":
		def := content.DefinitionFor(resource.ContentTypeID)
		if d, err := content.NewCatalog(h.queries).GetDefinition(ctx, resource.ContentTypeID); err == nil {
			def = d
		}
		caps = editorCapabilitiesForEntry(def)
	case "layout-template":
		caps = editorCapabilitiesForLayoutTemplate(resource.Kind)
	case "site-part":
		caps = editorCapabilitiesForSitePart()
	default:
		caps = EditorCapabilities{SaveDraft: true, Publish: true, Preview: true}
	}
	if actions.PreviewURL == "" {
		actions.PreviewURL = previewURL
	}
	// SiteParts catalog for template/site-part contexts
	var sitePartsCatalog any
	if contextKind == "layout-template" || contextKind == "single-template" || contextKind == "archive-template" || contextKind == "site-part" {
		if parts, err := h.queries.ListSiteParts(ctx); err == nil {
			catalogList := []map[string]string{}
			for _, p := range parts {
				// For site-part editor, exclude self
				if resource.Type == "site-part" && p.ID == resource.ID {
					continue
				}
				catalogList = append(catalogList, map[string]string{"id": p.ID, "name": p.Name})
			}
			sitePartsCatalog = catalogList
		}
	}
	migratedJSON, _ := json.Marshal(doc)
	return editorBootstrap{
		Document:         json.RawMessage(migratedJSON),
		Catalog:          catalog,
		Definitions:      defs,
		PreviewURL:       previewURL,
		ContentTypeID:    resource.ContentTypeID,
		ContentTypes:     contentTypes,
		FieldCatalogs:    fieldCatalogs,
		TaxonomyCatalogs: taxonomyCatalogs,
		Patterns:         patterns,
		ContextKind:      contextKind,
		SiteParts:        sitePartsCatalog,
		Forms:            h.formOptions(ctx),
		TemplateKind:     resource.Kind,
		SitePartID:       resource.ID,
		Resource:         resource,
		Capabilities:     caps,
		Actions:          actions,
	}, nil
}

func (h *Handler) formOptions(ctx context.Context) []editorOption {
	if h.forms == nil {
		return nil
	}
	items, err := h.forms.List(ctx)
	if err != nil {
		return nil
	}
	options := make([]editorOption, 0, len(items))
	for _, item := range items {
		label := item.Name
		if !item.Active {
			label += " (Disabled)"
		}
		options = append(options, editorOption{Value: item.ID, Label: label})
	}
	return options
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
	if err == nil {
		if verr := validateDocumentNodeIDsSafe(doc); verr != nil {
			http.Error(w, "Invalid node ID", http.StatusUnprocessableEntity)
			return
		}
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
	renderInput := rendering.RenderInput{
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
	}
	if r.FormValue("editor_canvas") == "1" || r.URL.Query().Get("editor_canvas") == "1" || r.Header.Get("X-Stratum-Editor-Canvas") == "1" {
		ids := collectDocumentNodeIDs(doc)
		renderInput.EditorCanvas = &rendering.EditorCanvas{
			Enabled:             true,
			EditableNodeIDs:     ids,
			InstanceScope:       "root",
			PrimaryResourceType: "entry",
			PrimaryResourceID:   payload.EntryID,
		}
	}
	page, err := h.documentPreview(r.Context(), renderInput)
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

func collectDocumentNodeIDs(doc *document.Document) map[string]struct{} {
	if doc == nil {
		return nil
	}
	ids := make(map[string]struct{})
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			ids[n.ID] = struct{}{}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(doc.Nodes)
	return ids
}

func isSafeNodeID(id string) bool {
	// Permissive check: only reject IDs that could break HTML comments or marker parsing.
	// We allow legacy IDs with dots, but reject dangerous sequences.
	if id == "" {
		return false
	}
	if strings.Contains(id, "--") || strings.Contains(id, "<") || strings.Contains(id, ">") || strings.Contains(id, ":") || strings.Contains(id, "/") {
		return false
	}
	for _, c := range id {
		if c == '"' || c == '\'' {
			return false
		}
	}
	if len(id) > 128 {
		return false
	}
	return true
}

func validateDocumentNodeIDsSafe(doc *document.Document) error {
	if doc == nil {
		return nil
	}
	var walk func([]document.Node) error
	walk = func(nodes []document.Node) error {
		for _, n := range nodes {
			if !isSafeNodeID(n.ID) {
				return fmt.Errorf("invalid node id %q", n.ID)
			}
			if len(n.Children) > 0 {
				if err := walk(n.Children); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(doc.Nodes); err != nil {
		return err
	}
	return nil
}


func trimSlashes(s string) string {
	for len(s) > 0 && (s[0] == '/' || s[len(s)-1] == '/') {
		s = s[1 : len(s)-1]
	}
	return s
}
