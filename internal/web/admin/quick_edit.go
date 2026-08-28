package admin

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/slug"
)

func (h *Handler) quickEditPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.quickEditForm(w, r, pageContentType)
		return
	}
	h.quickEditSave(w, r, pageContentType, "pages")
}

func (h *Handler) quickEditPost(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.quickEditForm(w, r, postContentType)
		return
	}
	h.quickEditSave(w, r, postContentType, "posts")
}

func (h *Handler) quickEditForm(w http.ResponseWriter, r *http.Request, contentType string) {
	if !h.validCSRF(r) && r.Method == http.MethodGet {
		// For GET, we don't require CSRF but we will generate token for form
	}
	id := r.PathValue("id")
	entry, err := h.queries.GetEntry(r.Context(), id)
	if err != nil || entry.ContentTypeID != contentType {
		http.NotFound(w, r)
		return
	}
	rev, err := h.queries.GetLatestEntryRevision(r.Context(), id)
	if err != nil {
		http.Error(w, "Revision not found", http.StatusNotFound)
		return
	}
	token, _ := h.csrfToken(w, r)
	// Build options
	var parentOpts string
	var hierarchyWarning string
	if contentType == pageContentType {
		opts, warning := h.hierarchyParentOptions(r.Context(), contentType, entry.ID, rev.ParentEntryID.String)
		hierarchyWarning = warning
		parentOpts = `<option value="">No parent</option>`
		for _, o := range opts {
			sel := ""
			if rev.ParentEntryID.Valid && rev.ParentEntryID.String == o.ID {
				sel = " selected"
			}
			parentOpts += fmt.Sprintf(`<option value="%s"%s>%s</option>`, html.EscapeString(o.ID), sel, html.EscapeString(o.Label))
		}
		_ = hierarchyWarning
	}
	// Layout templates
	layoutOpts := ""
	selectedLayout := ""
	if rev.LayoutTemplateID.Valid {
		selectedLayout = rev.LayoutTemplateID.String
	}
	templates := h.loadLayoutTemplateOptions(r.Context(), contentType)
	// Determine default template for "Use default" label
	defaultName := ""
	hasDefault := false
	if ct, err := h.queries.GetContentType(r.Context(), contentType); err == nil && ct.DefaultLayoutTemplateID.Valid {
		hasDefault = true
		if tmpl, err := h.queries.GetLayoutTemplate(r.Context(), ct.DefaultLayoutTemplateID.String); err == nil {
			defaultName = tmpl.Name
		}
	}
	defaultOption := ""
	if hasDefault {
		sel := ""
		if selectedLayout == "" {
			sel = " selected"
		}
		label := "Use default"
		if defaultName != "" {
			label = "Use default — " + defaultName
		}
		defaultOption = fmt.Sprintf(`<option value=""%s>%s</option>`, sel, html.EscapeString(label))
	} else {
		sel := ""
		if selectedLayout == "" {
			sel = " selected"
		}
		defaultOption = fmt.Sprintf(`<option value=""%s>No template — direct content</option>`, sel)
	}
	for _, t := range templates {
		sel := ""
		if t.ID == selectedLayout {
			sel = " selected"
		}
		layoutOpts += fmt.Sprintf(`<option value="%s"%s>%s</option>`, html.EscapeString(t.ID), sel, html.EscapeString(t.Name))
	}
	// Taxonomy for posts
	taxHtml := ""
	if contentType == postContentType {
		// Load categories and tags panels for quick edit
		if taxRows, err := h.queries.ListTaxonomiesByContentType(r.Context(), contentType); err == nil {
			for _, tax := range taxRows {
				terms, _ := h.queries.ListTermsByTaxonomy(r.Context(), tax.ID)
				assigned := map[string]bool{}
				if termRows, err := h.queries.ListTermsForRevision(r.Context(), rev.ID); err == nil {
					for _, tr := range termRows {
						if tr.TaxonomyID == tax.ID {
							assigned[tr.ID] = true
						}
					}
				}
				if tax.Hierarchical != 0 {
					taxHtml += fmt.Sprintf(`<div class="form-group"><label class="form-label">%s</label><div style="max-height:120px; overflow:auto; border:1px solid var(--ui-border); padding:8px; border-radius:var(--ui-radius-sm);">`, html.EscapeString(tax.PluralName))
					for _, term := range terms {
						checked := ""
						if assigned[term.ID] {
							checked = " checked"
						}
						taxHtml += fmt.Sprintf(`<label class="check-row"><input type="checkbox" name="taxonomy_%s" value="%s"%s> %s <small class="muted">(%s)</small></label>`, html.EscapeString(tax.ID), html.EscapeString(term.ID), checked, html.EscapeString(term.Name), html.EscapeString(term.Slug))
					}
					if len(terms) == 0 {
						taxHtml += `<p class="muted">No ` + html.EscapeString(tax.PluralName) + ` yet.</p>`
					}
					taxHtml += `</div></div>`
				} else {
					// Flat tags comma separated
					var names []string
					for _, term := range terms {
						if assigned[term.ID] {
							names = append(names, term.Name)
						}
					}
					val := html.EscapeString(strings.Join(names, ", "))
					taxHtml += fmt.Sprintf(`<div class="form-group"><label class="form-label" for="qe-tax-%s">%s</label><input class="form-control" type="text" id="qe-tax-%s" name="taxonomy_%s" value="%s" placeholder="comma separated"><p class="form-help">Separate with commas.</p></div>`, html.EscapeString(tax.ID), html.EscapeString(tax.PluralName), html.EscapeString(tax.ID), html.EscapeString(tax.ID), val)
				}
			}
		}
	}
	statusLabel, _ := h.entryEditorStatus(r, entry)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Build form
	titleEsc := html.EscapeString(rev.Title)
	slugEsc := html.EscapeString(rev.Slug)
	excerptEsc := html.EscapeString(stringValue(rev.Excerpt))
	orderVal := fmt.Sprintf("%d", rev.MenuOrder)
	// Determine excerpt visibility
	excerptField := ""
	definition := content.DefinitionFor(contentType)
	if cat, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), contentType); err == nil {
		definition = cat
	}
	if definition.Capabilities.HasExcerpt {
		excerptField = fmt.Sprintf(`<div class="form-group"><label class="form-label" for="qe-excerpt">Excerpt</label><textarea class="form-control" id="qe-excerpt" name="excerpt" rows="2">%s</textarea></div>`, excerptEsc)
	}
	// Build HTML
	htmlOut := fmt.Sprintf(`
<form method="post" action="/admin/%s/%s/quick-edit" class="quick-edit-form">
  <input type="hidden" name="csrf_token" value="%s">
  <div style="display:grid; gap:12px; max-width:640px;">
    <div style="display:flex; gap:12px; align-items:center; flex-wrap:wrap;">
      <span class="status-badge status-badge--draft">%s</span>
      <small class="muted">Quick Edit</small>
    </div>
    <div class="form-group"><label class="form-label" for="qe-title">Title</label><input class="form-control" type="text" id="qe-title" name="title" value="%s" required></div>
    <div class="form-group"><label class="form-label" for="qe-slug">Slug</label><input class="form-control" type="text" id="qe-slug" name="slug" value="%s" pattern="[a-z0-9]+(-[a-z0-9]+)*" required><p class="form-help">Lowercase letters, numbers, hyphens.</p></div>
    %s
`, mapContentTypeToPluralPath(contentType), html.EscapeString(id), html.EscapeString(token), html.EscapeString(statusLabel), titleEsc, slugEsc, excerptField)
	if contentType == pageContentType {
		htmlOut += fmt.Sprintf(`
    <div class="form-group"><label class="form-label" for="qe-parent">Parent</label><select class="form-control" id="qe-parent" name="parent_entry_id">%s</select></div>
    <div class="form-group"><label class="form-label" for="qe-order">Order</label><input class="form-control" type="number" id="qe-order" name="menu_order" min="0" step="1" value="%s"></div>
    <div class="form-group"><label class="form-label" for="qe-template">Template</label><select class="form-control" id="qe-template" name="layout_template_id">%s%s</select></div>
`, parentOpts, html.EscapeString(orderVal), defaultOption, layoutOpts)
	}
	if taxHtml != "" {
		htmlOut += taxHtml
	}
	htmlOut += `
    <div class="admin-page-actions">
      <button type="submit" name="quick_action" value="save" class="button">Save draft</button>
      <button type="submit" name="quick_action" value="publish" class="button button-primary">Publish changes</button>
      <button type="button" class="button" data-quick-cancel>Cancel</button>
    </div>
  </div>
</form>
`
	// Wrap in td? The fetch expects to inject into <td colspan>
	w.Write([]byte(htmlOut))
}

func mapContentTypeToPluralPath(ct string) string {
	if ct == "page" {
		return "pages"
	}
	if ct == "post" {
		return "posts"
	}
	return "content/" + ct
}

func (h *Handler) quickEditSave(w http.ResponseWriter, r *http.Request, contentType, activeMenu string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	slugVal := strings.TrimSpace(r.FormValue("slug"))
	excerpt := strings.TrimSpace(r.FormValue("excerpt"))
	parentID := strings.TrimSpace(r.FormValue("parent_entry_id"))
	layoutID := strings.TrimSpace(r.FormValue("layout_template_id"))
	orderStr := strings.TrimSpace(r.FormValue("menu_order"))
	publish := r.FormValue("quick_action") == "publish"
	// Also support publish via formaction? but quick_action is enough
	if r.FormValue("publish") != "" {
		publish = true
	}
	// Load existing entry and revision to preserve other fields
	entry, rev, err := h.entryAndLatestRevision(r.Context(), id, contentType)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Preserve existing values for fields not in quick edit
	docJSON := rev.DocumentJson
	fieldsJson := rev.FieldsJson
	fieldValues := fieldValues(rev.FieldsJson)
	// Layout template validation: if empty means inherit default (allowed)
	// Taxonomy handling
	taxonomyValues := map[string][]string{}
	if contentType == postContentType {
		// For quick edit posts, parse taxonomy values from form
		taxonomyValues = postedTaxonomyValues(r)
		// If no taxonomy values posted (checkboxes unchecked), ensure we still get empty correctly
		// postedTaxonomyValues already handles
	} else {
		// For pages, keep existing taxonomy assignments
		if termRows, err := h.queries.ListTermsForRevision(r.Context(), rev.ID); err == nil {
			for _, tr := range termRows {
				taxonomyValues[tr.TaxonomyID] = append(taxonomyValues[tr.TaxonomyID], tr.ID)
			}
		}
	}
	// Parent/order handling
	var menuOrder int64
	if orderStr != "" {
		if v, err := strconv.ParseInt(orderStr, 10, 64); err == nil && v >= 0 {
			menuOrder = v
		} else {
			h.setFlash(w, "Order must be a non-negative integer")
			http.Redirect(w, r, "/admin/"+activeMenu, http.StatusSeeOther)
			return
		}
	} else {
		menuOrder = rev.MenuOrder
	}
	// Build entryInput preserving other fields from latest
	definition, _ := content.NewCatalog(h.queries).GetDefinition(r.Context(), contentType)
	if definition.ID == "" {
		definition = content.DefinitionFor(contentType)
	}
	input := entryInput{
		title:            title,
		slug:             slugVal,
		excerpt:          excerpt,
		documentJSON:     docJSON,
		layoutTemplateID: layoutID,
		parentEntryID:    parentID,
		menuOrder:        menuOrder,
		fields:           fieldValues,
		visibility:       rev.Visibility,
		sticky:           rev.Sticky != 0,
		reviewState:      rev.ReviewState,
		commentsEnabled:  rev.CommentsEnabled != 0,
		taxonomyValues:   taxonomyValues,
	}
	// Preserve SEO etc from existing
	if rev.SeoTitle.Valid {
		input.seoTitle = rev.SeoTitle.String
	}
	if rev.SeoDescription.Valid {
		input.seoDescription = rev.SeoDescription.String
	}
	if rev.CanonicalUrl.Valid {
		input.canonicalURL = rev.CanonicalUrl.String
	}
	if rev.FeaturedMediaID.Valid {
		input.featuredMediaID = rev.FeaturedMediaID.String
	}
	if rev.SocialMediaID.Valid {
		input.socialMediaID = rev.SocialMediaID.String
	}
	if rev.SeoRobotsIndex.Valid {
		b := rev.SeoRobotsIndex.Int64 == 1
		input.robotsIndex = &b
	}
	if rev.SeoRobotsFollow.Valid {
		b := rev.SeoRobotsFollow.Int64 == 1
		input.robotsFollow = &b
	}
	input.schemaMode = rev.SchemaMode
	// Validate title/slug required
	if title == "" {
		h.setFlash(w, "Title is required")
		http.Redirect(w, r, "/admin/"+activeMenu, http.StatusSeeOther)
		return
	}
	if slugVal == "" {
		slugVal = slug.Slugify(title)
		if slugVal == "" {
			slugVal = "entry"
		}
		input.slug = slugVal
	}
	// Preserve unpublished changes semantics via writeEntry
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// Need to handle taxonomyValues for flat tags: ensure they are parsed correctly
	// postedTaxonomyValues expects r.Form with taxonomy_{id} keys; for quick edit posts, we already parsed, but need to ensure tags comma handling creates terms
	// For hierarchical, the handler will create terms if needed? quickEditSave uses writeEntry which will call taxonomyTermIDsForInput which handles tag creation
	// So we should let writeEntry handle it via taxonomyValues map

	// Special handling: for posts quick edit with categories/tags, the form may have not included some taxonomies; preserve existing for those not in form
	// Already handled via taxonomyValues map

	_ = entry
	_ = fieldsJson

	if err := h.writeEntry(r.Context(), contentType, user.ID, id, input, false, publish); err != nil {
		h.setFlash(w, entryWriteError(err))
		http.Redirect(w, r, "/admin/"+activeMenu, http.StatusSeeOther)
		return
	}
	if publish && h.runtime != nil {
		h.runtime.InvalidateEntry(id, contentType)
		h.runtime.InvalidateContent()
	}
	if publish {
		h.setFlash(w, contentTypeTitle(contentType)+" published via Quick Edit.")
	} else {
		h.setFlash(w, contentTypeTitle(contentType)+" draft saved via Quick Edit.")
	}
	// Handle Datastar?
	if isDatastarRequest(r) {
		// For Datastar, return toast and redirect? Simplified: return SSE
		writeSSE(w, toastEvent("success", "Quick Edit saved."))
		return
	}
	http.Redirect(w, r, "/admin/"+activeMenu, http.StatusSeeOther)
}
