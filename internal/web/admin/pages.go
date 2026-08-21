package admin

import (
	"bytes"
	"database/sql"
	"errors"
	"html"
	"log"
	"net/http"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const pageContentType = "page"

func (h *Handler) newPage(w http.ResponseWriter, r *http.Request) {
	h.renderEntryForm(w, r, entryFormData{
		Heading:       "Add New Page",
		Action:        "/admin/pages",
		PublishAction: "/admin/pages",
		BackURL:       "/admin/pages",
		DocumentJSON:  `{"version":1,"nodes":[]}`,
		Dirty:         "Saved",
		Status:        "Draft",
		ShowSEO:       true,
	}, "pages")
}

func (h *Handler) createPage(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	input, err := readEntryInput(r)
	if err != nil {
		h.renderEntryForm(w, r, entryFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages", BackURL: "/admin/pages", Title: r.FormValue("title"), Slug: r.FormValue("slug"), SEOTitle: r.FormValue("seo_title"), SEODescription: r.FormValue("seo_description"), CanonicalURL: r.FormValue("canonical_url"), DocumentJSON: postedDocument(r), Error: err.Error(), ShowSEO: true}, "pages")
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	entryID, err := randomID()
	if err == nil {
		err = h.writeEntry(r.Context(), pageContentType, user.ID, entryID, input, true, r.FormValue("publish") != "")
	}
	if err != nil {
		log.Printf("create page: %v", err)
		h.renderEntryForm(w, r, entryFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages", BackURL: "/admin/pages", Title: input.title, Slug: input.slug, SEOTitle: input.seoTitle, SEODescription: input.seoDescription, CanonicalURL: input.canonicalURL, DocumentJSON: input.documentJSON, Error: entryWriteError(err), ShowSEO: true}, "pages")
		return
	}
	if r.FormValue("publish") != "" {
		h.setFlash(w, "Page published.")
	} else {
		h.setFlash(w, "Page saved as draft.")
	}
	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (h *Handler) editPage(w http.ResponseWriter, r *http.Request) {
	entry, revision, err := h.entryAndLatestRevision(r.Context(), r.PathValue("id"), pageContentType)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status, publicURL := h.entryEditorStatus(r, entry)
	settings, _ := h.queries.GetSiteSettings(r.Context())
	h.renderEntryForm(w, r, entryFormData{
		Heading:       "Edit Page",
		Action:        "/admin/pages/" + entry.ID,
		PublishAction: "/admin/pages/" + entry.ID + "/publish",
		BackURL:       "/admin/pages",
		Title:         revision.Title,
		Slug:          entry.Slug,
		DocumentJSON:  revision.DocumentJson,
		CanonicalURL:  stringValue(revision.CanonicalUrl),
		SiteURL:       settings.SiteUrl,
		PublicPath:    h.entryPublicPath(r, entry.ID),
		Dirty:         "Saved",
		Status:        status,
		PublicURL:     publicURL,
		ShowSEO:       true,
	}, "pages")
}

// entryEditorStatus derives the displayed status label and public URL for an
// entry from its publish state.
func (h *Handler) entryEditorStatus(r *http.Request, entry db.Entry) (string, string) {
	if !entry.PublishedRevisionID.Valid {
		return "Draft", ""
	}
	publicURL := ""
	if path := h.entryPublicPath(r, entry.ID); path != "" {
		publicURL = absoluteURL(r, path)
	}
	return "Published", publicURL
}

// entryPublicPath returns the public route path for an entry, or "" if it is
// not published.
func (h *Handler) entryPublicPath(r *http.Request, entryID string) string {
	route, err := h.queries.GetEntryRoute(r.Context(), sql.NullString{String: entryID, Valid: true})
	if err != nil {
		return ""
	}
	return route.Path
}

func (h *Handler) savePage(w http.ResponseWriter, r *http.Request) {
	h.updateEntry(w, r, pageContentType, "pages", "/admin/pages", false)
}

func (h *Handler) publishPage(w http.ResponseWriter, r *http.Request) {
	h.updateEntry(w, r, pageContentType, "pages", "/admin/pages", true)
}

// updateEntry is shared by Pages and Posts. It validates the posted input and
// writes a new revision (preserving the public document until publish). The
// publish flag is decided by the route, not by a form field, so the editor can
// fire Save Draft and Publish through the same form via Datastar.
//
// When the request comes from Datastar the handler responds with SSE fragment
// patches (status region, inline error, toast) and keeps the editor mounted.
// Without the Datastar header it falls back to the classic full-page render or
// redirect, preserving progressive enhancement.
func (h *Handler) updateEntry(w http.ResponseWriter, r *http.Request, contentType, activeMenu, listingURL string, publish bool) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, patchElementsEvent("outer", "", editorErrorFragment(errors.New("invalid security token"))), toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	entryID := r.PathValue("id")
	input, err := readEntryInput(r)
	if err != nil {
		if isDatastarRequest(r) {
			h.editorSaveFragment(w, r, contentType, activeMenu, entryID, publish, input, err)
			return
		}
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, err)
		return
	}
	if _, _, err := h.entryAndLatestRevision(r.Context(), entryID, contentType); err != nil {
		http.NotFound(w, r)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	saveErr := h.writeEntry(r.Context(), contentType, user.ID, entryID, input, false, publish)
	if isDatastarRequest(r) {
		h.editorSaveFragment(w, r, contentType, activeMenu, entryID, publish, input, saveErr)
		return
	}
	if saveErr != nil {
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, saveErr)
		return
	}
	if publish {
		h.setFlash(w, contentTypeTitle(contentType)+" published.")
	} else {
		h.setFlash(w, contentTypeTitle(contentType)+" saved as draft.")
	}
	http.Redirect(w, r, listingURL, http.StatusSeeOther)
}

// renderEntryError re-renders the full editor form with the posted values and
// an inline error message. Used by the no-JS fallback path.
func (h *Handler) renderEntryError(w http.ResponseWriter, r *http.Request, contentType, activeMenu, entryID string, input entryInput, saveErr error) {
	data := entryFormData{
		Heading:        "Edit " + contentTypeTitle(contentType),
		Action:         "/" + activeMenu + "/" + entryID,
		PublishAction:  "/" + activeMenu + "/" + entryID + "/publish",
		BackURL:        "/" + activeMenu,
		Title:          input.title,
		Slug:           input.slug,
		Excerpt:        input.excerpt,
		SEOTitle:       input.seoTitle,
		SEODescription: input.seoDescription,
		CanonicalURL:   input.canonicalURL,
		DocumentJSON:   input.documentJSON,
		Error:          entryWriteError(saveErr),
		Dirty:          "Unsaved",
		Status:         "Draft",
		ShowSEO:        true,
	}
	if contentType == postContentType {
		data.ShowExcerpt = true
	}
	h.renderEntryForm(w, r, data, activeMenu)
}

// editorSaveFragment responds to a Datastar save/publish request. It patches
// the editor status region, the inline error paragraph, and a toast without
// reloading the document or kicking the user out of the editor.
func (h *Handler) editorSaveFragment(w http.ResponseWriter, r *http.Request, contentType, activeMenu, entryID string, publish bool, input entryInput, saveErr error) {
	view := h.editorStatusView(r, entryID, publish, saveErr)
	var statusBuf bytes.Buffer
	if err := h.entryTemplate.ExecuteTemplate(&statusBuf, "editor-status-region", view); err != nil {
		log.Printf("render editor status fragment: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	events := []sseEvent{
		patchElementsEvent("inner", "#editor-status-region", statusBuf.String()),
		patchElementsEvent("outer", "", editorErrorFragment(saveErr)),
	}
	if saveErr != nil {
		events = append(events, toastEvent("error", entryWriteError(saveErr)))
	} else if publish {
		events = append(events, toastEvent("success", contentTypeTitle(contentType)+" published."))
	} else {
		events = append(events, toastEvent("success", contentTypeTitle(contentType)+" draft saved."))
	}
	writeSSE(w, events...)
}

// editorStatusView derives the post-save status region values from the entry:
// whether it is published (and its public URL) and whether the dirty indicator
// should read "Saved" or "Unsaved".
func (h *Handler) editorStatusView(r *http.Request, entryID string, publish bool, saveErr error) editorStatusView {
	dirty := "Saved"
	if saveErr != nil {
		dirty = "Unsaved"
	}
	status := "Draft"
	publicURL := ""
	if saveErr == nil {
		if entry, err := h.queries.GetEntry(r.Context(), entryID); err == nil && entry.PublishedRevisionID.Valid {
			status = "Published"
			if path := h.entryPublicPath(r, entryID); path != "" {
				publicURL = absoluteURL(r, path)
			}
		}
	}
	return editorStatusView{Dirty: dirty, Status: status, PublicURL: publicURL}
}

// editorErrorFragment returns the #editor-error element, shown with a message
// on failure and hidden on success.
func editorErrorFragment(saveErr error) string {
	if saveErr == nil {
		return `<p id="editor-error" class="form-error" role="alert" hidden></p>`
	}
	return `<p id="editor-error" class="form-error" role="alert">` + html.EscapeString(entryWriteError(saveErr)) + `</p>`
}

func absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func contentTypeTitle(contentType string) string {
	switch contentType {
	case pageContentType:
		return "Page"
	case postContentType:
		return "Post"
	}
	return contentType
}
