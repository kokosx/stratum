package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/preview"
)

func (h *Handler) handleCreatePreviewLink(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	entryID := strings.TrimSpace(r.FormValue("entry_id"))
	if entryID == "" {
		entryID = strings.TrimSpace(r.PathValue("id"))
	}
	revisionID := strings.TrimSpace(r.FormValue("revision_id"))
	expiresStr := strings.TrimSpace(r.FormValue("expires"))
	if expiresStr == "" {
		expiresStr = "24h"
	}
	expires, err := preview.ParseExpiry(expiresStr)
	if err != nil {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", err.Error()))
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if revisionID == "" && entryID != "" {
		if rev, err := h.queries.GetLatestEntryRevision(r.Context(), entryID); err == nil {
			revisionID = rev.ID
		}
	}
	if entryID == "" || revisionID == "" {
		http.Error(w, "entry and revision required", http.StatusBadRequest)
		return
	}
	// Authorization: only users who can edit the entry may create preview links
	entry, err := h.queries.GetEntry(r.Context(), entryID)
	if err != nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}
	if !authz.CanAccessEntry(user.Role, user.ID, entry.AuthorID.String, entry.ContentTypeID, authz.EntryEdit) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	// Validate revision belongs to entry (also checked in service)
	if rev, err := h.queries.GetEntryRevision(r.Context(), revisionID); err != nil || rev.EntryID != entryID {
		http.Error(w, "revision not found", http.StatusBadRequest)
		return
	}
	svc := preview.NewService(h.database, h.queries)
	token, link, err := svc.Create(r.Context(), entryID, revisionID, user.ID, expires)
	if err != nil {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", err.Error()))
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	siteURL := ""
	if settings, err := h.queries.GetSiteSettings(r.Context()); err == nil {
		siteURL = strings.TrimRight(strings.TrimSpace(settings.SiteUrl), "/")
	}
	if siteURL == "" {
		if os.Getenv("STRATUM_ENV") == "production" {
			http.Error(w, "Configure Site URL before sharing external previews.", http.StatusBadRequest)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		siteURL = scheme + "://" + r.Host
	}
	fullURL := siteURL + "/_stratum/preview/" + token
	expiresAt := time.Unix(link.ExpiresAt, 0).Format("02 Jan, 15:04")
	csrfToken, _ := h.csrfToken(w, r)

	if isDatastarRequest(r) {
		html := fmt.Sprintf(`<div class="share-preview-result"><div class="form-group"><label>Preview URL</label><div style="display:flex;gap:8px;"><input class="form-control" value="%s" readonly style="flex:1"><button class="button" type="button" onclick="navigator.clipboard.writeText('%s');window.stratumToast('success','Copied')">Copy</button></div><p class="form-help">Expires %s</p></div><form method="post" action="/admin/preview-links/%s/revoke"><input type="hidden" name="csrf_token" value="%s"><button class="button button-small button-danger" type="submit" data-on:click__prevent="@post('/admin/preview-links/%s/revoke', {contentType: 'form'})">Revoke</button></form></div>`,
			escapeAttr(fullURL), escapeAttr(fullURL), escapeAttr(expiresAt), escapeAttr(link.ID), escapeAttr(csrfToken), escapeAttr(link.ID))
		writeSSE(w, patchElementsEvent("inner", "#preview-share-result", html), toastEvent("success", "Preview link created"))
		return
	}
	h.setFlash(w, fmt.Sprintf("Preview link created: %s (expires %s)", fullURL, expiresAt))
	referer := r.Header.Get("Referer")
	if referer == "" && entryID != "" {
		if e, err := h.queries.GetEntry(r.Context(), entryID); err == nil {
			referer = "/admin/" + contentTypeToAdminPath(e.ContentTypeID) + "/" + entryID + "/edit"
		}
	}
	if referer == "" {
		referer = "/admin"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func (h *Handler) handleRevokePreviewLink(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	svc := preview.NewService(h.database, h.queries)
	// Ownership check: load link → entry → CanAccessEntry
	link, err := svc.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	entry, err := h.queries.GetEntry(r.Context(), link.EntryID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !authz.CanAccessEntry(user.Role, user.ID, entry.AuthorID.String, entry.ContentTypeID, authz.EntryEdit) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := svc.Revoke(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, patchElementsEvent("outer", fmt.Sprintf("#preview-link-%s", id), ""), toastEvent("success", "Preview link revoked"))
		return
	}
	h.setFlash(w, "Preview link revoked")
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func (h *Handler) handleListPreviewLinks(w http.ResponseWriter, r *http.Request) {
	entryID := strings.TrimSpace(r.URL.Query().Get("entry_id"))
	if entryID == "" {
		entryID = strings.TrimSpace(r.PathValue("id"))
	}
	if entryID == "" {
		http.Error(w, "entry_id required", http.StatusBadRequest)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	entry, err := h.queries.GetEntry(r.Context(), entryID)
	if err != nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}
	if !authz.CanAccessEntry(user.Role, user.ID, entry.AuthorID.String, entry.ContentTypeID, authz.EntryEdit) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	svc := preview.NewService(h.database, h.queries)
	links, err := svc.ListActiveViewByEntry(r.Context(), entryID)
	if err != nil {
		log.Printf("list preview links: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(links); err != nil {
		log.Printf("encode preview links: %v", err)
	}
}

func contentTypeToAdminPath(ct string) string {
	switch ct {
	case "page":
		return "pages"
	case "post":
		return "posts"
	default:
		return "content/" + ct
	}
}

func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}
