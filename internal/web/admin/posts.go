package admin

import (
	"log"
	"net/http"
)

const postContentType = "post"

func (h *Handler) newPost(w http.ResponseWriter, r *http.Request) {
	ctID := postContentType
	var defaultTpl string
	if ct, err := h.queries.GetContentType(r.Context(), ctID); err == nil && ct.DefaultLayoutTemplateID.Valid {
		defaultTpl = ct.DefaultLayoutTemplateID.String
	}
	h.renderEntryForm(w, r, entryFormData{
		Heading:          "Add New Post",
		Action:           "/admin/posts",
		PublishAction:    "/admin/posts",
		BackURL:          "/admin/posts",
		DocumentJSON:     `{"version":1,"nodes":[]}`,
		Dirty:            "Saved",
		Status:           "Draft",
		ShowExcerpt:      true,
		ShowSEO:          true,
		ShowFeatured:     true,
		ContentTypeID:    ctID,
		LayoutTemplateID: defaultTpl,
		LayoutTemplates:  h.loadLayoutTemplateOptions(r.Context(), ctID),
	}, "posts")
}

func (h *Handler) createPost(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	input, err := readEntryInput(r)
	if err != nil {
		h.renderEntryForm(w, r, entryFormData{Heading: "Add New Post", Action: "/admin/posts", PublishAction: "/admin/posts", BackURL: "/admin/posts", Title: r.FormValue("title"), Slug: r.FormValue("slug"), Excerpt: r.FormValue("excerpt"), SEOTitle: r.FormValue("seo_title"), SEODescription: r.FormValue("seo_description"), CanonicalURL: r.FormValue("canonical_url"), FeaturedMediaID: r.FormValue("featured_media_id"), SocialMediaID: r.FormValue("social_media_id"), RobotsIndex: r.FormValue("seo_robots_index"), RobotsFollow: r.FormValue("seo_robots_follow"), SchemaMode: r.FormValue("schema_mode"), DocumentJSON: postedDocument(r), ContentTypeID: postContentType, LayoutTemplateID: r.FormValue("layout_template_id"), LayoutTemplates: h.loadLayoutTemplateOptions(r.Context(), postContentType), Error: err.Error(), ShowExcerpt: true, ShowSEO: true, ShowFeatured: true}, "posts")
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	entryID, err := randomID()
	if err == nil {
		err = h.writeEntry(r.Context(), postContentType, user.ID, entryID, input, true, r.FormValue("publish") != "")
	}
	if err != nil {
		log.Printf("create post: %v", err)
		h.renderEntryForm(w, r, entryFormData{Heading: "Add New Post", Action: "/admin/posts", PublishAction: "/admin/posts", BackURL: "/admin/posts", Title: input.title, Slug: input.slug, Excerpt: input.excerpt, SEOTitle: input.seoTitle, SEODescription: input.seoDescription, CanonicalURL: input.canonicalURL, FeaturedMediaID: input.featuredMediaID, SocialMediaID: input.socialMediaID, RobotsIndex: robotsInputFormValue(input.robotsIndex), RobotsFollow: robotsInputFormValue(input.robotsFollow), SchemaMode: input.schemaMode, DocumentJSON: input.documentJSON, ContentTypeID: postContentType, LayoutTemplateID: input.layoutTemplateID, LayoutTemplates: h.loadLayoutTemplateOptions(r.Context(), postContentType), Error: entryWriteError(err), ShowExcerpt: true, ShowSEO: true, ShowFeatured: true}, "posts")
		return
	}
	if r.FormValue("publish") != "" && h.runtime != nil {
		h.runtime.InvalidateEntry(entryID, postContentType)
	}
	if r.FormValue("publish") != "" {
		h.setFlash(w, "Post published.")
	} else {
		h.setFlash(w, "Post saved as draft.")
	}
	http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
}

func (h *Handler) editPost(w http.ResponseWriter, r *http.Request) {
	entry, revision, err := h.entryAndLatestRevision(r.Context(), r.PathValue("id"), postContentType)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status, publicURL := h.entryEditorStatus(r, entry)
	settings, _ := h.queries.GetSiteSettings(r.Context())
	layoutID := ""
	if revision.LayoutTemplateID.Valid {
		layoutID = revision.LayoutTemplateID.String
	}
	h.renderEntryForm(w, r, entryFormData{
		Heading:          "Edit Post",
		Action:           "/admin/posts/" + entry.ID,
		PublishAction:    "/admin/posts/" + entry.ID + "/publish",
		BackURL:          "/admin/posts",
		Title:            revision.Title,
		Slug:             entry.Slug,
		Excerpt:          stringValue(revision.Excerpt),
		SEOTitle:         stringValue(revision.SeoTitle),
		SEODescription:   stringValue(revision.SeoDescription),
		CanonicalURL:     stringValue(revision.CanonicalUrl),
		FeaturedMediaID:  stringValue(revision.FeaturedMediaID),
		SocialMediaID:    stringValue(revision.SocialMediaID),
		RobotsIndex:      robotsFormValue(revision.SeoRobotsIndex),
		RobotsFollow:     robotsFormValue(revision.SeoRobotsFollow),
		SchemaMode:       revision.SchemaMode,
		SiteURL:          settings.SiteUrl,
		PublicPath:       h.entryPublicPath(r, entry.ID),
		EntryID:          entry.ID,
		DocumentJSON:     revision.DocumentJson,
		Dirty:            "Saved",
		Status:           status,
		PublicURL:        publicURL,
		ShowExcerpt:      true,
		ShowSEO:          true,
		ShowFeatured:     true,
		ContentTypeID:    postContentType,
		LayoutTemplateID: layoutID,
		LayoutTemplates:  h.loadLayoutTemplateOptions(r.Context(), postContentType),
	}, "posts")
}

func (h *Handler) savePost(w http.ResponseWriter, r *http.Request) {
	h.updateEntry(w, r, postContentType, "posts", "/admin/posts", false)
}

func (h *Handler) publishPost(w http.ResponseWriter, r *http.Request) {
	h.updateEntry(w, r, postContentType, "posts", "/admin/posts", true)
}
