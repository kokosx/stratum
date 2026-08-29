package admin

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/customcode"
)

func (h *Handler) requireCustomCodeAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if !authz.Allows(user.Role, authz.ManageSite) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

type customCodeListData struct {
	Section      string
	CSRFToken    string
	Flash        string
	Global       []customcode.Snippet
	TemplateID   string
	TemplateName string
	Templates    []struct{ ID, Name string }
	SelectedTPL  string
	Error        string
}

func (h *Handler) listCustomCode(w http.ResponseWriter, r *http.Request) {
	if !h.requireCustomCodeAdmin(w, r) {
		return
	}
	token, _ := h.csrfToken(w, r)
	data := customCodeListData{Section: "custom-code", CSRFToken: token, Flash: h.consumeFlash(w, r)}
	if h.customCode != nil {
		if gl, err := h.customCode.ListGlobal(r.Context()); err == nil {
			data.Global = gl
		}
	}
	// load template options for UI (need for global->template links)
	templates, _ := h.queries.ListLayoutTemplates(r.Context())
	for _, t := range templates {
		data.Templates = append(data.Templates, struct{ ID, Name string }{ID: t.ID, Name: t.Name})
	}
	h.renderCustomCode(w, r, data)
}

func (h *Handler) renderCustomCode(w http.ResponseWriter, r *http.Request, data customCodeListData) {
	state := ResolveNav(r.URL.Path)
	layout := LayoutData{Title: "Custom Code", ActiveMenu: state.ActiveSection, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: h.navForUser(r), Flash: data.Flash, CSRFToken: data.CSRFToken, Content: data}
	if err := h.customCodeTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		// fallback inline
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<pre>render error: " + template.HTMLEscapeString(err.Error()) + "</pre>"))
	}
}

func (h *Handler) createCustomCodeSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.requireCustomCodeAdmin(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	scope := strings.TrimSpace(r.FormValue("scope"))
	scopeID := strings.TrimSpace(r.FormValue("scope_id"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	placement := strings.TrimSpace(r.FormValue("placement"))
	code := r.FormValue("code")
	enabled := r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1"
	sortOrderStr := strings.TrimSpace(r.FormValue("sort_order"))
	var sortOrder int64
	if sortOrderStr != "" {
		if v, err := strconv.ParseInt(sortOrderStr, 10, 64); err == nil {
			sortOrder = v
		}
	}
	if scope == "" {
		// If path is template custom-code, force template scope
		if strings.Contains(r.URL.Path, "/appearance/templates/") {
			scope = string(customcode.ScopeTemplate)
			if scopeID == "" {
				scopeID = r.PathValue("id")
			}
		} else {
			scope = string(customcode.ScopeGlobal)
		}
	}
	if kind == "" {
		kind = string(customcode.KindHTML)
	}
	if placement == "" {
		placement = string(customcode.PlacementHead)
	}
	if scope == string(customcode.ScopeTemplate) && scopeID == "" {
		scopeID = r.FormValue("template_id")
		if scopeID == "" {
			scopeID = r.PathValue("id")
		}
	}
	if h.customCode == nil {
		http.Error(w, "Custom code service unavailable", http.StatusServiceUnavailable)
		return
	}
	_, err := h.customCode.Create(r.Context(), name, scope, scopeID, kind, placement, code, enabled, sortOrder)
	if err != nil {
		token, _ := h.csrfToken(w, r)
		data := customCodeListData{Section: "custom-code", CSRFToken: token, Error: err.Error()}
		h.renderCustomCode(w, r, data)
		return
	}
	h.setFlash(w, "Custom code snippet created.")
	if scope == string(customcode.ScopeTemplate) && scopeID != "" {
		http.Redirect(w, r, "/admin/appearance/templates/"+scopeID+"/custom-code", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/settings/custom-code", http.StatusSeeOther)
}

func (h *Handler) updateCustomCodeSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.requireCustomCodeAdmin(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	placement := strings.TrimSpace(r.FormValue("placement"))
	code := r.FormValue("code")
	enabled := r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1"
	sortOrderStr := strings.TrimSpace(r.FormValue("sort_order"))
	var sortOrder int64
	if sortOrderStr != "" {
		if v, err := strconv.ParseInt(sortOrderStr, 10, 64); err == nil {
			sortOrder = v
		}
	}
	if h.customCode == nil {
		http.Error(w, "Custom code service unavailable", http.StatusServiceUnavailable)
		return
	}
	_, err := h.customCode.Update(r.Context(), id, name, kind, placement, code, enabled, sortOrder)
	if err != nil {
		h.setFlash(w, err.Error())
		http.Redirect(w, r, "/admin/settings/custom-code", http.StatusSeeOther)
		return
	}
	h.setFlash(w, "Custom code updated.")
	http.Redirect(w, r, "/admin/settings/custom-code", http.StatusSeeOther)
}

func (h *Handler) deleteCustomCodeSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.requireCustomCodeAdmin(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if h.customCode == nil {
		http.Error(w, "Custom code service unavailable", http.StatusServiceUnavailable)
		return
	}
	_ = h.customCode.Delete(r.Context(), id)
	h.setFlash(w, "Custom code deleted.")
	http.Redirect(w, r, "/admin/settings/custom-code", http.StatusSeeOther)
}

func (h *Handler) toggleCustomCodeSnippet(w http.ResponseWriter, r *http.Request) {
	if !h.requireCustomCodeAdmin(w, r) {
		return
	}
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	enabled := r.FormValue("enabled") == "1" || r.FormValue("enabled") == "on"
	if h.customCode == nil {
		http.Error(w, "Custom code service unavailable", http.StatusServiceUnavailable)
		return
	}
	_ = h.customCode.Toggle(r.Context(), id, enabled)
	http.Redirect(w, r, "/admin/settings/custom-code", http.StatusSeeOther)
}

func (h *Handler) templateCustomCode(w http.ResponseWriter, r *http.Request) {
	if !h.requireCustomCodeAdmin(w, r) {
		return
	}
	templateID := r.PathValue("id")
	if templateID == "" {
		templateID = r.FormValue("template_id")
	}
	if templateID == "" {
		http.NotFound(w, r)
		return
	}
	tmpl, err := h.queries.GetLayoutTemplate(r.Context(), templateID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	token, _ := h.csrfToken(w, r)
	data := customCodeListData{Section: "custom-code", CSRFToken: token, TemplateID: templateID, TemplateName: tmpl.Name}
	if h.customCode != nil {
		if gl, err := h.customCode.ListForTemplate(r.Context(), templateID); err == nil {
			data.Global = gl
		}
	}
	state := ResolveNav(r.URL.Path)
	layout := LayoutData{Title: "Custom Code — " + tmpl.Name, ActiveMenu: state.ActiveSection, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: h.navForUser(r), Flash: h.consumeFlash(w, r), CSRFToken: token, Content: data}
	if err := h.customCodeTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// helpers for template snippet rendering
func (c customCodeListData) HasSnippets() bool { return len(c.Global) > 0 }

// Ensure import for sql is used
var _ = sql.ErrNoRows
