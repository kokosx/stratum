package admin

import (
	"log"
	"net/http"
	"time"

	"github.com/kokosx/stratum/internal/content"
)

func (h *Handler) customDefinition(w http.ResponseWriter, r *http.Request) (content.ContentTypeDefinition, bool) {
	definition, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), r.PathValue("type"))
	if err != nil || definition.ID == content.ContentTypePage || definition.ID == content.ContentTypePost {
		http.NotFound(w, r)
		return content.ContentTypeDefinition{}, false
	}
	return definition, true
}

func (h *Handler) editCustomEntry(w http.ResponseWriter, r *http.Request) {
	definition, ok := h.customDefinition(w, r)
	if !ok {
		return
	}
	entry, revision, err := h.entryAndLatestRevision(r.Context(), r.PathValue("id"), string(definition.ID))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status, publicURL := h.entryEditorStatus(r, entry)
	layoutID := ""
	if revision.LayoutTemplateID.Valid {
		layoutID = revision.LayoutTemplateID.String
	}
	settings, _ := h.queries.GetSiteSettings(r.Context())
	scheduledAt := ""
	hasScheduled := false
	if job, err := h.queries.GetActivePublicationJobByEntry(r.Context(), entry.ID); err == nil {
		hasScheduled = true
		loc := time.UTC
		if l, e := time.LoadLocation(settings.Timezone); e == nil {
			loc = l
		}
		scheduledAt = time.Unix(job.ScheduledAt, 0).In(loc).Format("2006-01-02T15:04")
	}
	base := "/admin/content/" + string(definition.ID)
	heading := "Edit item"
	if definition.ItemLabel() != "" {
		heading = "Edit " + definition.ItemLabel()
	}
	data := entryFormData{Heading: heading, Action: base + "/" + entry.ID, PublishAction: base + "/" + entry.ID + "/publish", BackURL: base, Title: revision.Title, Slug: revision.Slug, Excerpt: stringValue(revision.Excerpt), SEOTitle: stringValue(revision.SeoTitle), SEODescription: stringValue(revision.SeoDescription), CanonicalURL: stringValue(revision.CanonicalUrl), FeaturedMediaID: stringValue(revision.FeaturedMediaID), SocialMediaID: stringValue(revision.SocialMediaID), RobotsIndex: robotsFormValue(revision.SeoRobotsIndex), RobotsFollow: robotsFormValue(revision.SeoRobotsFollow), SchemaMode: revision.SchemaMode, SiteURL: settings.SiteUrl, PublicPath: h.entryPublicPath(r, entry.ID), EntryID: entry.ID, DocumentJSON: revision.DocumentJson, FieldValues: fieldValues(revision.FieldsJson), Dirty: "Saved", Status: status, PublicURL: publicURL, V2URL: base + "/" + entry.ID + "/edit?editor=v2", HasUnpublishedChanges: entry.PublishedRevisionID.Valid && entry.PublishedRevisionID.String != revision.ID, ContentTypeID: string(definition.ID), LayoutTemplateID: layoutID, LayoutTemplates: h.loadLayoutTemplateOptions(r.Context(), string(definition.ID)), ParentEntryID: stringValue(revision.ParentEntryID), MenuOrder: revision.MenuOrder, Hierarchical: definition.Capabilities.Hierarchical, Revisions: h.revisionHistory(r.Context(), entry), Visibility: revision.Visibility, ReviewState: revision.ReviewState, ScheduledAt: scheduledAt, HasScheduled: hasScheduled}
	if r.URL.Query().Get("editor") == "v2" {
		h.renderEntryFormV2(w, r, data)
		return
	}
	h.renderEntryForm(w, r, data, "content/"+string(definition.ID))
}
func (h *Handler) saveCustomEntry(w http.ResponseWriter, r *http.Request) {
	h.customUpdateEntry(w, r, false)
}
func (h *Handler) publishCustomEntry(w http.ResponseWriter, r *http.Request) {
	h.customUpdateEntry(w, r, true)
}
func (h *Handler) customUpdateEntry(w http.ResponseWriter, r *http.Request, publish bool) {
	d, ok := h.customDefinition(w, r)
	if !ok {
		return
	}
	base := "/admin/content/" + string(d.ID)
	h.updateEntry(w, r, string(d.ID), "content/"+string(d.ID), base, publish)
}
func (h *Handler) unpublishCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.unpublishEntry(w, r, string(d.ID), "content/"+string(d.ID))
	}
}
func (h *Handler) scheduleCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.scheduleEntry(w, r, string(d.ID), "content/"+string(d.ID))
	}
}
func (h *Handler) cancelScheduleCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.cancelScheduleEntry(w, r, string(d.ID), "content/"+string(d.ID))
	}
}
func (h *Handler) submitReviewCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.submitReviewEntry(w, r, string(d.ID), "content/"+string(d.ID))
	}
}
func (h *Handler) restoreCustomEntryRevision(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.restoreRevision(w, r, string(d.ID), "content/"+string(d.ID))
	}
}
func (h *Handler) previewCustomEntryRevision(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.previewRevision(w, r, string(d.ID))
	}
}
func (h *Handler) trashCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.trashEntry(w, r, string(d.ID), "/admin/content/"+string(d.ID))
	}
}
func (h *Handler) restoreCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.restoreEntry(w, r, string(d.ID), "/admin/content/"+string(d.ID))
	}
}
func (h *Handler) deleteCustomEntry(w http.ResponseWriter, r *http.Request) {
	d, ok := h.customDefinition(w, r)
	if ok {
		h.deleteEntryPermanently(w, r, string(d.ID), "/admin/content/"+string(d.ID))
	}
}
func (h *Handler) listCustomEntries(w http.ResponseWriter, r *http.Request) {
	definition, ok := h.customDefinition(w, r)
	if !ok {
		return
	}
	label := definition.Label()
	h.listEntries(w, r, string(definition.ID), label, "content/"+string(definition.ID))
}
func (h *Handler) newCustomEntry(w http.ResponseWriter, r *http.Request) {
	definition, ok := h.customDefinition(w, r)
	if !ok {
		return
	}
	base := "/admin/content/" + string(definition.ID)
	heading := "Add item"
	if definition.ItemLabel() != "" {
		heading = "Add " + definition.ItemLabel()
	} else {
		heading = "Add item"
	}
	h.renderEntryForm(w, r, entryFormData{Heading: heading, Action: base, PublishAction: base, BackURL: base, DocumentJSON: `{"version":1,"nodes":[]}`, Dirty: "Saved", Status: "Draft", ContentTypeID: string(definition.ID), Hierarchical: definition.Capabilities.Hierarchical, LayoutTemplates: h.loadLayoutTemplateOptions(r.Context(), string(definition.ID))}, "content/"+string(definition.ID))
}
func (h *Handler) createCustomEntry(w http.ResponseWriter, r *http.Request) {
	definition, ok := h.customDefinition(w, r)
	if !ok {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	input, err := readEntryInput(r, string(definition.ID))
	// readEntryInput keeps the historic builtin fallback for older callers; a
	// custom type must use its DB-backed schema to capture revision fields.
	input.fields = rawFieldValues(r, definition)
	base := "/admin/content/" + string(definition.ID)
	heading := "Add item"
	if definition.ItemLabel() != "" {
		heading = "Add " + definition.ItemLabel()
	}
	if err != nil {
		h.renderEntryForm(w, r, entryFormData{Heading: heading, Action: base, PublishAction: base, BackURL: base, Title: r.FormValue("title"), Slug: r.FormValue("slug"), DocumentJSON: postedDocument(r), ContentTypeID: string(definition.ID), Error: err.Error()}, "content/"+string(definition.ID))
		return
	}
	user, err := h.currentUser(r)
	var createdID string
	if err == nil {
		createdID, err = randomID()
		if err == nil {
			err = h.writeEntry(r.Context(), string(definition.ID), user.ID, createdID, input, true, r.FormValue("publish") != "")
		}
	}
	if err != nil {
		log.Printf("create custom entry: %v", err)
		h.renderEntryForm(w, r, entryFormData{Heading: heading, Action: base, PublishAction: base, BackURL: base, Title: input.title, Slug: input.slug, DocumentJSON: input.documentJSON, ContentTypeID: string(definition.ID), Error: entryWriteError(err)}, "content/"+string(definition.ID))
		return
	}
	if r.FormValue("publish") != "" && h.runtime != nil {
		// Publishing route-less structured data must invalidate dependent Collections via content-type tag,
		// not via route existence.
		h.runtime.InvalidateEntry(createdID, string(definition.ID))
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Saved.")
	http.Redirect(w, r, base, http.StatusSeeOther)
}
