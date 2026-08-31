package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/rendering"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

type layoutTemplatesData struct {
	Templates []layoutTemplateRow
	CSRFToken string
	Flash     string
	Error     string
}

type layoutTemplateRow struct {
	ID               string
	Name             string
	ContentTypeID    string
	ContentTypeName  string
	Kind             string
	Status           string
	IsPublished      bool
	IsDefault        bool
	IsDefaultArchive bool
}

type layoutTemplateFormData struct {
	Heading              string
	Action               string
	PublishAction        string
	DefaultAction        string
	ArchiveDefaultAction string
	ClearDefaultAction   string
	ClearArchiveAction   string
	DeleteAction         string
	RevisionsURL         string
	BackURL              string
	TemplateID           string
	Name                 string
	ContentTypeID        string
	ContentTypeName      string
	Kind                 string
	ReadOnlyCT           bool
	DocumentJSON         string
	EditorJSON           template.JS
	CSRFToken            string
	Error                string
	Dirty                string
	Status               string
	PublicNote           string
	IsDefault            bool
	IsDefaultArchive     bool
	ContentTypes         []ctOption
	Warning              string
	CanSetDefault        bool
	CanSetArchiveDefault bool
}

type ctOption struct {
	ID          string
	DisplayName string
}

func (h *Handler) listLayoutTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.queries.ListLayoutTemplates(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	cts, err := h.queries.ListContentTypes(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	ctMap := make(map[string]db.ContentType, len(cts))
	for _, ct := range cts {
		ctMap[ct.ID] = ct
	}
	latestRevs, err := h.queries.ListLatestLayoutRevisions(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	latestMap := make(map[string]db.LayoutTemplateRevision, len(latestRevs))
	for _, rev := range latestRevs {
		latestMap[rev.TemplateID] = rev
	}
	rows := make([]layoutTemplateRow, 0, len(templates))
	for _, t := range templates {
		ctName := t.ContentTypeID
		isDefault := false
		isDefaultArchive := false
		if ct, ok := ctMap[t.ContentTypeID]; ok {
			ctName = ct.DisplayName
			if ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String == t.ID {
				isDefault = true
			}
			if ct.DefaultArchiveTemplateID.Valid && ct.DefaultArchiveTemplateID.String == t.ID {
				isDefaultArchive = true
			}
		}
		status := layoutTemplateStatusFromMaps(t, latestMap)
		isPublished := t.PublishedRevisionID.Valid
		rows = append(rows, layoutTemplateRow{ID: t.ID, Name: t.Name, ContentTypeID: t.ContentTypeID, ContentTypeName: ctName, Kind: t.Kind, Status: status, IsPublished: isPublished, IsDefault: isDefault, IsDefaultArchive: isDefaultArchive})
	}
	data := layoutTemplatesData{Templates: rows, CSRFToken: token, Flash: h.consumeFlash(w, r)}
	if err := h.layoutTemplatesTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Templates", ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Flash: data.Flash, Content: data}); err != nil {
		log.Printf("render layout templates: %v", err)
	}
}

func layoutTemplateStatusFromMaps(tmpl db.LayoutTemplate, latestMap map[string]db.LayoutTemplateRevision) string {
	if !tmpl.PublishedRevisionID.Valid {
		return "Draft"
	}
	latest, ok := latestMap[tmpl.ID]
	if !ok {
		return "Published"
	}
	if latest.ID == tmpl.PublishedRevisionID.String {
		return "Published"
	}
	return "Published · Unpublished changes"
}

func (h *Handler) layoutTemplateStatus(r *http.Request, tmpl db.LayoutTemplate) string {
	if !tmpl.PublishedRevisionID.Valid {
		return "Draft"
	}
	latest, err := h.queries.GetLatestLayoutTemplateRevision(r.Context(), tmpl.ID)
	if err != nil {
		return "Published"
	}
	if latest.ID == tmpl.PublishedRevisionID.String {
		return "Published"
	}
	return "Published · Unpublished changes"
}

func (h *Handler) newLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	cts, _ := h.queries.ListContentTypes(r.Context())
	var opts []ctOption
	for _, ct := range cts {
		opts = append(opts, ctOption{ID: ct.ID, DisplayName: ct.DisplayName})
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind != "archive" {
		kind = "single"
	}
	data := layoutTemplateFormData{
		Heading:      "Create template",
		Action:       "/admin/appearance/templates",
		BackURL:      "/admin/appearance/templates",
		CSRFToken:    token,
		ContentTypes: opts,
		Kind:         kind,
	}
	if err := h.layoutTemplateFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Create template", ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render new template form: %v", err)
	}
}

func (h *Handler) createLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	ctID := strings.TrimSpace(r.FormValue("content_type_id"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = "single"
	}
	if kind != "single" && kind != "archive" {
		kind = "single"
	}
	if name == "" {
		h.renderLayoutCreateError(w, r, "Name is required", name, ctID)
		return
	}
	if ctID == "" {
		h.renderLayoutCreateError(w, r, "Content type is required", name, ctID)
		return
	}
	id, err := h.layoutsService.CreateWithKind(r.Context(), name, ctID, kind)
	if err != nil {
		log.Printf("create layout template: %v", err)
		h.renderLayoutCreateError(w, r, entryWriteError(err), name, ctID)
		return
	}
	http.Redirect(w, r, "/admin/appearance/templates/"+id+"/edit", http.StatusSeeOther)
}

func (h *Handler) renderLayoutCreateError(w http.ResponseWriter, r *http.Request, msg, name, ctID string) {
	token, _ := h.csrfToken(w, r)
	cts, _ := h.queries.ListContentTypes(r.Context())
	var opts []ctOption
	for _, ct := range cts {
		opts = append(opts, ctOption{ID: ct.ID, DisplayName: ct.DisplayName})
	}
	data := layoutTemplateFormData{
		Heading:       "Create template",
		Action:        "/admin/appearance/templates",
		BackURL:       "/admin/appearance/templates",
		Name:          name,
		ContentTypeID: ctID,
		CSRFToken:     token,
		Error:         msg,
		ContentTypes:  opts,
	}
	if err := h.layoutTemplateFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Create template", ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render create error: %v", err)
	}
}

func (h *Handler) editLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	latest, err := h.queries.GetLatestLayoutTemplateRevision(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	status := h.layoutTemplateStatus(r, tmpl)
	ctName := tmpl.ContentTypeID
	if ct, err := h.queries.GetContentType(r.Context(), tmpl.ContentTypeID); err == nil {
		ctName = ct.DisplayName
	}
	isDefault := false
	if ct, err := h.queries.GetContentType(r.Context(), tmpl.ContentTypeID); err == nil && ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String == tmpl.ID {
		isDefault = true
	}
	h.renderLayoutTemplateEditor(w, r, tmpl, latest, token, status, ctName, isDefault, "")
}

func (h *Handler) renderLayoutTemplateEditor(w http.ResponseWriter, r *http.Request, tmpl db.LayoutTemplate, rev db.LayoutTemplateRevision, token, status, ctName string, isDefault bool, errMsg string) {
	doc, err := document.Decode([]byte(rev.DocumentJson))
	if err != nil {
		http.Error(w, "Invalid stored document", http.StatusInternalServerError)
		return
	}
	catalogMode := "layout-template"
	if tmpl.Kind == "archive" {
		catalogMode = "archive-template"
	} else if tmpl.Kind == "single" {
		catalogMode = "single-template"
	}
	previewURL := "/admin/appearance/templates/" + tmpl.ID + "/preview"
	resource := EditorResource{Type: "layout-template", ID: tmpl.ID, Kind: tmpl.Kind, Label: tmpl.Name, ContentTypeID: tmpl.ContentTypeID}
	actions := EditorActions{PreviewURL: previewURL, SaveURL: "/admin/appearance/templates/" + tmpl.ID, PublishURL: "/admin/appearance/templates/" + tmpl.ID + "/publish", BackURL: "/admin/appearance/templates"}
	bs, berr := buildEditorBootstrap(r.Context(), h, resource, doc, catalogMode, previewURL, actions)
	if berr != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	bootstrap, err := json.Marshal(bs)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	isDefaultArchive := false
	canSetDefault, canSetArchiveDefault := true, false
	warning := ""
	if ct, err := h.queries.GetContentType(r.Context(), tmpl.ContentTypeID); err == nil {
		isDefaultArchive = ct.DefaultArchiveTemplateID.Valid && ct.DefaultArchiveTemplateID.String == tmpl.ID
	}
	if definition, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), tmpl.ContentTypeID); err == nil {
		canSetDefault = definition.Routing.Single
		canSetArchiveDefault = definition.Routing.Archive
		if tmpl.Kind == "single" && definition.Capabilities.HasContent && !documentHasBlock(doc.Nodes, "core/content-slot") {
			warning = "This template does not include Entry Content. Freeform content from entries will not be displayed."
		}
	}
	data := layoutTemplateFormData{
		Heading:              "Edit template",
		Action:               "/admin/appearance/templates/" + tmpl.ID,
		PublishAction:        "/admin/appearance/templates/" + tmpl.ID + "/publish",
		DefaultAction:        "/admin/appearance/templates/" + tmpl.ID + "/default",
		ArchiveDefaultAction: "/admin/appearance/templates/" + tmpl.ID + "/default-archive",
		ClearDefaultAction:   "/admin/appearance/templates/" + tmpl.ID + "/clear-default",
		ClearArchiveAction:   "/admin/appearance/templates/" + tmpl.ID + "/clear-default-archive",
		DeleteAction:         "/admin/appearance/templates/" + tmpl.ID + "/delete",
		RevisionsURL:         "/admin/appearance/templates/" + tmpl.ID + "/revisions",
		BackURL:              "/admin/appearance/templates",
		TemplateID:           tmpl.ID,
		Name:                 tmpl.Name,
		ContentTypeID:        tmpl.ContentTypeID,
		ContentTypeName:      ctName,
		Kind:                 tmpl.Kind,
		ReadOnlyCT:           true,
		DocumentJSON:         rev.DocumentJson,
		EditorJSON:           template.JS(bootstrap),
		CSRFToken:            token,
		Error:                errMsg,
		Dirty:                "Saved",
		Status:               status,
		IsDefault:            isDefault,
		IsDefaultArchive:     isDefaultArchive,
		Warning:              warning,
		CanSetDefault:        canSetDefault,
		CanSetArchiveDefault: canSetArchiveDefault,
	}
	if err := h.layoutTemplateEditorTemplate.ExecuteTemplate(w, "editor_layout.html", LayoutData{Title: "Edit template - " + tmpl.Name, ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render layout editor: %v", err)
	}
}

func documentHasBlock(nodes []document.Node, block string) bool {
	for _, node := range nodes {
		if node.Block == block || documentHasBlock(node.Children, block) {
			return true
		}
	}
	return false
}

func (h *Handler) saveLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, patchElementsEvent("outer", "", editorErrorFragment(errors.New("invalid security token"))), toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	docJSON := postedDocument(r)
	if name == "" {
		h.handleLayoutSaveError(w, r, tmpl, "Name is required")
		return
	}
	if docJSON == "" {
		h.handleLayoutSaveError(w, r, tmpl, "Document is required")
		return
	}
	user, _ := h.currentUser(r)
	author := ""
	if user.ID != "" {
		author = user.ID
	}
	saveErr := h.layoutsService.SaveDraft(r.Context(), id, name, docJSON, author)
	if saveErr != nil {
		h.handleLayoutSaveError(w, r, tmpl, saveErr.Error())
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Template draft saved."))
		return
	}
	h.setFlash(w, "Template draft saved.")
	http.Redirect(w, r, "/admin/appearance/templates", http.StatusSeeOther)
}

func (h *Handler) handleLayoutSaveError(w http.ResponseWriter, r *http.Request, tmpl db.LayoutTemplate, msg string) {
	if isDatastarRequest(r) {
		writeSSE(w, patchElementsEvent("outer", "", `<p id="editor-error" class="form-error" role="alert">`+template.HTMLEscapeString(msg)+`</p>`), toastEvent("error", msg))
		return
	}
	latest, _ := h.queries.GetLatestLayoutTemplateRevision(r.Context(), tmpl.ID)
	token, _ := h.csrfToken(w, r)
	status := h.layoutTemplateStatus(r, tmpl)
	ctName := tmpl.ContentTypeID
	if ct, err := h.queries.GetContentType(r.Context(), tmpl.ContentTypeID); err == nil {
		ctName = ct.DisplayName
	}
	isDefault := false
	if ct, err := h.queries.GetContentType(r.Context(), tmpl.ContentTypeID); err == nil && ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String == tmpl.ID {
		isDefault = true
	}
	h.renderLayoutTemplateEditor(w, r, tmpl, latest, token, status, ctName, isDefault, msg)
}

func (h *Handler) publishLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	docJSON := postedDocument(r)
	if docJSON == "" {
		if latest, lerr := h.queries.GetLatestLayoutTemplateRevision(r.Context(), id); lerr == nil {
			docJSON = latest.DocumentJson
		}
		if name == "" {
			name = tmpl.Name
		}
	}
	if name == "" {
		name = tmpl.Name
	}
	if docJSON == "" {
		h.handleLayoutSaveError(w, r, tmpl, "Document is required")
		return
	}
	user, _ := h.currentUser(r)
	author := ""
	if user.ID != "" {
		author = user.ID
	}
	pubErr := h.layoutsService.Publish(r.Context(), id, name, docJSON, author)
	if pubErr != nil {
		h.handleLayoutSaveError(w, r, tmpl, pubErr.Error())
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateTemplate(id)
		// If this template is the default for its content type, all entries using the default must also be invalidated.
		if ct, err := h.queries.GetContentType(r.Context(), tmpl.ContentTypeID); err == nil {
			if (ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String == id) || (ct.DefaultArchiveTemplateID.Valid && ct.DefaultArchiveTemplateID.String == id) {
				h.runtime.InvalidateContent()
			}
		}
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Template published."))
		return
	}
	h.setFlash(w, "Template published.")
	http.Redirect(w, r, "/admin/appearance/templates", http.StatusSeeOther)
}

func (h *Handler) previewLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	docJSON := r.FormValue("document_json")
	if docJSON == "" {
		var payload struct {
			Document json.RawMessage `json:"document"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) == nil && len(payload.Document) > 0 {
			docJSON = string(payload.Document)
		}
	}
	if docJSON == "" {
		if latest, err := h.queries.GetLatestLayoutTemplateRevision(r.Context(), id); err == nil {
			docJSON = latest.DocumentJson
		}
	}
	doc, err := document.Decode([]byte(docJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := validateDocumentNodeIDsSafe(doc); err != nil {
		http.Error(w, "Invalid node ID", http.StatusUnprocessableEntity)
		return
	}
	if err := layouts.ValidateTemplateDocument(h.blocks, doc, tmpl.Kind, nil); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if tmpl.Kind == "archive" {
		h.previewArchiveLayoutTemplate(w, r, tmpl, doc)
		return
	}
	sampleEntryDoc := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "sample-text", Block: "core/text", Version: 1, Props: json.RawMessage(`{"text":"This is where the entry content will appear."}`), Settings: json.RawMessage(`{}`)},
		},
	}
	composed, err := layouts.Compose(doc, sampleEntryDoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	sampleTitle := "Example Post Title"
	sampleExcerpt := "This is an example excerpt used while previewing a layout template."
	path := "/example-post"
	if tmpl.ContentTypeID == "page" {
		path = "/example-page"
		sampleTitle = "Example Page Title"
	}
	fieldValuesForPreview := map[string]any{}
	if rows, queryErr := content.NewRepository(h.queries).QueryPublished(r.Context(), content.EntryQuery{ContentType: content.ContentTypeID(tmpl.ContentTypeID), Limit: 1}); queryErr == nil && len(rows) > 0 {
		if published, publishedErr := h.queries.GetPublishedEntryByID(r.Context(), rows[0].ID); publishedErr == nil {
			if actualDoc, decodeErr := document.Decode([]byte(published.DocumentJson)); decodeErr == nil {
				sampleEntryDoc = actualDoc
			}
			sampleTitle = published.Title
			sampleExcerpt = stringValue(published.Excerpt)
			path = rows[0].RoutePath
			fieldValuesForPreview = fieldValues(published.FieldsJson)
		}
	}
	composed, err = layouts.Compose(doc, sampleEntryDoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	input := RenderInput{
		Document:      composed,
		Title:         sampleTitle,
		Excerpt:       sampleExcerpt,
		Path:          path,
		EntryID:       "preview-layout-" + id,
		ContentTypeID: tmpl.ContentTypeID,
		Fields:        fieldValuesForPreview,
	}
	if r.FormValue("editor_canvas") == "1" || r.URL.Query().Get("editor_canvas") == "1" {
		ids := collectDocumentNodeIDs(doc)
		input.EditorCanvas = &rendering.EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root", PrimaryResourceType: "layout-template", PrimaryResourceID: tmpl.ID}
	}
	if h.documentPreview == nil {
		http.Error(w, "Preview renderer is unavailable", http.StatusServiceUnavailable)
		return
	}
	input.LayoutTemplateID = ""
	page, err := h.documentPreview(r.Context(), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func (h *Handler) previewArchiveLayoutTemplate(w http.ResponseWriter, r *http.Request, tmpl db.LayoutTemplate, doc *document.Document) {
	definition, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), tmpl.ContentTypeID)
	if err != nil {
		http.Error(w, "Content type is unavailable", http.StatusUnprocessableEntity)
		return
	}
	// Archive preview context: allow filtering by taxonomy term (e.g. Category: News) using same renderer.
	previewTaxID := strings.TrimSpace(r.FormValue("preview_taxonomy_id"))
	previewTermID := strings.TrimSpace(r.FormValue("preview_term_id"))
	var previewTerm *db.Term
	var previewTax *db.Taxonomy
	if previewTaxID != "" && previewTermID != "" {
		if tx, err := h.queries.GetTaxonomy(r.Context(), previewTaxID); err == nil {
			previewTax = &tx
		}
		if t, err := h.queries.GetTerm(r.Context(), previewTermID); err == nil && previewTax != nil && t.TaxonomyID == previewTaxID {
			previewTerm = &t
		}
		if previewTerm == nil {
			previewTaxID, previewTermID = "", ""
			previewTax, previewTerm = nil, nil
		}
	}
	perPage := 10
	var total int64
	var previewEntries []rendering.ArchiveEntry
	var archiveTitle, archiveDesc string
	var path string
	if previewTerm != nil {
		// Filter entries by term, preserve revision semantics (published revisions only).
		termRows, err := h.queries.ListPublishedEntriesByTerm(r.Context(), db.ListPublishedEntriesByTermParams{TermID: previewTermID, ContentTypeID: tmpl.ContentTypeID, Limit: int64(perPage), Offset: 0})
		if err != nil {
			http.Error(w, "Archive entries are unavailable", http.StatusUnprocessableEntity)
			return
		}
		if cnt, err := h.queries.ListPublishedEntriesByTermCount(r.Context(), db.ListPublishedEntriesByTermCountParams{TermID: previewTermID, ContentTypeID: tmpl.ContentTypeID}); err == nil {
			total = cnt
		} else {
			total = int64(len(termRows))
		}
		entries := make([]rendering.ArchiveEntry, 0, len(termRows))
		for _, row := range termRows {
			fields, _ := content.DecodeFieldSnapshot(row.FieldsJson)
			excerpt := ""
			if row.Excerpt.Valid {
				excerpt = row.Excerpt.String
			}
			entries = append(entries, rendering.ArchiveEntry{ID: row.ID, Slug: row.Slug, ContentTypeID: tmpl.ContentTypeID, Title: row.Title, Excerpt: excerpt, URL: row.RoutePath, Fields: fields})
		}
		previewEntries = entries
		archiveTitle = previewTerm.Name
		archiveDesc = previewTerm.Description
		// Derive archive path from term route if available.
		if route, err := h.queries.GetRouteByTaxonomyTerm(r.Context(), db.GetRouteByTaxonomyTermParams{TaxonomyID: sql.NullString{String: previewTaxID, Valid: true}, TermID: sql.NullString{String: previewTermID, Valid: true}}); err == nil {
			path = route.Path
		} else if previewTax != nil {
			path = taxonomy.TaxonomyTermPath(taxonomy.Taxonomy{ID: previewTax.ID, RouteBase: previewTax.RouteBase}, previewTerm.Slug)
		} else {
			path = definition.Routing.BasePath
			if path == "" {
				path = "/"
			}
		}
	} else {
		qrows, err := content.NewRepository(h.queries).QueryPublished(r.Context(), content.EntryQuery{ContentType: content.ContentTypeID(tmpl.ContentTypeID), Limit: perPage})
		if err != nil {
			http.Error(w, "Archive entries are unavailable", http.StatusUnprocessableEntity)
			return
		}
		entries := make([]rendering.ArchiveEntry, 0, len(qrows))
		for _, row := range qrows {
			fields, _ := content.DecodeFieldSnapshot(row.FieldsJSON)
			entries = append(entries, rendering.ArchiveEntry{ID: row.ID, Slug: row.Slug, ContentTypeID: tmpl.ContentTypeID, Title: row.Title, Excerpt: row.Excerpt, URL: row.RoutePath, Fields: fields})
		}
		previewEntries = entries
		if cnt, err := h.queries.CountPublishedEntriesByContentType(r.Context(), tmpl.ContentTypeID); err == nil {
			total = cnt
		} else {
			total = int64(len(entries))
		}
		path = definition.Routing.BasePath
		if path == "" {
			path = "/"
		}
		archiveTitle = definition.EffectiveLabel()
		archiveDesc = ""
	}
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}
	archive := &rendering.ArchiveContext{
		Entries:     previewEntries,
		Pagination:  rendering.PaginationContext{Current: 1, TotalPages: totalPages, TotalItems: total},
		Permalink:   path,
		Title:       archiveTitle,
		Description: archiveDesc,
	}
	if previewTaxID != "" && previewTermID != "" {
		archive.TaxonomyID = previewTaxID
		archive.TermID = previewTermID
	}
	if h.documentPreview == nil {
		http.Error(w, "Preview renderer is unavailable", http.StatusServiceUnavailable)
		return
	}
	renderInput := RenderInput{Document: doc, Title: archive.Title, Path: path, ContentTypeID: tmpl.ContentTypeID, Archive: archive}
	if r.FormValue("editor_canvas") == "1" || r.URL.Query().Get("editor_canvas") == "1" {
		ids := collectDocumentNodeIDs(doc)
		renderInput.EditorCanvas = &rendering.EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root", PrimaryResourceType: "layout-template", PrimaryResourceID: tmpl.ID}
	}
	page, err := h.documentPreview(r.Context(), renderInput)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func (h *Handler) setDefaultLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if err := h.layoutsService.SetDefault(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Default template updated."))
		return
	}
	h.setFlash(w, "Default template updated.")
	http.Redirect(w, r, "/admin/appearance/templates/"+id+"/edit", http.StatusSeeOther)
}

func (h *Handler) setDefaultArchiveTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if err := h.layoutsService.SetDefaultArchive(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Default archive template updated."))
		return
	}
	h.setFlash(w, "Default archive template updated.")
	http.Redirect(w, r, "/admin/appearance/templates/"+id+"/edit", http.StatusSeeOther)
}

func (h *Handler) clearDefaultLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	h.clearDefaultTemplate(w, r, false)
}

func (h *Handler) clearDefaultArchiveTemplate(w http.ResponseWriter, r *http.Request) {
	h.clearDefaultTemplate(w, r, true)
}

func (h *Handler) clearDefaultTemplate(w http.ResponseWriter, r *http.Request, archive bool) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if archive {
		err = h.layoutsService.ClearDefaultArchive(r.Context(), tmpl.ContentTypeID)
	} else {
		err = h.layoutsService.ClearDefault(r.Context(), tmpl.ContentTypeID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateContent()
	}
	h.setFlash(w, "Default template cleared.")
	http.Redirect(w, r, "/admin/appearance/templates/"+tmpl.ID+"/edit", http.StatusSeeOther)
}

func (h *Handler) deleteLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	usage, err := h.layoutsService.Usage(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if usage.InUse() {
		message := "This template cannot be deleted because it is currently in use."
		if usage.DefaultSingleFor != "" {
			message += " Default Single Template for " + usage.DefaultSingleFor + "."
		}
		if usage.DefaultArchiveFor != "" {
			message += " Default Archive Template for " + usage.DefaultArchiveFor + "."
		}
		if usage.ExplicitEntries > 0 {
			message += " Explicitly selected by " + fmt.Sprint(usage.ExplicitEntries) + " entries."
		}
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	if err := h.layoutsService.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateAll()
	}
	h.setFlash(w, "Template deleted.")
	http.Redirect(w, r, "/admin/appearance/templates", http.StatusSeeOther)
}

func (h *Handler) listLayoutTemplateRevisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	revisions, err := h.queries.ListLayoutTemplateRevisions(r.Context(), id)
	if err != nil {
		http.Error(w, "Revision history is unavailable", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	latestID := ""
	if len(revisions) > 0 {
		latestID = revisions[0].ID
	}
	rows := make([]revisionHistoryRow, 0, len(revisions))
	for _, revision := range revisions {
		status := ""
		if tmpl.PublishedRevisionID.Valid && tmpl.PublishedRevisionID.String == revision.ID {
			status = "Published"
		} else if revision.ID == latestID {
			status = "Current draft"
		}
		author := ""
		if revision.CreatedBy.Valid {
			author = revision.CreatedBy.String
		}
		rows = append(rows, revisionHistoryRow{
			ID:         revision.ID,
			Number:     revision.RevisionNumber,
			CreatedAt:  formatRevisionTime(revision.CreatedAt),
			Author:     author,
			Status:     status,
			PreviewURL: "/admin/appearance/templates/" + template.URLQueryEscaper(id) + "/revisions/" + template.URLQueryEscaper(revision.ID) + "/preview",
			RestoreURL: "/admin/appearance/templates/" + template.URLQueryEscaper(id) + "/revisions/" + template.URLQueryEscaper(revision.ID) + "/restore",
			CanRestore: true,
		})
	}
	h.renderRevisions(w, r, revisionHistoryData{
		Heading:    "Revision history",
		BackURL:    "/admin/appearance/templates/" + template.URLQueryEscaper(id) + "/edit",
		EntityName: tmpl.Name,
		EntityKind: "Template",
		Revisions:  rows,
		CSRFToken:  token,
		Flash:      h.consumeFlash(w, r),
	})
}

func (h *Handler) previewLayoutTemplateRevision(w http.ResponseWriter, r *http.Request) {
	id, revisionID := r.PathValue("id"), r.PathValue("revisionID")
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	revision, err := h.queries.GetLayoutTemplateRevision(r.Context(), revisionID)
	if err != nil || revision.TemplateID != id {
		http.NotFound(w, r)
		return
	}
	doc, err := document.Decode([]byte(revision.DocumentJson))
	if err != nil || layouts.ValidateTemplateDocument(h.blocks, doc, tmpl.Kind, nil) != nil {
		http.Error(w, "Invalid template revision", http.StatusUnprocessableEntity)
		return
	}
	if err := validateDocumentNodeIDsSafe(doc); err != nil {
		http.Error(w, "Invalid node ID", http.StatusUnprocessableEntity)
		return
	}
	if tmpl.Kind == "archive" {
		h.previewArchiveLayoutTemplate(w, r, tmpl, doc)
		return
	}
	sample := &document.Document{Version: 1, Nodes: []document.Node{{ID: "preview-content", Block: "core/text", Version: 1, Props: json.RawMessage(`{"text":"This is where the entry content will appear."}`), Settings: json.RawMessage(`{}`)}}}
	title, excerpt, path := "Example Entry", "", "/example"
	fields := map[string]any{}
	if rows, queryErr := content.NewRepository(h.queries).QueryPublished(r.Context(), content.EntryQuery{ContentType: content.ContentTypeID(tmpl.ContentTypeID), Limit: 1}); queryErr == nil && len(rows) > 0 {
		if published, publishedErr := h.queries.GetPublishedEntryByID(r.Context(), rows[0].ID); publishedErr == nil {
			sample, _ = document.Decode([]byte(published.DocumentJson))
			title, excerpt, path = published.Title, stringValue(published.Excerpt), rows[0].RoutePath
			fields = fieldValues(published.FieldsJson)
		}
	}
	composed, err := layouts.Compose(doc, sample)
	if err != nil || h.documentPreview == nil {
		http.Error(w, "Preview is unavailable", http.StatusUnprocessableEntity)
		return
	}
	page, err := h.documentPreview(r.Context(), RenderInput{Document: composed, Title: title, Excerpt: excerpt, Path: path, ContentTypeID: tmpl.ContentTypeID, Fields: fields})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (h *Handler) restoreLayoutTemplateRevision(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, _ := h.currentUser(r)
	if _, err := h.layoutsService.RestoreRevision(r.Context(), r.PathValue("id"), r.PathValue("revisionID"), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.setFlash(w, "Revision restored as a new draft.")
	http.Redirect(w, r, "/admin/appearance/templates/"+r.PathValue("id")+"/edit", http.StatusSeeOther)
}
