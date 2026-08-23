package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/layouts"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type layoutTemplatesData struct {
	Templates []layoutTemplateRow
	CSRFToken string
	Flash     string
	Error     string
}

type layoutTemplateRow struct {
	ID              string
	Name            string
	ContentTypeID   string
	ContentTypeName string
	Status          string // Published / Draft changes / Unpublished
	IsDefault       bool
}

type layoutTemplateFormData struct {
	Heading          string
	Action           string
	PublishAction    string
	DefaultAction    string
	BackURL          string
	TemplateID       string
	Name             string
	ContentTypeID    string
	ContentTypeName  string
	ReadOnlyCT       bool
	DocumentJSON     string
	EditorJSON       template.JS
	CSRFToken        string
	Error            string
	Dirty            string
	Status           string
	PublicNote       string
	IsDefault        bool
	ContentTypes     []ctOption
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
	// Load content types for default check
	rows := make([]layoutTemplateRow, 0, len(templates))
	for _, t := range templates {
		status := h.layoutTemplateStatus(r, t)
		ctName := t.ContentTypeID
		if ct, err := h.queries.GetContentType(r.Context(), t.ContentTypeID); err == nil {
			ctName = ct.DisplayName
		}
		isDefault := false
		if ct, err := h.queries.GetContentType(r.Context(), t.ContentTypeID); err == nil && ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String == t.ID {
			isDefault = true
		}
		rows = append(rows, layoutTemplateRow{ID: t.ID, Name: t.Name, ContentTypeID: t.ContentTypeID, ContentTypeName: ctName, Status: status, IsDefault: isDefault})
	}
	data := layoutTemplatesData{Templates: rows, CSRFToken: token, Flash: h.consumeFlash(w, r)}
	if err := h.layoutTemplatesTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Templates", ActiveMenu: "appearance", CSRFToken: token, Flash: data.Flash, Content: data}); err != nil {
		log.Printf("render layout templates: %v", err)
	}
}

func (h *Handler) layoutTemplateStatus(r *http.Request, tmpl db.LayoutTemplate) string {
	if !tmpl.PublishedRevisionID.Valid {
		return "Unpublished"
	}
	latest, err := h.queries.GetLatestLayoutTemplateRevision(r.Context(), tmpl.ID)
	if err != nil {
		return "Published"
	}
	if latest.ID == tmpl.PublishedRevisionID.String {
		return "Published"
	}
	return "Draft changes"
}

func (h *Handler) newLayoutTemplate(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	cts, _ := h.queries.ListContentTypes(r.Context())
	var opts []ctOption
	for _, ct := range cts {
		opts = append(opts, ctOption{ID: ct.ID, DisplayName: ct.DisplayName})
	}
	data := layoutTemplateFormData{
		Heading:      "Create Template",
		Action:       "/admin/appearance/templates",
		BackURL:      "/admin/appearance/templates",
		CSRFToken:    token,
		ContentTypes: opts,
	}
	if err := h.layoutTemplateFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Create Template", ActiveMenu: "appearance", CSRFToken: token, Content: data}); err != nil {
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
	if name == "" {
		h.renderLayoutCreateError(w, r, "Name is required", name, ctID)
		return
	}
	if ctID == "" {
		h.renderLayoutCreateError(w, r, "Content type is required", name, ctID)
		return
	}
	if _, err := h.queries.GetContentType(r.Context(), ctID); err != nil {
		h.renderLayoutCreateError(w, r, "Invalid content type", name, ctID)
		return
	}
	// Create logical template + initial revision with single slot
	id, err := randomID()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	revID, err := randomID()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	now := time.Now().Unix()
	slotID, _ := randomID()
	// Use stable deterministic? Use random but okay; spec says stable seeded IDs for defaults, but new ones random.
	docJSON := `{"version":1,"nodes":[{"id":"` + slotID + `","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	// Validate
	if h.blocks != nil {
		if d, err := document.Decode([]byte(docJSON)); err == nil {
			if err := layouts.ValidateLayoutTemplateDocument(h.blocks, d); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	tx, err := h.database.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	if err := qtx.CreateLayoutTemplate(r.Context(), db.CreateLayoutTemplateParams{ID: id, Name: name, ContentTypeID: ctID, PublishedRevisionID: sql.NullString{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		log.Printf("create layout template: %v", err)
		h.renderLayoutCreateError(w, r, "Could not create template", name, ctID)
		return
	}
	if err := qtx.CreateLayoutTemplateRevision(r.Context(), db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: id, RevisionNumber: 1, DocumentJson: docJSON, CreatedBy: sql.NullString{}, CreatedAt: now}); err != nil {
		log.Printf("create layout revision: %v", err)
		h.renderLayoutCreateError(w, r, "Could not create template revision", name, ctID)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
		Heading:      "Create Template",
		Action:       "/admin/appearance/templates",
		BackURL:      "/admin/appearance/templates",
		Name:         name,
		ContentTypeID: ctID,
		CSRFToken:    token,
		Error:        msg,
		ContentTypes: opts,
	}
	if err := h.layoutTemplateFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Create Template", ActiveMenu: "appearance", CSRFToken: token, Content: data}); err != nil {
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
	bootstrap, err := json.Marshal(editorBootstrap{
		Document: json.RawMessage(rev.DocumentJson), Catalog: h.blocks.EditorCatalogFor("layout-template"), Definitions: h.blocks.EditorDefinitions(doc), PreviewURL: "/admin/appearance/templates/" + tmpl.ID + "/preview",
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := layoutTemplateFormData{
		Heading:       "Edit Template",
		Action:        "/admin/appearance/templates/" + tmpl.ID,
		PublishAction: "/admin/appearance/templates/" + tmpl.ID + "/publish",
		DefaultAction: "/admin/appearance/templates/" + tmpl.ID + "/default",
		BackURL:       "/admin/appearance/templates",
		TemplateID:    tmpl.ID,
		Name:          tmpl.Name,
		ContentTypeID: tmpl.ContentTypeID,
		ContentTypeName: ctName,
		ReadOnlyCT:    true,
		DocumentJSON:  rev.DocumentJson,
		EditorJSON:    template.JS(bootstrap),
		CSRFToken:     token,
		Error:         errMsg,
		Dirty:         "Saved",
		Status:        status,
		IsDefault:     isDefault,
	}
	// Use separate template for layout editor (reuses entry_form layout but with own bootstrap)
	if err := h.layoutTemplateEditorTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Edit Template - " + tmpl.Name, ActiveMenu: "appearance", CSRFToken: token, Content: data}); err != nil {
		log.Printf("render layout editor: %v", err)
	}
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
	doc, err := document.Decode([]byte(docJSON))
	if err != nil {
		h.handleLayoutSaveError(w, r, tmpl, "Invalid document: "+err.Error())
		return
	}
	if err := layouts.ValidateLayoutTemplateDocument(h.blocks, doc); err != nil {
		h.handleLayoutSaveError(w, r, tmpl, err.Error())
		return
	}
	// Persist: create new revision, update name and updated_at
	user, _ := h.currentUser(r)
	author := ""
	if user.ID != "" {
		author = user.ID
	}
	now := time.Now().Unix()
	revID, err := randomID()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	tx, err := h.database.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	latest, err := qtx.GetLatestLayoutTemplateRevision(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	nextRev := latest.RevisionNumber + 1
	// Update name if changed
	if tmpl.Name != name {
		if err := qtx.UpdateLayoutTemplate(r.Context(), db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: id}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		// still update updated_at? spec says updated_at updated on publish? We'll update anyway
		_ = qtx.UpdateLayoutTemplate(r.Context(), db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: id})
	}
	var createdBy sql.NullString
	if author != "" {
		createdBy = sql.NullString{String: author, Valid: true}
	}
	if err := qtx.CreateLayoutTemplateRevision(r.Context(), db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: id, RevisionNumber: nextRev, DocumentJson: docJSON, CreatedBy: createdBy, CreatedAt: now}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// No page cache invalidation for draft
	if isDatastarRequest(r) {
		latest2, _ := h.queries.GetLatestLayoutTemplateRevision(r.Context(), id)
		tmpl2, _ := h.queries.GetLayoutTemplate(r.Context(), id)
		status := h.layoutTemplateStatus(r, tmpl2)
		var buf bytes.Buffer
		// reuse editor status? We'll just toast
		_ = latest2
		_ = status
		_ = buf
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
		// try loading latest if not posted (e.g. publish button without doc)
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
	doc, err := document.Decode([]byte(docJSON))
	if err != nil {
		h.handleLayoutSaveError(w, r, tmpl, "Invalid document: "+err.Error())
		return
	}
	if err := layouts.ValidateLayoutTemplateDocument(h.blocks, doc); err != nil {
		h.handleLayoutSaveError(w, r, tmpl, err.Error())
		return
	}
	user, _ := h.currentUser(r)
	author := ""
	if user.ID != "" {
		author = user.ID
	}
	now := time.Now().Unix()
	revID, err := randomID()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	tx, err := h.database.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	latest, err := qtx.GetLatestLayoutTemplateRevision(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	nextRev := latest.RevisionNumber + 1
	// Update name if needed
	if tmpl.Name != name {
		if err := qtx.UpdateLayoutTemplate(r.Context(), db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: id}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	var createdBy sql.NullString
	if author != "" {
		createdBy = sql.NullString{String: author, Valid: true}
	}
	if err := qtx.CreateLayoutTemplateRevision(r.Context(), db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: id, RevisionNumber: nextRev, DocumentJson: docJSON, CreatedBy: createdBy, CreatedAt: now}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := qtx.SetLayoutTemplatePublishedRevision(r.Context(), db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateLayoutTemplates()
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
		// Try JSON body
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
	if err := layouts.ValidateLayoutTemplateDocument(h.blocks, doc); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	// Build sample entry doc
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
	input := RenderInput{
		Document:       composed,
		Title:          sampleTitle,
		Excerpt:        sampleExcerpt,
		Path:           path,
		EntryID:        "preview-layout-" + id,
	}
	// Use public preview but we already composed, so we can call documentPreview directly if it handles layout? But we composed already, so avoid double compose.
	// Instead manually prepare and theme via public handler? Simpler to call documentPreview with a doc that has no layout template ID so it won't try to compose again.
	if h.documentPreview == nil {
		http.Error(w, "Preview renderer is unavailable", http.StatusServiceUnavailable)
		return
	}
	// Temporarily set LayoutTemplateID empty to avoid double-compose
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

func (h *Handler) setDefaultLayoutTemplate(w http.ResponseWriter, r *http.Request) {
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
	// Only published templates can be default
	if !tmpl.PublishedRevisionID.Valid {
		http.Error(w, "Template must be published to be default", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	if err := h.queries.SetContentTypeDefaultLayoutTemplate(r.Context(), db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: sql.NullString{String: id, Valid: true}, UpdatedAt: now, ID: tmpl.ContentTypeID}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Default template updated."))
		return
	}
	h.setFlash(w, "Default template updated.")
	http.Redirect(w, r, "/admin/appearance/templates/"+id+"/edit", http.StatusSeeOther)
}


