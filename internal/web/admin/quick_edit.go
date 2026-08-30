package admin

import (
	"bytes"
	"html/template"
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

func (h *Handler) quickEditCustom(w http.ResponseWriter, r *http.Request) {
	ct := r.PathValue("type")
	definition, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), ct)
	if err != nil || definition.ID == content.ContentTypePage || definition.ID == content.ContentTypePost {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		h.quickEditForm(w, r, ct)
		return
	}
	h.quickEditSave(w, r, ct, "content/"+ct)
}

type quickEditFieldVM struct {
	Key         string
	Label       string
	Type        string
	Required    bool
	Value       string
	Checked     bool
	Options     []string
	Placeholder string
	HelpText    string
	InputType   string
}

type quickEditVM struct {
	Action        string
	CSRFToken     string
	Status        string
	Title         string
	Slug          string
	Excerpt       string
	ShowExcerpt   bool
	IsPage        bool
	ParentOptions []struct {
		ID       string
		Label    string
		Selected bool
	}
	MenuOrder     string
	LayoutOptions []struct {
		ID   string
		Name string
	}
	SelectedLayout       string
	DefaultTemplateLabel string
	TaxonomyHTML         template.HTML
	CustomFields         []quickEditFieldVM
}

const quickEditTemplateHTML = `<form method="post" action="{{ .Action }}" class="quick-edit-form">
  <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
  <div class="admin-form-card" style="display:grid; gap:14px;">
    <div class="admin-page-actions">
      <span class="status-indicator status-indicator--muted">● {{ .Status }}</span>
      <small class="muted">Quick Edit</small>
    </div>
    <div class="form-group"><label class="form-label" for="qe-title">Title</label><input class="form-control" type="text" id="qe-title" name="title" value="{{ .Title }}" required></div>
    <div class="form-group"><label class="form-label" for="qe-slug">Slug</label><input class="form-control" type="text" id="qe-slug" name="slug" value="{{ .Slug }}" pattern="[a-z0-9]+(-[a-z0-9]+)*" required><p class="form-help">Lowercase letters, numbers, hyphens.</p></div>
    {{ if .ShowExcerpt }}
    <div class="form-group"><label class="form-label" for="qe-excerpt">Excerpt</label><textarea class="form-control" id="qe-excerpt" name="excerpt" rows="2">{{ .Excerpt }}</textarea></div>
    {{ end }}
    {{ if .IsPage }}
    <div class="form-group"><label class="form-label" for="qe-parent">Parent</label><select class="form-control" id="qe-parent" name="parent_entry_id"><option value="">No parent</option>{{ range .ParentOptions }}<option value="{{ .ID }}" {{ if .Selected }}selected{{ end }}>{{ .Label }}</option>{{ end }}</select></div>
    <div class="form-group"><label class="form-label" for="qe-order">Order</label><input class="form-control" type="number" id="qe-order" name="menu_order" min="0" step="1" value="{{ .MenuOrder }}"></div>
    <div class="form-group"><label class="form-label" for="qe-template">Template</label><select class="form-control" id="qe-template" name="layout_template_id"><option value="" {{ if eq .SelectedLayout "" }}selected{{ end }}>{{ .DefaultTemplateLabel }}</option>{{ range .LayoutOptions }}<option value="{{ .ID }}" {{ if eq $.SelectedLayout .ID }}selected{{ end }}>{{ .Name }}</option>{{ end }}</select></div>
    {{ end }}
    {{ range .CustomFields }}
    <div class="form-group">
      <label class="form-label" for="qe-field-{{ .Key }}">{{ .Label }}{{ if .Required }} *{{ end }}</label>
      {{ if eq .Type "textarea" }}<textarea class="form-control" id="qe-field-{{ .Key }}" name="field_{{ .Key }}" rows="3" {{ if .Required }}required{{ end }}>{{ .Value }}</textarea>
      {{ else if eq .Type "number" }}<input class="form-control" type="number" id="qe-field-{{ .Key }}" name="field_{{ .Key }}" value="{{ .Value }}" step="any" {{ if .Required }}required{{ end }}>
      {{ else if eq .Type "select" }}<select class="form-control" id="qe-field-{{ .Key }}" name="field_{{ .Key }}" {{ if .Required }}required{{ end }}><option value="">— Select —</option>{{ range .Options }}<option value="{{ . }}" {{ if eq $.Value . }}selected{{ end }}>{{ . }}</option>{{ end }}</select>
      {{ else if eq .Type "boolean" }}<label class="check-row"><input type="checkbox" name="field_{{ .Key }}" value="1" {{ if .Checked }}checked{{ end }}> Yes</label>
      {{ else if eq .Type "media" }}<input type="hidden" name="field_{{ .Key }}" value="{{ .Value }}"><p class="form-help">Media: {{ .Value }} — Edit full item to change media.</p>
      {{ else }}<input class="form-control" type="{{ .InputType }}" id="qe-field-{{ .Key }}" name="field_{{ .Key }}" value="{{ .Value }}" {{ if .Placeholder }}placeholder="{{ .Placeholder }}"{{ end }} {{ if .Required }}required{{ end }}>
      {{ end }}
      {{ if .HelpText }}<p class="form-help">{{ .HelpText }}</p>{{ end }}
    </div>
    {{ end }}
    {{ if .TaxonomyHTML }}<div>{{ .TaxonomyHTML }}</div>{{ end }}
    <div class="admin-page-actions">
      <button type="submit" name="quick_action" value="save" class="button">Save draft</button>
      <button type="submit" name="quick_action" value="publish" class="button button-primary">Publish changes</button>
      <button type="button" class="button" data-quick-cancel>Cancel</button>
    </div>
  </div>
</form>`

func (h *Handler) quickEditForm(w http.ResponseWriter, r *http.Request, contentType string) {
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

	definition := content.DefinitionFor(contentType)
	if cat, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), contentType); err == nil {
		definition = cat
	}

	statusLabel, _ := h.entryEditorStatus(r, entry)

	var parentOpts []struct {
		ID       string
		Label    string
		Selected bool
	}
	if contentType == pageContentType {
		opts, _ := h.hierarchyParentOptions(r.Context(), contentType, entry.ID, rev.ParentEntryID.String)
		for _, o := range opts {
			selected := rev.ParentEntryID.Valid && rev.ParentEntryID.String == o.ID
			parentOpts = append(parentOpts, struct {
				ID       string
				Label    string
				Selected bool
			}{ID: o.ID, Label: o.Label, Selected: selected})
		}
	}
	layoutOptsRaw := h.loadLayoutTemplateOptions(r.Context(), contentType)
	var layoutOpts []struct {
		ID   string
		Name string
	}
	for _, o := range layoutOptsRaw {
		layoutOpts = append(layoutOpts, struct {
			ID   string
			Name string
		}{ID: o.ID, Name: o.Name})
	}
	selectedLayout := ""
	if rev.LayoutTemplateID.Valid {
		selectedLayout = rev.LayoutTemplateID.String
	}
	defaultLabel := "Use default"
	hasDefault := false
	defaultName := ""
	if ct, err := h.queries.GetContentType(r.Context(), contentType); err == nil && ct.DefaultLayoutTemplateID.Valid {
		hasDefault = true
		if tmpl, err := h.queries.GetLayoutTemplate(r.Context(), ct.DefaultLayoutTemplateID.String); err == nil {
			defaultName = tmpl.Name
		}
	}
	if hasDefault {
		if defaultName != "" {
			defaultLabel = "Use default — " + defaultName
		}
	} else {
		defaultLabel = "No template — direct content"
	}

	taxHtml := ""
	if contentType == postContentType {
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
					taxHtml += `<div class="form-group"><label class="form-label">` + template.HTMLEscapeString(tax.PluralName) + `</label><div style="max-height:120px; overflow:auto; border:1px solid var(--ui-border); padding:8px; border-radius:var(--ui-radius-sm);">`
					for _, term := range terms {
						checked := ""
						if assigned[term.ID] {
							checked = " checked"
						}
						taxHtml += `<label class="check-row"><input type="checkbox" name="taxonomy_` + template.HTMLEscapeString(tax.ID) + `" value="` + template.HTMLEscapeString(term.ID) + `"` + checked + `> ` + template.HTMLEscapeString(term.Name) + ` <small class="muted">(` + template.HTMLEscapeString(term.Slug) + `)</small></label>`
					}
					if len(terms) == 0 {
						taxHtml += `<p class="muted">No ` + template.HTMLEscapeString(tax.PluralName) + ` yet.</p>`
					}
					taxHtml += `</div></div>`
				} else {
					var names []string
					for _, term := range terms {
						if assigned[term.ID] {
							names = append(names, term.Name)
						}
					}
					val := template.HTMLEscapeString(strings.Join(names, ", "))
					taxHtml += `<div class="form-group"><label class="form-label" for="qe-tax-` + template.HTMLEscapeString(tax.ID) + `">` + template.HTMLEscapeString(tax.PluralName) + `</label><input class="form-control" type="text" id="qe-tax-` + template.HTMLEscapeString(tax.ID) + `" name="taxonomy_` + template.HTMLEscapeString(tax.ID) + `" value="` + val + `" placeholder="comma separated"><p class="form-help">Separate with commas.</p></div>`
				}
			}
		}
	}

	existingFields := fieldValues(rev.FieldsJson)
	var customFields []quickEditFieldVM
	for _, f := range definition.Fields {
		vm := quickEditFieldVM{
			Key: f.Key, Label: f.Label, Type: string(f.Type), Required: f.Required, Placeholder: f.UI.Placeholder, HelpText: f.HelpText,
		}
		switch f.Type {
		case content.FieldText:
			vm.InputType = "text"
		case content.FieldNumber:
			vm.InputType = "number"
			vm.Type = "number"
		case content.FieldEmail:
			vm.InputType = "email"
		case content.FieldURL:
			vm.InputType = "url"
		case content.FieldSelect:
			vm.Type = "select"
			vm.Options = f.Validation.Options
		case content.FieldBoolean:
			vm.Type = "boolean"
		case content.FieldMedia:
			vm.Type = "media"
		case content.FieldTextarea:
			vm.Type = "textarea"
		default:
			vm.InputType = "text"
		}
		raw, ok := existingFields[f.Key]
		if !ok && f.Default != nil {
			raw = f.Default
		}
		if f.Type == content.FieldBoolean {
			if b, ok := raw.(bool); ok {
				vm.Checked = b
			}
		} else if f.Type == content.FieldMedia {
			if s, ok := raw.(string); ok {
				vm.Value = s
			} else if raw != nil {
				vm.Value = fieldValueString(raw)
			}
		} else {
			if raw != nil {
				vm.Value = fieldValueString(raw)
			}
		}
		// Only include supported quick-edit types; media preserved with note
		supported := map[content.FieldType]bool{
			content.FieldText: true, content.FieldTextarea: true, content.FieldNumber: true,
			content.FieldEmail: true, content.FieldURL: true, content.FieldSelect: true,
			content.FieldBoolean: true, content.FieldMedia: true,
		}
		if supported[f.Type] {
			customFields = append(customFields, vm)
		}
	}

	vm := quickEditVM{
		Action:               "/admin/" + mapContentTypeToPluralPath(contentType) + "/" + id + "/quick-edit",
		CSRFToken:            token,
		Status:               statusLabel,
		Title:                rev.Title,
		Slug:                 rev.Slug,
		Excerpt:              stringValue(rev.Excerpt),
		ShowExcerpt:          definition.Capabilities.HasExcerpt,
		IsPage:               contentType == pageContentType,
		ParentOptions:        parentOpts,
		MenuOrder:            strconv.FormatInt(rev.MenuOrder, 10),
		LayoutOptions:        layoutOpts,
		SelectedLayout:       selectedLayout,
		DefaultTemplateLabel: defaultLabel,
		TaxonomyHTML:         template.HTML(taxHtml),
		CustomFields:         customFields,
	}

	tmpl := template.Must(template.New("quick_edit").Parse(quickEditTemplateHTML))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vm); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
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
	if r.FormValue("publish") != "" {
		publish = true
	}
	entry, rev, err := h.entryAndLatestRevision(r.Context(), id, contentType)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	existingFields := fieldValues(rev.FieldsJson)
	definition, _ := content.NewCatalog(h.queries).GetDefinition(r.Context(), contentType)
	if definition.ID == "" {
		definition = content.DefinitionFor(contentType)
	}

	taxonomyValues := map[string][]string{}
	if contentType == postContentType {
		taxonomyValues = postedTaxonomyValues(r)
	} else {
		if termRows, err := h.queries.ListTermsForRevision(r.Context(), rev.ID); err == nil {
			for _, tr := range termRows {
				taxonomyValues[tr.TaxonomyID] = append(taxonomyValues[tr.TaxonomyID], tr.ID)
			}
		}
	}

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

	rawFields := make(map[string]any)
	for k, v := range existingFields {
		rawFields[k] = v
	}
	for _, field := range definition.Fields {
		formKey := "field_" + field.Key
		switch field.Type {
		case content.FieldBoolean:
			if _, ok := r.Form[formKey]; ok {
				rawFields[field.Key] = r.FormValue(formKey) == "1" || r.FormValue(formKey) == "on" || r.FormValue(formKey) == "true"
			} else {
				// Check if field was rendered; if so unchecked means false
				rawFields[field.Key] = false
			}
		case content.FieldMedia:
			if v, ok := r.Form[formKey]; ok && len(v) > 0 {
				rawFields[field.Key] = v[0]
			}
		case content.FieldNumber:
			if v, ok := r.Form[formKey]; ok && len(v) > 0 {
				trimmed := strings.TrimSpace(v[0])
				if trimmed == "" {
					if field.Required {
						rawFields[field.Key] = ""
					} else {
						delete(rawFields, field.Key)
					}
				} else {
					rawFields[field.Key] = trimmed
				}
			}
		case content.FieldText, content.FieldTextarea, content.FieldEmail, content.FieldURL, content.FieldSelect:
			if v, ok := r.Form[formKey]; ok {
				val := strings.TrimSpace(v[0])
				if val == "" && !field.Required {
					delete(rawFields, field.Key)
				} else {
					rawFields[field.Key] = val
				}
			}
		default:
		}
	}
	validatedFields := existingFields
	if len(definition.Fields) > 0 {
		normalized, err := content.ValidateFields(definition, rawFields, content.FieldValidationOptions{MediaExists: func(id string) bool {
			if id == "" {
				return false
			}
			if h.media != nil {
				if _, medErr := h.media.Get(r.Context(), id); medErr == nil {
					return true
				}
				return true
			}
			return true
		}})
		if err != nil {
			h.setFlash(w, err.Error())
			http.Redirect(w, r, "/admin/"+activeMenu, http.StatusSeeOther)
			return
		}
		validatedFields = normalized
	}

	input := entryInput{
		title:            title,
		slug:             slugVal,
		excerpt:          excerpt,
		documentJSON:     rev.DocumentJson,
		layoutTemplateID: layoutID,
		parentEntryID:    parentID,
		menuOrder:        menuOrder,
		fields:           validatedFields,
		visibility:       rev.Visibility,
		sticky:           rev.Sticky != 0,
		reviewState:      rev.ReviewState,
		commentsEnabled:  rev.CommentsEnabled != 0,
		taxonomyValues:   taxonomyValues,
	}
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
	_ = entry
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
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
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Quick Edit saved."))
		return
	}
	http.Redirect(w, r, "/admin/"+activeMenu, http.StatusSeeOther)
}
