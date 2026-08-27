package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
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
	ID, Name, PluralName, BasePath string
	Builtin, Public, Archive       bool
}
type contentTypeForm struct {
	ID, Name, PluralName, BasePath                        string
	Public, Hierarchical, Archive, Excerpt, Featured, SEO bool
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
		rows = append(rows, contentTypeRow{ID: string(d.ID), Name: d.Name, PluralName: d.PluralName, BasePath: d.Routing.BasePath, Builtin: d.ID == content.ContentTypePage || d.ID == content.ContentTypePost, Public: d.Capabilities.Public, Archive: d.IsArchived()})
	}
	h.renderContentTypes(w, r, contentTypesData{Mode: "list", Rows: rows})
}
func (h *Handler) newContentType(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: contentTypeForm{Public: true, SEO: true}})
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
	// Built-ins are not creatable via UI, but guard anyway.
	if f.ID == string(content.ContentTypePage) || f.ID == string(content.ContentTypePost) {
		h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: f, Error: "reserved content type key"})
		return
	}
	// Page/Post routing is core-owned; ignore generic URL base for them (not applicable on create).
	if f.ID == string(content.ContentTypePage) || f.ID == string(content.ContentTypePost) {
		f.BasePath = ""
	}
	tx, err := h.database.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	if err := content.NewCatalog(qtx).CreateContentType(r.Context(), contentTypeInput(f, nil)); err != nil {
		h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: f, Error: err.Error()})
		return
	}
	// Only create public routes for public types; private types remain unroutable until enabled.
	if f.Public && f.BasePath != "" {
		if err := routing.SyncContentTypeRouting(r.Context(), qtx, f.ID, f.BasePath, f.BasePath, f.Archive, time.Now().Unix()); err != nil {
			h.renderContentTypes(w, r, contentTypesData{Mode: "new", Form: f, Error: err.Error()})
			return
		}
	} else if f.Public && f.Archive && f.BasePath != "" {
		// Ensure archive route for public archived types even if no entries yet (handled above when base set).
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", 500)
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
	// Core-owned routing for Page/Post: ignore generic base/archive changes.
	isBuiltin := d.ID == content.ContentTypePage || d.ID == content.ContentTypePost
	prevPublic := d.Capabilities.Public
	newPublic := f.Public
	if isBuiltin {
		newPublic = prevPublic
		f.Public = prevPublic
		f.BasePath = ""
		f.Archive = d.IsArchived()
		f.Hierarchical = d.Capabilities.Hierarchical
	}
	prevBase := d.Routing.BasePath
	newBase := f.BasePath
	prevArchive := d.IsArchived()
	newArchive := f.Archive

	tx, err := h.database.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	if err := content.NewCatalog(qtx).UpdateContentType(r.Context(), contentTypeInput(f, fields)); err != nil {
		h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: f, Fields: fields, Error: err.Error()})
		return
	}
	now := time.Now().Unix()
	// Public → private: delete all public entry and archive routes transactionally.
	if prevPublic && !newPublic {
		if route, err := qtx.GetArchiveRouteByContentType(r.Context(), sql.NullString{String: string(d.ID), Valid: true}); err == nil {
			_ = qtx.DeleteRoute(r.Context(), route.ID)
		}
		// Delete canonical entry routes in bounded batches via hierarchy listing (covers all published regardless of route).
		hRows, _ := qtx.ListPublishedHierarchyForContentType(r.Context(), string(d.ID))
		for _, row := range hRows {
			if rt, err := qtx.GetEntryRoute(r.Context(), sql.NullString{String: row.EntryID, Valid: true}); err == nil {
				_ = qtx.DeleteRoute(r.Context(), rt.ID)
			}
		}
		// Also cover any flat published entries that might not appear in hierarchy? Hierarchy already covers all published for this type.
	} else if !prevPublic && newPublic {
		// Private → public: recreate archive and entry routes for already-published entries.
		if newArchive && newBase != "" {
			if _, err := qtx.GetArchiveRouteByContentType(r.Context(), sql.NullString{String: string(d.ID), Valid: true}); err != nil {
				b := make([]byte, 16)
				_, _ = rand.Read(b)
				id := base64.RawURLEncoding.EncodeToString(b)
				_ = qtx.CreateRoute(r.Context(), db.CreateRouteParams{ID: id, Path: newBase, RouteType: routing.RouteTypeArchive, ContentTypeID: sql.NullString{String: string(d.ID), Valid: true}, CreatedAt: now, UpdatedAt: now})
			}
		}
		hRows, _ := qtx.ListPublishedHierarchyForContentType(r.Context(), string(d.ID))
		// For hierarchical types, compute parent-aware paths; for flat, use base+slug.
		isHier := d.Capabilities.Hierarchical || f.Hierarchical
		if isHier && len(hRows) > 0 {
			// Build hierarchy for path computation.
			nodes := make([]content.HierarchyNode, 0, len(hRows))
			for _, r := range hRows {
				parent := ""
				if r.ParentEntryID.Valid {
					parent = r.ParentEntryID.String
				}
				nodes = append(nodes, content.HierarchyNode{EntryID: r.EntryID, Slug: r.Slug, ParentEntryID: parent, MenuOrder: r.MenuOrder, Title: r.Title})
			}
			if hTree, err := content.NewHierarchy(nodes); err == nil {
				// Compute desired paths via new base.
				paths := make(map[string]string, len(nodes))
				var compile func(string) (string, error)
				compile = func(id string) (string, error) {
					if p, ok := paths[id]; ok {
						return p, nil
					}
					n, ok := hTree.Node(id)
					if !ok {
						return "", fmt.Errorf("missing %s", id)
					}
					var p string
					if n.ParentEntryID == "" {
						def := content.ContentTypeDefinition{ID: d.ID, Routing: content.RoutingPolicy{BasePath: newBase}, Capabilities: content.Capabilities{HasArchive: newArchive}}
						p = routing.EntryPathForDefinition(def, n.Slug, "")
					} else {
						pp, err := compile(n.ParentEntryID)
						if err != nil {
							return "", err
						}
						p = routing.ChildEntryPath(pp, n.Slug)
					}
					paths[id] = p
					return p, nil
				}
				for _, n := range nodes {
					if _, err := compile(n.EntryID); err != nil {
						h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: f, Fields: fields, Error: err.Error()})
						return
					}
				}
				for id, p := range paths {
					_ = routing.UpsertEntryRoute(r.Context(), qtx, id, p, now)
				}
			}
		} else {
			for _, row := range hRows {
				def := content.ContentTypeDefinition{ID: d.ID, Routing: content.RoutingPolicy{BasePath: newBase}, Capabilities: content.Capabilities{HasArchive: newArchive}}
				p := routing.EntryPathForDefinition(def, row.Slug, "")
				_ = routing.UpsertEntryRoute(r.Context(), qtx, row.EntryID, p, now)
			}
		}
	} else if prevPublic && newPublic {
		// Public → public: handle base/archive moves.
		if !isBuiltin && (prevBase != newBase || prevArchive != newArchive) {
			// If new base is empty but archive wants path, validation already enforced; fallback to previous.
			if newBase == "" && newArchive {
				// Public archived types require base – keep previous to avoid invalid state.
				newBase = prevBase
			}
			if newBase != "" {
				if err := routing.SyncContentTypeRouting(r.Context(), qtx, string(d.ID), prevBase, newBase, newArchive, now); err != nil {
					h.renderContentTypes(w, r, contentTypesData{Mode: "edit", Form: f, Fields: fields, Error: err.Error()})
					return
				}
			} else {
				// Base cleared while staying public without archive – just delete archive if needed.
				if !newArchive {
					if route, err := qtx.GetArchiveRouteByContentType(r.Context(), sql.NullString{String: string(d.ID), Valid: true}); err == nil {
						_ = qtx.DeleteRoute(r.Context(), route.ID)
					}
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", 500)
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
	// Clean up archive route if present (no entries exist, so single routes already empty)
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
	return contentTypeForm{ID: strings.TrimSpace(r.FormValue("id")), Name: strings.TrimSpace(r.FormValue("name")), PluralName: strings.TrimSpace(r.FormValue("plural_name")), BasePath: strings.TrimSpace(r.FormValue("base_path")), Public: r.FormValue("public") != "", Hierarchical: r.FormValue("hierarchical") != "", Archive: r.FormValue("archive") != "", Excerpt: r.FormValue("excerpt") != "", Featured: r.FormValue("featured") != "", SEO: r.FormValue("seo") != ""}
}
func formFromDefinition(d content.ContentTypeDefinition) contentTypeForm {
	return contentTypeForm{ID: string(d.ID), Name: d.Name, PluralName: d.PluralName, BasePath: d.Routing.BasePath, Public: d.Capabilities.Public, Hierarchical: d.Capabilities.Hierarchical, Archive: d.IsArchived(), Excerpt: d.Capabilities.HasExcerpt, Featured: d.Capabilities.HasFeatured, SEO: d.Capabilities.HasSEO}
}
func contentTypeInput(f contentTypeForm, fields []content.FieldDefinition) content.ContentTypeInput {
	return content.ContentTypeInput{ID: content.ContentTypeID(f.ID), Name: f.Name, PluralName: f.PluralName, Public: f.Public, Hierarchical: f.Hierarchical, Config: content.ContentTypeConfig{Fields: fields, Features: content.ContentTypeFeatures{Excerpt: f.Excerpt, FeaturedMedia: f.Featured, SEO: f.SEO}, Routing: content.ContentTypeRouting{BasePath: f.BasePath, Archive: f.Archive}}}
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
	// Bounded scans: entry revisions
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
	// Layout template revisions
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
	// Also check orderBy / filters stored as JSON in block settings? Already covered by document_json scan.
	return count
}
