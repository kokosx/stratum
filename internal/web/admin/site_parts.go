package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/siteparts"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type sitePartsData struct {
	Header    *db.SitePart
	Footer    *db.SitePart
	Parts     []sitePartRow
	CSRFToken string
	Flash     string
	Error     string
}

type sitePartRow struct {
	ID     string
	Name   string
	Status string
}

type sitePartFormData struct {
	Heading   string
	Action    string
	BackURL   string
	Name      string
	Location  string
	CSRFToken string
	Error     string
}

type sitePartEditorData struct {
	Heading        string
	Action         string
	PublishAction  string
	LocationAction string
	BackURL        string
	SitePartID     string
	Name           string
	DocumentJSON   string
	EditorJSON     template.JS
	CSRFToken      string
	Error          string
	Dirty          string
	Status         string
	IsPublished    bool
	Location       string
	DeleteAction   string
	RevisionsURL   string
}

func (h *Handler) listSiteParts(w http.ResponseWriter, r *http.Request) {
	parts, err := h.queries.ListSiteParts(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	var headerPart, footerPart *db.SitePart
	if row, err := h.queries.GetSitePartLocation(r.Context(), "header"); err == nil && row.SitePartID.Valid {
		if p, err := h.queries.GetSitePart(r.Context(), row.SitePartID.String); err == nil {
			headerPart = &p
		}
	}
	if row, err := h.queries.GetSitePartLocation(r.Context(), "footer"); err == nil && row.SitePartID.Valid {
		if p, err := h.queries.GetSitePart(r.Context(), row.SitePartID.String); err == nil {
			footerPart = &p
		}
	}
	latestRevs, _ := h.queries.ListLatestSitePartRevisions(r.Context())
	latestMap := make(map[string]db.SitePartRevision, len(latestRevs))
	for _, rev := range latestRevs {
		latestMap[rev.SitePartID] = rev
	}
	rows := make([]sitePartRow, 0, len(parts))
	for _, p := range parts {
		status := sitePartStatus(p, latestMap)
		rows = append(rows, sitePartRow{ID: p.ID, Name: p.Name, Status: status})
	}
	data := sitePartsData{Header: headerPart, Footer: footerPart, Parts: rows, CSRFToken: token, Flash: h.consumeFlash(w, r)}
	if err := h.sitePartsTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Site Parts", ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Flash: data.Flash, Content: data}); err != nil {
		log.Printf("render site parts: %v", err)
	}
}

func sitePartStatus(p db.SitePart, latestMap map[string]db.SitePartRevision) string {
	if !p.PublishedRevisionID.Valid {
		return "Draft"
	}
	latest, ok := latestMap[p.ID]
	if !ok {
		return "Published"
	}
	if latest.ID == p.PublishedRevisionID.String {
		return "Published"
	}
	return "Published · Unpublished changes"
}

func (h *Handler) newSitePart(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	loc := strings.TrimSpace(r.URL.Query().Get("location"))
	if loc != "header" && loc != "footer" {
		loc = ""
	}
	heading := "Create Site Part"
	if loc == "header" {
		heading = "Create Header"
	} else if loc == "footer" {
		heading = "Create Footer"
	}
	data := sitePartFormData{
		Heading:   heading,
		Action:    "/admin/appearance/site-parts/new",
		BackURL:   "/admin/appearance/site-parts",
		CSRFToken: token,
		Location:  loc,
	}
	if err := h.sitePartFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: heading, ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render new site part form: %v", err)
	}
}

func (h *Handler) createSitePart(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	loc := strings.TrimSpace(r.FormValue("location"))
	if name == "" {
		h.renderSitePartCreateError(w, r, "Name is required", name, loc)
		return
	}
	id, err := h.sitePartsService.CreateForLocation(r.Context(), name, loc)
	if err != nil {
		log.Printf("create site part: %v", err)
		h.renderSitePartCreateError(w, r, entryWriteError(err), name, loc)
		return
	}
	redirect := "/admin/appearance/site-parts/" + id + "/edit"
	if loc == "header" || loc == "footer" {
		redirect += "?intendedLocation=" + loc
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handler) renderSitePartCreateError(w http.ResponseWriter, r *http.Request, msg, name, loc string) {
	token, _ := h.csrfToken(w, r)
	data := sitePartFormData{
		Heading:   "Create Site Part",
		Action:    "/admin/appearance/site-parts/new",
		BackURL:   "/admin/appearance/site-parts",
		Name:      name,
		Location:  loc,
		CSRFToken: token,
		Error:     msg,
	}
	if err := h.sitePartFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Create Site Part", ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render create error: %v", err)
	}
}

func (h *Handler) editSitePart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	part, err := h.queries.GetSitePart(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	latest, err := h.queries.GetLatestSitePartRevision(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	status := sitePartStatus(part, map[string]db.SitePartRevision{latest.SitePartID: latest})
	loc := ""
	if row, err := h.queries.GetSitePartLocation(r.Context(), "header"); err == nil && row.SitePartID.Valid && row.SitePartID.String == id {
		loc = "header"
	} else if row, err := h.queries.GetSitePartLocation(r.Context(), "footer"); err == nil && row.SitePartID.Valid && row.SitePartID.String == id {
		loc = "footer"
	}
	if loc == "" {
		intent := strings.TrimSpace(r.URL.Query().Get("intendedLocation"))
		if intent == "header" || intent == "footer" {
			loc = intent
		}
	}
	h.renderSitePartEditor(w, r, part, latest, token, status, loc, "")
}

func (h *Handler) renderSitePartEditor(w http.ResponseWriter, r *http.Request, part db.SitePart, rev db.SitePartRevision, token, status, loc, errMsg string) {
	doc, err := document.Decode([]byte(rev.DocumentJson))
	if err != nil {
		http.Error(w, "Invalid stored document", http.StatusInternalServerError)
		return
	}
	contentTypes, fieldCatalogs := h.editorOptions(r.Context())
	taxonomyCatalogs := h.taxonomyCatalogs(r.Context())
	sitePartsCatalog := []map[string]string{}
	if parts, err := h.queries.ListSiteParts(r.Context()); err == nil {
		for _, p := range parts {
			if p.ID == part.ID {
				continue
			}
			sitePartsCatalog = append(sitePartsCatalog, map[string]string{"id": p.ID, "name": p.Name})
		}
	}
	bootstrap, err := json.Marshal(map[string]any{
		"document": json.RawMessage(rev.DocumentJson), "catalog": h.blocks.EditorCatalogFor("site-part"), "definitions": h.blocks.EditorDefinitions(doc), "previewURL": "/admin/appearance/site-parts/" + part.ID + "/preview", "contentTypes": contentTypes, "fieldCatalogs": fieldCatalogs, "taxonomyCatalogs": taxonomyCatalogs, "siteParts": sitePartsCatalog, "sitePartID": part.ID, "contextKind": "site-part",
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	isPublished := part.PublishedRevisionID.Valid
	locAction := ""
	if loc != "" {
		locAction = "/admin/appearance/site-parts/location"
	}
	data := sitePartEditorData{
		Heading:        "Edit Site Part",
		Action:         "/admin/appearance/site-parts/" + part.ID,
		PublishAction:  "/admin/appearance/site-parts/" + part.ID + "/publish",
		LocationAction: locAction,
		BackURL:        "/admin/appearance/site-parts",
		SitePartID:     part.ID,
		Name:           part.Name,
		DocumentJSON:   rev.DocumentJson,
		EditorJSON:     template.JS(bootstrap),
		CSRFToken:      token,
		Error:          errMsg,
		Dirty:          "Saved",
		Status:         status,
		IsPublished:    isPublished,
		Location:       loc,
		DeleteAction:   "/admin/appearance/site-parts/" + part.ID + "/delete",
		RevisionsURL:   "/admin/appearance/site-parts/" + part.ID + "/revisions",
	}
	if err := h.sitePartEditorTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Edit Site Part - " + part.Name, ActiveMenu: ResolveNav(r.URL.Path).ActiveSection, ActiveSection: ResolveNav(r.URL.Path).ActiveSection, ActiveItem: ResolveNav(r.URL.Path).ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render site part editor: %v", err)
	}
}

func (h *Handler) saveSitePart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, patchElementsEvent("outer", "", editorErrorFragment(errors.New("invalid security token"))), toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	part, err := h.queries.GetSitePart(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	docJSON := postedDocument(r)
	if name == "" {
		h.handleSitePartSaveError(w, r, part, "Name is required")
		return
	}
	if docJSON == "" {
		h.handleSitePartSaveError(w, r, part, "Document is required")
		return
	}
	user, _ := h.currentUser(r)
	author := ""
	if user.ID != "" {
		author = user.ID
	}
	saveErr := h.sitePartsService.SaveDraft(r.Context(), id, name, docJSON, author)
	if saveErr != nil {
		h.handleSitePartSaveError(w, r, part, saveErr.Error())
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Site part draft saved."))
		return
	}
	h.setFlash(w, "Site part draft saved.")
	http.Redirect(w, r, "/admin/appearance/site-parts", http.StatusSeeOther)
}

func (h *Handler) handleSitePartSaveError(w http.ResponseWriter, r *http.Request, part db.SitePart, msg string) {
	if isDatastarRequest(r) {
		writeSSE(w, patchElementsEvent("outer", "", `<p id="editor-error" class="form-error" role="alert">`+template.HTMLEscapeString(msg)+`</p>`), toastEvent("error", msg))
		return
	}
	latest, _ := h.queries.GetLatestSitePartRevision(r.Context(), part.ID)
	token, _ := h.csrfToken(w, r)
	status := sitePartStatus(part, map[string]db.SitePartRevision{latest.SitePartID: latest})
	loc := ""
	if row, err := h.queries.GetSitePartLocation(r.Context(), "header"); err == nil && row.SitePartID.Valid && row.SitePartID.String == part.ID {
		loc = "header"
	} else if row, err := h.queries.GetSitePartLocation(r.Context(), "footer"); err == nil && row.SitePartID.Valid && row.SitePartID.String == part.ID {
		loc = "footer"
	}
	h.renderSitePartEditor(w, r, part, latest, token, status, loc, msg)
}

func (h *Handler) publishSitePart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	part, err := h.queries.GetSitePart(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	docJSON := postedDocument(r)
	if docJSON == "" {
		if latest, lerr := h.queries.GetLatestSitePartRevision(r.Context(), id); lerr == nil {
			docJSON = latest.DocumentJson
		}
		if name == "" {
			name = part.Name
		}
	}
	if name == "" {
		name = part.Name
	}
	if docJSON == "" {
		h.handleSitePartSaveError(w, r, part, "Document is required")
		return
	}
	user, _ := h.currentUser(r)
	author := ""
	if user.ID != "" {
		author = user.ID
	}
	pubErr := h.sitePartsService.Publish(r.Context(), id, name, docJSON, author)
	if pubErr != nil {
		h.handleSitePartSaveError(w, r, part, pubErr.Error())
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateSitePart(id)
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Site part published."))
		return
	}
	h.setFlash(w, "Site part published.")
	http.Redirect(w, r, "/admin/appearance/site-parts", http.StatusSeeOther)
}

func (h *Handler) previewSitePart(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	_, err := h.queries.GetSitePart(r.Context(), id)
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
		if latest, err := h.queries.GetLatestSitePartRevision(r.Context(), id); err == nil {
			docJSON = latest.DocumentJson
		}
	}
	doc, err := document.Decode([]byte(docJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := siteparts.ValidateSitePartDocument(h.blocks, doc); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	input := h.sitePartPreviewInput(r, id, doc)
	if h.documentPreview == nil {
		http.Error(w, "Preview renderer is unavailable", http.StatusServiceUnavailable)
		return
	}
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

func (h *Handler) sitePartPreviewInput(r *http.Request, partID string, candidate *document.Document) RenderInput {
	input := RenderInput{Document: candidate, Title: "Site Part Preview", Path: "/", EntryID: "preview-site-part-" + partID}
	location := ""
	for _, name := range []string{"header", "footer"} {
		if row, err := h.queries.GetSitePartLocation(r.Context(), name); err == nil && row.SitePartID.Valid && row.SitePartID.String == partID {
			location = name
			break
		}
	}
	if location == "" {
		return input
	}
	input.Document = &document.Document{Version: 1, Nodes: []document.Node{{ID: "site-part-preview-main", Block: "core/text", Version: 1, Props: json.RawMessage(`{"text":"Representative page content"}`), Settings: json.RawMessage(`{}`)}}}
	if location == "header" {
		input.HeaderDocument = candidate
	} else {
		input.FooterDocument = candidate
	}
	return input
}

func (h *Handler) setSitePartLocation(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	loc := strings.TrimSpace(r.FormValue("location"))
	sitePartID := strings.TrimSpace(r.FormValue("site_part_id"))
	if sitePartID == "" {
		sitePartID = strings.TrimSpace(r.FormValue("sitePartId"))
	}
	if loc != "header" && loc != "footer" {
		http.Error(w, "Invalid location", http.StatusBadRequest)
		return
	}
	if sitePartID == "" {
		http.Error(w, "Site part is required", http.StatusBadRequest)
		return
	}
	if err := h.sitePartsService.SetLocation(r.Context(), loc, sitePartID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.runtime != nil {
		// A location reassignment changes a global region on every page. Cached
		// pages may still be tagged with the previously assigned Site Part, so
		// selectively invalidating only the new part is insufficient.
		h.runtime.InvalidateAll()
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Location updated."))
		return
	}
	h.setFlash(w, "Location updated.")
	http.Redirect(w, r, "/admin/appearance/site-parts", http.StatusSeeOther)
}

func (h *Handler) clearSitePartLocation(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	loc := strings.TrimSpace(r.FormValue("location"))
	if loc != "header" && loc != "footer" {
		http.Error(w, "Invalid location", http.StatusBadRequest)
		return
	}
	if err := h.queries.ClearSitePartLocation(r.Context(), loc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateAll()
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Location cleared."))
		return
	}
	h.setFlash(w, "Location cleared.")
	http.Redirect(w, r, "/admin/appearance/site-parts", http.StatusSeeOther)
}

func (h *Handler) deleteSitePart(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if used, loc := h.sitePartsService.IsUsedAsHeaderOrFooter(r.Context(), id); used {
		http.Error(w, "Cannot delete site part assigned as "+loc, http.StatusBadRequest)
		return
	}
	referenced, count, referenceErr := h.sitePartsService.IsReferenced(r.Context(), id)
	if referenceErr != nil {
		http.Error(w, "Could not verify Site Part usage", http.StatusInternalServerError)
		return
	}
	if referenced {
		http.Error(w, "Cannot delete Site Part: used by "+fmt.Sprint(count)+" documents.", http.StatusBadRequest)
		return
	}
	if err := h.queries.DeleteSitePart(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateAll()
	}
	h.setFlash(w, "Site part deleted.")
	http.Redirect(w, r, "/admin/appearance/site-parts", http.StatusSeeOther)
}

func (h *Handler) listSitePartRevisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	part, err := h.queries.GetSitePart(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	revisions, err := h.queries.ListSitePartRevisions(r.Context(), id)
	if err != nil {
		http.Error(w, "Revision history is unavailable", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	var out strings.Builder
	out.WriteString(`<!doctype html><meta name="robots" content="noindex,nofollow"><title>Site Part revisions</title><main><h1>Revision history: ` + template.HTMLEscapeString(part.Name) + `</h1><p><a href="/admin/appearance/site-parts/` + template.URLQueryEscaper(id) + `/edit">Back to editor</a></p><ol>`)
	latestID := ""
	if len(revisions) > 0 {
		latestID = revisions[0].ID
	}
	for _, revision := range revisions {
		status := "Revision " + fmt.Sprint(revision.RevisionNumber)
		if part.PublishedRevisionID.Valid && part.PublishedRevisionID.String == revision.ID {
			status += " · Published"
		} else if revision.ID == latestID {
			status += " · Current draft"
		}
		author := ""
		if revision.CreatedBy.Valid {
			author = " · Author " + template.HTMLEscapeString(revision.CreatedBy.String)
		}
		createdAt := time.Unix(revision.CreatedAt, 0).Format("2006-01-02 15:04")
		out.WriteString(`<li><strong>` + template.HTMLEscapeString(status) + `</strong> · <time>` + createdAt + `</time>` + author + ` <a href="/admin/appearance/site-parts/` + template.URLQueryEscaper(id) + `/revisions/` + template.URLQueryEscaper(revision.ID) + `/preview">Preview</a> <form method="post" action="/admin/appearance/site-parts/` + template.URLQueryEscaper(id) + `/revisions/` + template.URLQueryEscaper(revision.ID) + `/restore" style="display:inline"><input type="hidden" name="csrf_token" value="` + template.HTMLEscapeString(token) + `"><button type="submit">Restore</button></form></li>`)
	}
	out.WriteString(`</ol></main>`)
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(out.String()))
}

func (h *Handler) previewSitePartRevision(w http.ResponseWriter, r *http.Request) {
	id, revisionID := r.PathValue("id"), r.PathValue("revisionID")
	if _, err := h.queries.GetSitePart(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}
	revision, err := h.queries.GetSitePartRevision(r.Context(), revisionID)
	if err != nil || revision.SitePartID != id {
		http.NotFound(w, r)
		return
	}
	doc, err := document.Decode([]byte(revision.DocumentJson))
	if err != nil || siteparts.ValidateSitePartDocument(h.blocks, doc) != nil || h.documentPreview == nil {
		http.Error(w, "Invalid Site Part revision", http.StatusUnprocessableEntity)
		return
	}
	page, err := h.documentPreview(r.Context(), h.sitePartPreviewInput(r, id, doc))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (h *Handler) restoreSitePartRevision(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, _ := h.currentUser(r)
	if _, err := h.sitePartsService.RestoreRevision(r.Context(), r.PathValue("id"), r.PathValue("revisionID"), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.setFlash(w, "Revision restored as a new draft.")
	http.Redirect(w, r, "/admin/appearance/site-parts/"+r.PathValue("id")+"/edit", http.StatusSeeOther)
}
