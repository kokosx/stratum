package admin

import (
	"database/sql"
	"html"
	"net/http"
	"strconv"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/comments"
)

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	user, _ := h.currentUser(r)
	if !authz.Allows(user.Role, authz.ReadComments) && !authz.Allows(user.Role, authz.ModerateComments) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}
	search := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 20
	offset := (page - 1) * limit
	// Count by status for tabs
	counts, _ := h.comments.CountByStatus(r.Context())
	totalByStatus := map[string]int64{"all": 0, "pending": 0, "approved": 0, "spam": 0, "trash": 0}
	var totalAll int64
	for _, c := range []string{"pending", "approved", "spam", "trash"} {
		v := counts[c]
		totalByStatus[c] = v
		totalAll += v
	}
	totalByStatus["all"] = totalAll

	// Filter status for query
	filterStatus := ""
	if status != "all" {
		filterStatus = status
	}
	rows, total, err := h.comments.ListFiltered(r.Context(), filterStatus, search, "", limit, offset)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	data := map[string]any{
		"Comments":  rows,
		"Counts":    totalByStatus,
		"Status":    status,
		"Search":    search,
		"Page":      page,
		"Total":     total,
		"Pages":     (int(total) + limit - 1) / limit,
		"CSRFToken": token,
	}
	state := ResolveNav(r.URL.Path)
	if err := h.commentsTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Comments", ActiveMenu: state.ActiveSection, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: h.navForUser(r), Flash: h.consumeFlash(w, r), CSRFToken: token, Content: data}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) moderateComment(w http.ResponseWriter, r *http.Request, action string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, _ := h.currentUser(r)
	if !authz.Allows(user.Role, authz.ModerateComments) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	now := currentUnix()
	var err error
	switch action {
	case "approve":
		err = h.comments.Approve(r.Context(), id, now)
	case "pending":
		err = h.comments.SetPending(r.Context(), id, now)
	case "spam":
		err = h.comments.Spam(r.Context(), id, now)
	case "trash":
		err = h.comments.Trash(r.Context(), id, now)
	case "restore":
		err = h.comments.Restore(r.Context(), id, now)
	case "delete":
		err = h.comments.Delete(r.Context(), id)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if err == comments.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Invalidate handled inside service if approved status changed
	h.setFlash(w, "Comment updated.")
	http.Redirect(w, r, "/admin/comments", http.StatusSeeOther)
}

func (h *Handler) bulkComments(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, _ := h.currentUser(r)
	if !authz.Allows(user.Role, authz.ModerateComments) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	ids := r.Form["ids"]
	action := r.FormValue("action")
	if len(ids) == 0 || action == "" {
		http.Redirect(w, r, "/admin/comments", http.StatusSeeOther)
		return
	}
	now := currentUnix()
	switch action {
	case "approve", "pending", "spam", "trash":
		_ = h.comments.BulkUpdateStatus(r.Context(), ids, action, now)
	case "restore":
		for _, id := range ids {
			_ = h.comments.Restore(r.Context(), id, now)
		}
	case "delete":
		for _, id := range ids {
			_ = h.comments.Delete(r.Context(), id)
		}
	}
	h.setFlash(w, "Bulk action completed.")
	http.Redirect(w, r, "/admin/comments", http.StatusSeeOther)
}

func currentUnix() int64 {
	// Use time.Now().Unix() for real handling; tests can override via context if needed
	return timeNowUnix()
}

func timeNowUnix() int64 {
	// Separate for testing if needed
	return timeNow().Unix()
}

var timeNow = func() time.Time { return time.Now() }

func init() {
	_ = html.EscapeString
	_ = sql.ErrNoRows
	_ = authz.ModerateComments
}
