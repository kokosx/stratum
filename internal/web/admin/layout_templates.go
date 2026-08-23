package admin

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"

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
	Status          string
	IsDefault       bool
	ParentName      string
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
	ParentID         string
	ParentOptions    []parentOption
	ParentName       string
}

type parentOption struct {
	ID   string
	Name string
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
	// Map for parent names
	parentNames := map[string]string{}
	for _, t := range templates {
		parentNames[t.ID] = t.Name
	}
	rows := make([]layoutTemplateRow, 0, len(templates))
	for _, t := range templates {
		ctName := t.ContentTypeID
		isDefault := false
		if ct, ok := ctMap[t.ContentTypeID]; ok {
			ctName = ct.DisplayName
			if ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String == t.ID {
				isDefault = true
			}
		}
		status := layoutTemplateStatusFromMaps(t, latestMap)
		parentName := ""
		if t.ParentTemplateID.Valid {
			if n, ok := parentNames[t.ParentTemplateID.String]; ok {
				parentName = n
			} else {
				parentName = t.ParentTemplateID.String
			}
		}
		rows = append(rows, layoutTemplateRow{ID: t.ID, Name: t.Name, ContentTypeID: t.ContentTypeID, ContentTypeName: ctName, Status: status, IsDefault: isDefault, ParentName: parentName})
	}
	data := layoutTemplatesData{Templates: rows, CSRFToken: token, Flash: h.consumeFlash(w, r)}
	if err := h.layoutTemplatesTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Templates", ActiveMenu: "appearance", CSRFToken: token, Flash: data.Flash, Content: data}); err != nil {
		log.Printf("render layout templates: %v", err)
	}
}

func layoutTemplateStatusFromMaps(tmpl db.LayoutTemplate, latestMap map[string]db.LayoutTemplateRevision) string {
	if !tmpl.PublishedRevisionID.Valid {
		return "Unpublished"
	}
	latest, ok := latestMap[tmpl.ID]
	if !ok {
		return "Published"
	}
	if latest.ID == tmpl.PublishedRevisionID.String {
		return "Published"
	}
	return "Draft changes"
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
	// Parent options: all published templates
	var parentOpts []parentOption
	if pubs, err := h.queries.ListLayoutTemplates(r.Context()); err == nil {
		for _, t := range pubs {
			if t.PublishedRevisionID.Valid {
				parentOpts = append(parentOpts, parentOption{ID: t.ID, Name: t.Name + " (" + t.ContentTypeID + ")"})
			}
		}
	}
	data := layoutTemplateFormData{
		Heading:      "Create Template",
		Action:       "/admin/appearance/templates",
		BackURL:      "/admin/appearance/templates",
		CSRFToken:    token,
		ContentTypes: opts,
		ParentOptions: parentOpts,
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
	parentID := strings.TrimSpace(r.FormValue("parent_template_id"))
	if name == "" {
		h.renderLayoutCreateError(w, r, "Name is required", name, ctID, parentID)
		return
	}
	if ctID == "" {
		h.renderLayoutCreateError(w, r, "Content type is required", name, ctID, parentID)
		return
	}
	var id string
	var err error
	if parentID != "" {
		id, err = h.layoutsService.CreateWithParent(r.Context(), name, ctID, parentID)
	} else {
		id, err = h.layoutsService.Create(r.Context(), name, ctID)
	}
	if err != nil {
		log.Printf("create layout template: %v", err)
		h.renderLayoutCreateError(w, r, entryWriteError(err), name, ctID, parentID)
		return
	}
	http.Redirect(w, r, "/admin/appearance/templates/"+id+"/edit", http.StatusSeeOther)
}

func (h *Handler) renderLayoutCreateError(w http.ResponseWriter, r *http.Request, msg, name, ctID, parentID string) {
	token, _ := h.csrfToken(w, r)
	cts, _ := h.queries.ListContentTypes(r.Context())
	var opts []ctOption
	for _, ct := range cts {
		opts = append(opts, ctOption{ID: ct.ID, DisplayName: ct.DisplayName})
	}
	var parentOpts []parentOption
	if pubs, err := h.queries.ListLayoutTemplates(r.Context()); err == nil {
		for _, t := range pubs {
			if t.PublishedRevisionID.Valid {
				parentOpts = append(parentOpts, parentOption{ID: t.ID, Name: t.Name + " (" + t.ContentTypeID + ")"})
			}
		}
	}
	data := layoutTemplateFormData{
		Heading:      "Create Template",
		Action:       "/admin/appearance/templates",
		BackURL:      "/admin/appearance/templates",
		Name:         name,
		ContentTypeID: ctID,
		ParentID:     parentID,
		CSRFToken:    token,
		Error:        msg,
		ContentTypes: opts,
		ParentOptions: parentOpts,
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
	// Parent options: published templates of same content type, excluding self
	var parentOpts []parentOption
	if rows, err := h.queries.ListPublishedLayoutTemplatesByContentType(r.Context(), tmpl.ContentTypeID); err == nil {
		for _, t := range rows {
			if t.ID == tmpl.ID {
				continue
			}
			parentOpts = append(parentOpts, parentOption{ID: t.ID, Name: t.Name})
		}
	}
	parentName := ""
	if tmpl.ParentTemplateID.Valid {
		if p, err := h.queries.GetLayoutTemplate(r.Context(), tmpl.ParentTemplateID.String); err == nil {
			parentName = p.Name
		}
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
		ParentID:      "",
		ParentOptions: parentOpts,
		ParentName:    parentName,
	}
	if tmpl.ParentTemplateID.Valid {
		data.ParentID = tmpl.ParentTemplateID.String
	}
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
	parentID := strings.TrimSpace(r.FormValue("parent_template_id"))
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
	parentProvided := r.FormValue("parent_template_id") != "" || r.PostFormValue("parent_template_id") == ""
	// Detect if field was submitted (even empty). Use Form.Has
	parentProvided = hasFormValue(r, "parent_template_id")
	var saveErr error
	if parentProvided {
		saveErr = h.layoutsService.SaveDraftWithParent(r.Context(), id, name, docJSON, parentID, author, true)
	} else {
		saveErr = h.layoutsService.SaveDraft(r.Context(), id, name, docJSON, author)
	}
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

func hasFormValue(r *http.Request, key string) bool {
	_ = r.ParseForm()
	_, ok := r.Form[key]
	return ok
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
	parentID := strings.TrimSpace(r.FormValue("parent_template_id"))
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
	parentProvided := hasFormValue(r, "parent_template_id")
	var pubErr error
	if parentProvided {
		pubErr = h.layoutsService.PublishWithParent(r.Context(), id, name, docJSON, parentID, author, true)
	} else {
		pubErr = h.layoutsService.Publish(r.Context(), id, name, docJSON, author)
	}
	if pubErr != nil {
		h.handleLayoutSaveError(w, r, tmpl, pubErr.Error())
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
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Default template updated."))
		return
	}
	h.setFlash(w, "Default template updated.")
	http.Redirect(w, r, "/admin/appearance/templates/"+id+"/edit", http.StatusSeeOther)
}
