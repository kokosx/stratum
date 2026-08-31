package admin

import (
	"database/sql"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
)

func (h *Handler) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user, err := h.currentUser(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if user.Role != string(authz.RoleAdmin) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.agents.List(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Enrich with last used? For now simple
	type agentRow struct {
		ID        string
		Name      string
		Status    string
		CreatedAt string
		UpdatedAt string
	}
	var rows []agentRow
	for _, a := range agents {
		rows = append(rows, agentRow{
			ID: a.ID, Name: a.Name, Status: a.Status,
			CreatedAt: time.Unix(a.CreatedAt, 0).Format("02 Jan 2006"),
			UpdatedAt: time.Unix(a.UpdatedAt, 0).Format("02 Jan 2006"),
		})
	}
	token, _ := h.csrfToken(w, r)
	state := ResolveNav(r.URL.Path)
	_ = h.agentsTemplate.ExecuteTemplate(w, "layout.html", LayoutData{
		Title: "Agents", ActiveMenu: "settings", ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem,
		Nav: h.navForUser(r), CSRFToken: token,
		Content: map[string]any{"Agents": rows, "CSRFToken": token},
	})
}

func (h *Handler) newAgentForm(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	defs, _ := content.NewCatalog(h.queries).ListDefinitions(r.Context())
	users, _ := h.queries.ListUsers(r.Context())
	state := ResolveNav(r.URL.Path)
	_ = h.agentFormTemplate.ExecuteTemplate(w, "layout.html", LayoutData{
		Title: "New Agent", ActiveMenu: "settings", ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem,
		Nav: h.navForUser(r), CSRFToken: token,
		Content: map[string]any{"ContentTypes": defs, "Users": users, "CSRFToken": token, "IsNew": true},
	})
}

func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	defaultAuthor := strings.TrimSpace(r.FormValue("default_author_id"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	user, _ := h.currentUser(r)
	agent, err := h.agents.Create(r.Context(), name, defaultAuthor, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Parse grants from form: permissions per content type
	grants := parseAgentGrantsFromForm(r)
	_ = h.agents.ReplaceGrants(r.Context(), agent.ID, grants)
	// Auto-create first token
	issued, err := h.agents.IssueToken(r.Context(), agent.ID, "Initial token", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Show onboarding screen with raw token (once)
	h.renderAgentOnboarding(w, r, agent.ID, name, issued.Raw)
}

func (h *Handler) viewAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	agent, err := h.agents.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	grants, _ := h.agents.ListGrants(r.Context(), id)
	tokens, _ := h.agents.ListTokens(r.Context(), id)
	defs, _ := content.NewCatalog(h.queries).ListDefinitions(r.Context())
	users, _ := h.queries.ListUsers(r.Context())
	token, _ := h.csrfToken(w, r)
	// Build grants map for template
	grantMap := map[string]map[string]bool{}
	for _, g := range grants {
		if _, ok := grantMap[g.Permission]; !ok {
			grantMap[g.Permission] = map[string]bool{}
		}
		grantMap[g.Permission][g.Scope] = true
	}
	state := ResolveNav(r.URL.Path)
	_ = h.agentDetailTemplate.ExecuteTemplate(w, "layout.html", LayoutData{
		Title: agent.Name, ActiveMenu: "settings", ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem,
		Nav: h.navForUser(r), CSRFToken: token,
		Content: map[string]any{
			"Agent": agent, "Grants": grantMap, "Tokens": tokens,
			"ContentTypes": defs, "Users": users, "CSRFToken": token,
		},
	})
}

func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	defaultAuthor := strings.TrimSpace(r.FormValue("default_author_id"))
	status := strings.TrimSpace(r.FormValue("status"))
	if name != "" {
		if err := h.agents.Update(r.Context(), id, name, defaultAuthor); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if status == "active" || status == "disabled" {
		_ = h.agents.SetStatus(r.Context(), id, status)
	}
	grants := parseAgentGrantsFromForm(r)
	_ = h.agents.ReplaceGrants(r.Context(), id, grants)
	http.Redirect(w, r, "/admin/settings/agents/"+id, http.StatusSeeOther)
}

func (h *Handler) createAgentToken(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	label := strings.TrimSpace(r.FormValue("label"))
	issued, err := h.agents.IssueToken(r.Context(), id, label, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	agent, _ := h.agents.Get(r.Context(), id)
	h.renderAgentOnboarding(w, r, id, agent.Name, issued.Raw)
}

func (h *Handler) revokeAgentToken(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	tokenID := r.PathValue("tokenID")
	if tokenID == "" {
		tokenID = r.FormValue("token_id")
	}
	_ = h.agents.RevokeToken(r.Context(), tokenID)
	http.Redirect(w, r, "/admin/settings/agents/"+id, http.StatusSeeOther)
}

func (h *Handler) toggleAgentStatus(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	agent, err := h.agents.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newStatus := "active"
	if agent.Status == "active" {
		newStatus = "disabled"
	}
	_ = h.agents.SetStatus(r.Context(), id, newStatus)
	http.Redirect(w, r, "/admin/settings/agents", http.StatusSeeOther)
}

func (h *Handler) renderAgentOnboarding(w http.ResponseWriter, r *http.Request, agentID, agentName, rawToken string) {
	token, _ := h.csrfToken(w, r)
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" && r.Host == "localhost" || strings.HasPrefix(r.Host, "127.0.0.1") {
		scheme = "http"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	endpoint := scheme + "://" + host + "/stratum/mcp"
	// Build MCP config JSON
	mcpConfig := `{
  "mcpServers": {
    "stratum": {
      "url": "` + endpoint + `",
      "headers": {
        "Authorization": "Bearer ` + rawToken + `"
      }
    }
  }
}`
	setupPrompt := "Add this StratumCMS MCP server to my MCP configuration.\n\nRemote MCP URL:\n\n" + endpoint + "\n\nAuthenticate with this Bearer token:\n\n" + rawToken + "\n\nPreserve every existing MCP server and unrelated configuration.\n\nDo not remove or replace existing configuration.\n\nAfter adding Stratum, verify the connection using the Stratum site information tool."
	state := ResolveNav(r.URL.Path)
	_ = h.agentOnboardingTemplate.ExecuteTemplate(w, "layout.html", LayoutData{
		Title: "Agent Ready", ActiveMenu: "settings", ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem,
		Nav: h.navForUser(r), CSRFToken: token,
		Content: map[string]any{
			"AgentID": agentID, "AgentName": agentName, "RawToken": rawToken,
			"Endpoint": endpoint, "MCPConfig": mcpConfig, "SetupPrompt": setupPrompt, "CSRFToken": token,
		},
	})
}

func parseAgentGrantsFromForm(r *http.Request) []authz.AgentGrant {
	var out []authz.AgentGrant
	// Permissions are encoded as form keys: grant_entries.read_* etc
	// For content-type scoped perms: grant_entries.read_content_type:post etc
	// Simpler: expect form values like grant_entries.read=* and grant_entries.edit=content_type:post
	// We'll parse all form keys starting with grant_
	for key, vals := range r.Form {
		if !strings.HasPrefix(key, "grant_") {
			continue
		}
		perm := strings.TrimPrefix(key, "grant_")
		// perm is like entries.read
		for _, v := range vals {
			scope := strings.TrimSpace(v)
			if scope == "" {
				continue
			}
			// Split multiple scopes comma separated?
			for _, s := range strings.Split(scope, ",") {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				out = append(out, authz.AgentGrant{Permission: perm, Scope: s})
			}
		}
	}
	// Also handle matrix checkboxes: e.g., entries_read_post=on, entries_edit_post=on
	// We'll support pattern: perm_<content_type>
	matrixPerms := []string{"entries.read", "entries.create", "entries.edit", "entries.publish", "entries.trash"}
	for _, perm := range matrixPerms {
		keyPrefix := "grant_" + perm // e.g., grant_entries.read
		// Direct star
		if r.FormValue(keyPrefix) == "*" || r.FormValue(keyPrefix+"_star") == "on" {
			out = append(out, authz.AgentGrant{Permission: perm, Scope: "*"})
		}
		// Per content type checkboxes: grant_entries.read_post etc? We'll check using content type IDs
		// We don't have catalog here; rely on form keys containing content_type:
		for key := range r.Form {
			if strings.HasPrefix(key, "grant_"+perm+"__") && r.FormValue(key) == "on" {
				ct := strings.TrimPrefix(key, "grant_"+perm+"__")
				if ct != "" {
					out = append(out, authz.AgentGrant{Permission: perm, Scope: "content_type:" + ct})
				}
			}
		}
	}
	// Non-entry perms: media.read etc as single checkbox with "*"
	for _, perm := range []string{"media.read", "media.upload", "media.edit", "taxonomies.read", "taxonomies.assign", "content_types.read", "blocks.read", "site.read"} {
		if r.FormValue("grant_"+perm) == "on" || r.FormValue("grant_"+perm) == "*" {
			out = append(out, authz.AgentGrant{Permission: perm, Scope: "*"})
		}
	}
	return out
}

// Helper to check sql error
func isAgentNotFound(err error) bool { return err != nil && strings.Contains(err.Error(), "agent not found") }

var _ = sql.ErrNoRows
var _ = template.New
