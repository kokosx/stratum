package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"html"
	"log"
	"net/http"
	"time"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/seo"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const pageContentType = "page"

func (h *Handler) newPage(w http.ResponseWriter, r *http.Request) {
	ctID := pageContentType
	// Preselect default layout template for new entries
	var defaultTpl string
	if ct, err := h.queries.GetContentType(r.Context(), ctID); err == nil && ct.DefaultLayoutTemplateID.Valid {
		defaultTpl = ct.DefaultLayoutTemplateID.String
	}
	h.renderEntryForm(w, r, entryFormData{
		Heading:          "Add New Page",
		Action:           "/admin/pages",
		PublishAction:    "/admin/pages",
		BackURL:          "/admin/pages",
		DocumentJSON:     `{"version":1,"nodes":[]}`,
		Dirty:            "Saved",
		Status:           "Draft",
		ShowSEO:          true,
		ShowFeatured:     true,
		ContentTypeID:    ctID,
		LayoutTemplateID: defaultTpl,
		LayoutTemplates:  h.loadLayoutTemplateOptions(r.Context(), ctID),
		CommentsEnabled:  false,
		SupportsComments: content.DefinitionFor(ctID).Capabilities.SupportsComments,
	}, "pages")
}

func (h *Handler) createPage(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	input, err := readEntryInput(r, pageContentType)
	if err != nil {
		h.renderEntryForm(w, r, entryFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages", BackURL: "/admin/pages", Title: r.FormValue("title"), Slug: r.FormValue("slug"), SEOTitle: r.FormValue("seo_title"), SEODescription: r.FormValue("seo_description"), CanonicalURL: r.FormValue("canonical_url"), FeaturedMediaID: r.FormValue("featured_media_id"), SocialMediaID: r.FormValue("social_media_id"), RobotsIndex: r.FormValue("seo_robots_index"), RobotsFollow: r.FormValue("seo_robots_follow"), SchemaMode: r.FormValue("schema_mode"), DocumentJSON: postedDocument(r), ContentTypeID: pageContentType, LayoutTemplateID: r.FormValue("layout_template_id"), LayoutTemplates: h.loadLayoutTemplateOptions(r.Context(), pageContentType), Error: err.Error(), ShowSEO: true, ShowFeatured: true, CommentsEnabled: input.commentsEnabled, SupportsComments: content.DefinitionFor(pageContentType).Capabilities.SupportsComments}, "pages")
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
		h.renderEntryForm(w, r, entryFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages", BackURL: "/admin/pages", Title: input.title, Slug: input.slug, SEOTitle: input.seoTitle, SEODescription: input.seoDescription, CanonicalURL: input.canonicalURL, FeaturedMediaID: input.featuredMediaID, SocialMediaID: input.socialMediaID, RobotsIndex: robotsInputFormValue(input.robotsIndex), RobotsFollow: robotsInputFormValue(input.robotsFollow), SchemaMode: input.schemaMode, DocumentJSON: input.documentJSON, ContentTypeID: pageContentType, LayoutTemplateID: input.layoutTemplateID, LayoutTemplates: h.loadLayoutTemplateOptions(r.Context(), pageContentType), Error: entryWriteError(err), ShowSEO: true, ShowFeatured: true, CommentsEnabled: input.commentsEnabled, SupportsComments: content.DefinitionFor(pageContentType).Capabilities.SupportsComments}, "pages")
		return
	}
	if r.FormValue("publish") != "" && h.runtime != nil {
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
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
	isPostsPage := settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entry.ID
	postsPath := ""
	warning := ""
	if isPostsPage {
		if settings.HomepageMode == "latest_posts" {
			postsPath = "/"
		} else {
			postsPath = seo.PostsArchivePath(settings.PostsBasePath)
			if postsPath == "" {
				postsPath = seo.DefaultPostsBase
			}
		}
		// Check if published/latest revision contains an archive Posts block (automatic is alias for archive)
		hasArchiveBlock := false
		if doc, dErr := document.Decode([]byte(revision.DocumentJson)); dErr == nil {
			var walk func([]document.Node)
			walk = func(nodes []document.Node) {
				for _, n := range nodes {
					if n.Block == "core/posts" {
						source := "automatic"
						if len(n.Settings) > 0 {
							var s map[string]any
							if json.Unmarshal(n.Settings, &s) == nil {
								if v, ok := s["source"].(string); ok && v != "" {
									source = v
								}
							}
						}
						if source == "archive" {
							source = "automatic"
						}
						if source == "automatic" {
							hasArchiveBlock = true
						}
					}
					if len(n.Children) > 0 {
						walk(n.Children)
					}
				}
			}
			walk(doc.Nodes)
		}
		if !hasArchiveBlock {
			warning = "This Posts Page does not contain a Posts block. Posts will not be visible on the archive."
		}
	}
	hasUnpublished := entry.PublishedRevisionID.Valid && entry.PublishedRevisionID.String != revision.ID
	layoutID := ""
	if revision.LayoutTemplateID.Valid {
		layoutID = revision.LayoutTemplateID.String
	}
	// Publishing metadata for editor panel
	visibility := revision.Visibility
	if visibility == "" {
		visibility = "public"
	}
	sticky := revision.Sticky != 0
	// Scheduled job
	var scheduledAt string
	var scheduledUnix int64
	hasScheduled := false
	if job, err := h.queries.GetActivePublicationJobByEntry(r.Context(), entry.ID); err == nil {
		hasScheduled = true
		scheduledUnix = job.ScheduledAt
		loc := time.UTC
		if l, err := time.LoadLocation(settings.Timezone); err == nil {
			loc = l
		}
		scheduledAt = time.Unix(job.ScheduledAt, 0).In(loc).Format("2006-01-02T15:04")
	}
	h.renderEntryForm(w, r, entryFormData{
		Heading:               "Edit Page",
		Action:                "/admin/pages/" + entry.ID,
		PublishAction:         "/admin/pages/" + entry.ID + "/publish",
		BackURL:               "/admin/pages",
		Title:                 revision.Title,
		Slug:                  revision.Slug,
		Excerpt:               stringValue(revision.Excerpt),
		SEOTitle:              stringValue(revision.SeoTitle),
		SEODescription:        stringValue(revision.SeoDescription),
		CanonicalURL:          stringValue(revision.CanonicalUrl),
		FeaturedMediaID:       stringValue(revision.FeaturedMediaID),
		SocialMediaID:         stringValue(revision.SocialMediaID),
		RobotsIndex:           robotsFormValue(revision.SeoRobotsIndex),
		RobotsFollow:          robotsFormValue(revision.SeoRobotsFollow),
		SchemaMode:            revision.SchemaMode,
		SiteURL:               settings.SiteUrl,
		PublicPath:            h.entryPublicPath(r, entry.ID),
		EntryID:               entry.ID,
		DocumentJSON:          revision.DocumentJson,
		FieldValues:           fieldValues(revision.FieldsJson),
		Dirty:                 "Saved",
		Status:                status,
		PublicURL:             publicURL,
		ShowSEO:               true,
		ShowFeatured:          true,
		IsPostsPage:           isPostsPage,
		PostsPagePath:         postsPath,
		PostsPageWarning:      warning,
		HasUnpublishedChanges: hasUnpublished,
		ContentTypeID:         pageContentType,
		LayoutTemplateID:      layoutID,
		LayoutTemplates:       h.loadLayoutTemplateOptions(r.Context(), pageContentType),
		ParentEntryID:         stringValue(revision.ParentEntryID),
		MenuOrder:             revision.MenuOrder,
		Revisions:             h.revisionHistory(r.Context(), entry),
		Visibility:            visibility,
		Sticky:                sticky,
		SupportsSticky:        content.DefinitionFor(pageContentType).Capabilities.SupportsSticky,
		ScheduledAt:           scheduledAt,
		ScheduledAtUnix:       scheduledUnix,
		HasScheduled:          hasScheduled,
		ReviewState:           revision.ReviewState,
		CommentsEnabled:       revision.CommentsEnabled != 0,
		SupportsComments:      content.DefinitionFor(pageContentType).Capabilities.SupportsComments,
	}, "pages")
}

// entryEditorStatus derives the displayed status label and public URL for an
// entry from its publish state, including scheduled, pending, private and password.
func (h *Handler) entryEditorStatus(r *http.Request, entry db.Entry) (string, string) {
	latest, err := h.queries.GetLatestEntryRevision(r.Context(), entry.ID)
	hasScheduled := false
	if _, err2 := h.queries.GetActivePublicationJobByEntry(r.Context(), entry.ID); err2 == nil {
		hasScheduled = true
	}
	if hasScheduled {
		publicURL := ""
		if path := h.entryPublicPath(r, entry.ID); path != "" {
			publicURL = absoluteURL(r, path)
		}
		return "Scheduled", publicURL
	}
	// Determine base status from published visibility.
	if !entry.PublishedRevisionID.Valid {
		if err == nil && latest.ReviewState == "pending" {
			return "Pending Review", ""
		}
		return "Draft", ""
	}
	// Has published – inspect visibility for Private/Password labels.
	baseStatus := "Published"
	if pubRev, pErr := h.queries.GetEntryRevision(r.Context(), entry.PublishedRevisionID.String); pErr == nil {
		switch pubRev.Visibility {
		case "private":
			baseStatus = "Private"
		case "password":
			baseStatus = "Password Protected"
		}
	}
	publicURL := ""
	if path := h.entryPublicPath(r, entry.ID); path != "" {
		publicURL = absoluteURL(r, path)
	}
	// If unpublished draft exists, communicate both states.
	if err == nil && latest.ID != entry.PublishedRevisionID.String {
		if latest.ReviewState == "pending" {
			return baseStatus + " · Pending Review", publicURL
		}
		return baseStatus + " · Unpublished changes", publicURL
	}
	return baseStatus, publicURL
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

func (h *Handler) restorePageRevision(w http.ResponseWriter, r *http.Request) {
	h.restoreRevision(w, r, pageContentType, "pages")
}
func (h *Handler) unpublishPage(w http.ResponseWriter, r *http.Request) {
	h.unpublishEntry(w, r, pageContentType, "pages")
}
func (h *Handler) previewPageRevision(w http.ResponseWriter, r *http.Request) {
	h.previewRevision(w, r, pageContentType)
}
func (h *Handler) schedulePage(w http.ResponseWriter, r *http.Request) {
	h.scheduleEntry(w, r, pageContentType, "pages")
}
func (h *Handler) cancelSchedulePage(w http.ResponseWriter, r *http.Request) {
	h.cancelScheduleEntry(w, r, pageContentType, "pages")
}
func (h *Handler) submitReviewPage(w http.ResponseWriter, r *http.Request) {
	h.submitReviewEntry(w, r, pageContentType, "pages")
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
	input, err := readEntryInput(r, contentType)
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
	if saveErr == nil && publish && h.runtime != nil {
		if content.DefinitionFor(contentType).Capabilities.Hierarchical {
			// A parent publish can move every descendant path. Drop old path cache
			// entries before the single route-runtime reload exposes redirects.
			h.runtime.InvalidateContent()
			_ = h.runtime.ReloadRoutes(r.Context())
		} else {
			h.runtime.InvalidateEntry(entryID, contentType)
			h.runtime.InvalidateContent()
		}
		// If this entry is the Posts Page, posts_base_path may have changed – ensure site snapshot is fresh.
		// We still reload site but only invalidate site tag if reload succeeds; fallback already handled by InvalidateEntry.
		if s, err := h.queries.GetSiteSettings(r.Context()); err == nil && s.PostsPageEntryID.Valid && s.PostsPageEntryID.String == entryID {
			_ = h.runtime.ReloadSite(r.Context())
		}
	}
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
		Heading:          "Edit " + contentTypeTitle(contentType),
		Action:           "/admin/" + activeMenu + "/" + entryID,
		PublishAction:    "/admin/" + activeMenu + "/" + entryID + "/publish",
		BackURL:          "/admin/" + activeMenu,
		Title:            input.title,
		Slug:             input.slug,
		Excerpt:          input.excerpt,
		SEOTitle:         input.seoTitle,
		SEODescription:   input.seoDescription,
		CanonicalURL:     input.canonicalURL,
		FeaturedMediaID:  input.featuredMediaID,
		SocialMediaID:    input.socialMediaID,
		RobotsIndex:      robotsInputFormValue(input.robotsIndex),
		RobotsFollow:     robotsInputFormValue(input.robotsFollow),
		SchemaMode:       input.schemaMode,
		DocumentJSON:     input.documentJSON,
		ContentTypeID:    contentType,
		LayoutTemplateID: input.layoutTemplateID,
		LayoutTemplates:  h.loadLayoutTemplateOptions(r.Context(), contentType),
		ParentEntryID:    input.parentEntryID,
		MenuOrder:        input.menuOrder,
		Error:            entryWriteError(saveErr),
		Dirty:            "Unsaved",
		Status:           "Draft",
		ShowSEO:          true,
		ShowFeatured:     true,
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
		if entry, err := h.queries.GetEntry(r.Context(), entryID); err == nil {
			status, publicURL = h.entryEditorStatus(r, entry)
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
