package admin

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/contenttypes"
)

type contentTypesData struct {
	Mode                string
	Rows                []contentTypeRow
	Form                contentTypeForm
	Fields              []content.FieldDefinition
	Error, CSRFToken    string
	FieldRemovalWarning string
	PendingRemoveKey    string
	FieldRemovalCount   int
}
type contentTypeRow struct {
	ID, Label, ItemLabel, BasePath string
	Builtin, Single, Archive      bool
	HasContent                    bool
}
type contentTypeForm struct {
	ID, Name, PluralName, BasePath string // Name=ItemLabel, PluralName=Label (for backward compat)
	Single, Archive, Hierarchical bool
	HasContent, Excerpt, Featured, SEO bool
	Preset string // structured | pages (only on create)
}

func (h *Handler) requireManageSite(w http.ResponseWriter, r *http.Request) bool {
	user, err := h.currentUser(r)
	if err != nil || !authz.Allows(user.Role, authz.ManageSite) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}
func (h *Handler) listContentTypes(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	defs, err := content.NewCatalog(h.queries).ListDefinitions(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	rows := make([]contentTypeRow, 0, len(defs))
	for _, d := range defs {
		rows = append(rows, contentTypeRow{
			ID: string(d.ID), Label: d.Label(), ItemLabel: d.ItemLabel(),
			BasePath: d.Routing.BasePath, Builtin: d.ID == content.ContentTypePage || d.ID == content.ContentTypePost,
			Single: d.Routing.Single, Archive: d.Routing.Archive, HasContent: d.Capabilities.HasContent,
		})
	}
	h.renderContentTypes(w, r, contentTypesData{Mode: "list", Rows: rows})
}
func (h *Handler) newContentType(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	// Default preset structured (simplest)
	h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: contentTypeForm{Preset: "structured", SEO: false}})
}
func (h *Handler) createContentType(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}
	f := contentTypeFormFromRequest(r)
	if f.ID == string(content.ContentTypePage) || f.ID == string(content.ContentTypePost) {
		h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: f, Error: "reserved content type key"})
		return
	}
	if f.ID == string(content.ContentTypePage) || f.ID == string(content.ContentTypePost) {
		f.BasePath = ""
	}
	// Apply preset defaults if present (simple create flow)
	preset := strings.TrimSpace(r.FormValue("preset"))
	if preset == "" {
		preset = f.Preset
	}
	if preset != "" {
		if preset == "pages" || preset == "content_with_pages" {
			f.Single = true
			f.HasContent = true
			if f.Featured == false && !r.Form.Has("featured") {
				f.Featured = true
			}
			if f.SEO == false && !r.Form.Has("seo") {
				f.SEO = true
			}
			if strings.TrimSpace(f.BasePath) == "" && f.Single {
				base := "/" + strings.ToLower(strings.ReplaceAll(f.ID, "_", "-"))
				f.BasePath = base
			}
		} else {
			// structured items preset
			f.Single = false
			f.Archive = false
			f.HasContent = false
			f.Excerpt = false
			f.Featured = false
			f.SEO = false
			f.BasePath = ""
		}
	} else {
		// Legacy path: no preset, respect explicit fields (including public alias already mapped to Single)
		// If HasContent not specified explicitly, infer from Single for backward compat
		if !r.Form.Has("has_content") && !r.Form.Has("content") {
			// Default HasContent true for backward compat when Single true, false otherwise?
			// Keep whatever was parsed (false) but for legacy products that expect content, we need true
			// Heuristic: if Single true and no explicit has_content, assume true (historical default)
			if f.Single {
				f.HasContent = true
			}
		}
	}
	// If single/archive both false, ignore base
	if !f.Single && !f.Archive {
		f.BasePath = ""
	}
	svc := contenttypes.New(h.database, h.queries)
	if err := svc.Create(r.Context(), contentTypeInput(f, nil)); err != nil {
		h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: f, Error: err.Error()})
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Content type created.")
	http.Redirect(w, r, "/admin/settings/content-types/"+f.ID, http.StatusSeeOther)
}

func (h *Handler) editContentType(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	d, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: formFromDefinition(d), Fields: d.Fields})
}
func (h *Handler) saveContentType(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}
	d, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f := contentTypeFormFromRequest(r)
	f.ID = r.PathValue("id")
	// Parse new fields: single, has_content, etc. contentTypeFormFromRequest already does
	fields := append([]content.FieldDefinition(nil), d.Fields...)
	for i := range fields {
		if r.FormValue("field_present_"+fields[i].Key) != "" {
			if label := strings.TrimSpace(r.FormValue("field_label_" + fields[i].Key)); label != "" {
				fields[i].Label = label
			}
			fields[i].Required = r.FormValue("field_required_"+fields[i].Key) != ""
			fields[i].HelpText = strings.TrimSpace(r.FormValue("field_help_" + fields[i].Key))
			fields[i].UI.Placeholder = strings.TrimSpace(r.FormValue("field_placeholder_" + fields[i].Key))
		}
	}
	target := r.FormValue("field_key_action")
	if v := r.FormValue("remove_field"); v != "" && v != "1" {
		target = v
	}
	if v := r.FormValue("move_up"); v != "" && v != "1" {
		target = v
	}
	if v := r.FormValue("move_down"); v != "" && v != "1" {
		target = v
	}
	if r.FormValue("remove_field") != "" {
		usage := countFieldUsage(r.Context(), h.database, target)
		if usage > 0 && r.FormValue("confirm_remove") == "" {
			warning := fmt.Sprintf("This field is used in %d places. Existing revisions keep their stored values, but dynamic blocks referring to fields.%s will stop resolving in future contexts.", usage, target)
			h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: f, Fields: fields, Error: warning, FieldRemovalWarning: warning, PendingRemoveKey: target, FieldRemovalCount: usage})
			return
		}
		for i, fld := range fields {
			if fld.Key == target {
				fields = append(fields[:i], fields[i+1:]...)
				break
			}
		}
	}
	if r.FormValue("move_up") != "" {
		for i, fld := range fields {
			if fld.Key == target && i > 0 {
				fields[i-1], fields[i] = fields[i], fields[i-1]
				break
			}
		}
	}
	if r.FormValue("move_down") != "" {
		for i, fld := range fields {
			if fld.Key == target && i+1 < len(fields) {
				fields[i+1], fields[i] = fields[i], fields[i+1]
				break
			}
		}
	}
	if r.FormValue("add_field") != "" {
		field, err := fieldDefinitionFromRequest(r)
		if err != nil {
			h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: f, Fields: fields, Error: err.Error()})
			return
		}
		fields = append(fields, field)
	}
	// Preserve capabilities during field manipulation (add/remove/move) where checkboxes are not re-sent
	isFieldOp := r.FormValue("add_field") != "" || r.FormValue("remove_field") != "" || r.FormValue("move_up") != "" || r.FormValue("move_down") != ""
	if isFieldOp {
		if !r.Form.Has("has_content") && !r.Form.Has("content") {
			f.HasContent = d.Capabilities.HasContent
		}
		if !r.Form.Has("single") && !r.Form.Has("public") && !r.Form.Has("has_single") {
			f.Single = d.Routing.Single
		}
		if !r.Form.Has("archive") {
			f.Archive = d.Routing.Archive
		}
		if !r.Form.Has("hierarchical") {
			f.Hierarchical = d.Capabilities.Hierarchical
		}
		if !r.Form.Has("excerpt") {
			f.Excerpt = d.Capabilities.HasExcerpt
		}
		if !r.Form.Has("featured") {
			f.Featured = d.Capabilities.HasFeatured
		}
		if !r.Form.Has("seo") {
			f.SEO = d.Capabilities.HasSEO
		}
		if !r.Form.Has("base_path") {
			f.BasePath = d.Routing.BasePath
		}
	}
	// For built-ins, ignore routing changes (core-owned)
	isBuiltin := d.ID == content.ContentTypePage || d.ID == content.ContentTypePost
	if isBuiltin {
		f.Single = d.Routing.Single
		f.Archive = d.Routing.Archive
		f.BasePath = d.Routing.BasePath
		f.Hierarchical = d.Capabilities.Hierarchical
		f.HasContent = d.Capabilities.HasContent
	}
	// If both single and archive off, clear base
	if !f.Single && !f.Archive {
		f.BasePath = ""
	}
	svc := contenttypes.New(h.database, h.queries)
	if err := svc.Update(r.Context(), string(d.ID), contentTypeInput(f, fields)); err != nil {
		h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: f, Fields: fields, Error: err.Error()})
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Content type saved.")
	http.Redirect(w, r, "/admin/settings/content-types/"+f.ID, http.StatusSeeOther)
}
func (h *Handler) deleteContentType(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}
	id := r.PathValue("id")
	tx, err := h.database.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	def, err := content.NewCatalog(qtx).GetDefinition(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if def.Routing.BasePath != "" {
		if route, err := qtx.GetArchiveRouteByContentType(r.Context(), sql.NullString{String: id, Valid: true}); err == nil {
			_ = qtx.DeleteRoute(r.Context(), route.ID)
		}
	}
	if err := content.NewCatalog(qtx).DeleteContentType(r.Context(), id); err != nil {
		h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: formFromDefinition(def), Fields: def.Fields, Error: err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Content type deleted.")
	http.Redirect(w, r, "/admin/settings/content-types", http.StatusSeeOther)
}

func (h *Handler) renderContentTypes(w http.ResponseWriter, r *http.Request, data contentTypesData) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	data.CSRFToken = token
	layout := h.layoutDataWithFlash(w, r, "Content Types")
	layout.Content = data
	if err := h.contentTypesTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render content types: %v", err)
	}
}
func contentTypeFormFromRequest(r *http.Request) contentTypeForm {
	_ = r.ParseForm()
	// Support legacy "public" alias for Single
	single := r.FormValue("single") != "" || r.FormValue("has_single") != "" || r.FormValue("routing_single") != "" || r.FormValue("public") != ""
	return contentTypeForm{
		ID: strings.TrimSpace(r.FormValue("id")),
		Name: strings.TrimSpace(r.FormValue("name")),
		PluralName: strings.TrimSpace(r.FormValue("plural_name")),
		BasePath: strings.TrimSpace(r.FormValue("base_path")),
		Single: single,
		Archive: r.FormValue("archive") != "",
		Hierarchical: r.FormValue("hierarchical") != "",
		HasContent: r.FormValue("has_content") != "" || r.FormValue("content") != "",
		Excerpt: r.FormValue("excerpt") != "",
		Featured: r.FormValue("featured") != "",
		SEO: r.FormValue("seo") != "",
		Preset: strings.TrimSpace(r.FormValue("preset")),
	}
}
func formFromDefinition(d content.ContentTypeDefinition) contentTypeForm {
	return contentTypeForm{
		ID: string(d.ID), Name: d.Name, PluralName: d.PluralName,
		BasePath: d.Routing.BasePath, Single: d.Routing.Single, Hierarchical: d.Capabilities.Hierarchical,
		Archive: d.Routing.Archive, HasContent: d.Capabilities.HasContent,
		Excerpt: d.Capabilities.HasExcerpt, Featured: d.Capabilities.HasFeatured, SEO: d.Capabilities.HasSEO,
	}
}
func contentTypeInput(f contentTypeForm, fields []content.FieldDefinition) content.ContentTypeInput {
	// Map new fields to config
	return content.ContentTypeInput{
		ID: content.ContentTypeID(f.ID), Name: f.Name, PluralName: f.PluralName,
		Hierarchical: f.Hierarchical,
		Public: f.Single, // sync for backward compat
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Fields: fields,
			Features: content.ContentTypeFeatures{Content: f.HasContent, Excerpt: f.Excerpt, FeaturedMedia: f.Featured, SEO: f.SEO},
			Routing: content.ContentTypeRouting{Single: f.Single, BasePath: f.BasePath, Archive: f.Archive},
		},
	}
}
func fieldDefinitionFromRequest(r *http.Request) (content.FieldDefinition, error) {
	typ := content.FieldType(r.FormValue("field_type"))
	f := content.FieldDefinition{Key: strings.TrimSpace(r.FormValue("field_key")), Label: strings.TrimSpace(r.FormValue("field_label")), Type: typ, Required: r.FormValue("field_required") != "", HelpText: strings.TrimSpace(r.FormValue("field_help")), UI: content.FieldUI{Placeholder: strings.TrimSpace(r.FormValue("field_placeholder"))}}
	if typ == content.FieldSelect {
		for _, v := range strings.Split(r.FormValue("field_options"), ",") {
			if v = strings.TrimSpace(v); v != "" {
				f.Validation.Options = append(f.Validation.Options, v)
			}
		}
	}
	if raw := strings.TrimSpace(r.FormValue("field_min")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return content.FieldDefinition{}, fmt.Errorf("minimum must be a number")
		}
		f.Validation.Min = &v
	}
	if raw := strings.TrimSpace(r.FormValue("field_max")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return content.FieldDefinition{}, fmt.Errorf("maximum must be a number")
		}
		f.Validation.Max = &v
	}
	if raw := strings.TrimSpace(r.FormValue("field_default")); raw != "" {
		switch typ {
		case content.FieldNumber:
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return content.FieldDefinition{}, fmt.Errorf("default must be a number")
			}
			f.Default = v
		case content.FieldBoolean:
			v, err := strconv.ParseBool(raw)
			if err != nil {
				return content.FieldDefinition{}, fmt.Errorf("default must be true or false")
			}
			f.Default = v
		default:
			f.Default = raw
		}
	}
	if f.Key == "" || f.Label == "" {
		return content.FieldDefinition{}, fmt.Errorf("field label and key are required")
	}
	return f, content.ValidateFieldDefinition(f)
}

func countFieldUsage(ctx context.Context, database *sql.DB, fieldKey string) int {
	if database == nil || strings.TrimSpace(fieldKey) == "" {
		return 0
	}
	needle := `"fields.` + fieldKey + `"`
	count := 0
	rows, err := database.QueryContext(ctx, `SELECT document_json FROM entry_revisions WHERE document_json LIKE ?`, "%"+needle+"%")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var doc string
			if err := rows.Scan(&doc); err == nil && strings.Contains(doc, needle) {
				count++
				if count > 1000 {
					break
				}
			}
		}
	}
	rows2, err := database.QueryContext(ctx, `SELECT document_json FROM layout_template_revisions WHERE document_json LIKE ?`, "%"+needle+"%")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var doc string
			if err := rows2.Scan(&doc); err == nil && strings.Contains(doc, needle) {
				count++
				if count > 1000 {
					break
				}
			}
		}
	}
	return count
}
