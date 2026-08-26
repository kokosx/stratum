package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/comments"
)

type adminCommentView struct {
	ID, AuthorName, AuthorEmail, AuthorURL, Body, Status string
	CreatedAt                                            int64
	EntryID, EntryTitle, EntryURL                        string
}

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
	commentViews := make([]adminCommentView, 0, len(rows))
	for _, c := range rows {
		view := adminCommentView{ID: c.ID, AuthorName: c.AuthorName, AuthorEmail: c.AuthorEmail, AuthorURL: c.AuthorUrl, Body: c.Body, Status: c.Status, CreatedAt: c.CreatedAt, EntryID: c.EntryID, EntryTitle: c.EntryID}
		if entry, err := h.queries.GetEntry(r.Context(), c.EntryID); err == nil {
			view.EntryTitle = entry.Slug
			if rev, err := h.queries.GetLatestEntryRevision(r.Context(), entry.ID); err == nil && rev.Title != "" {
				view.EntryTitle = rev.Title
			}
			switch entry.ContentTypeID {
			case "post":
				view.EntryURL = "/admin/posts/" + entry.ID + "/edit"
			case "page":
				view.EntryURL = "/admin/pages/" + entry.ID + "/edit"
			}
		}
		commentViews = append(commentViews, view)
	}
	data := map[string]any{
		"Comments":  commentViews,
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
		if !authz.Allows(user.Role, authz.DeleteComments) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
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
		status := action
		if action == "approve" {
			status = comments.StatusApproved
		}
		if err := h.comments.BulkUpdateStatus(r.Context(), ids, status, now); err != nil {
			h.setFlash(w, "Bulk action completed with errors: "+err.Error())
			http.Redirect(w, r, "/admin/comments", http.StatusSeeOther)
			return
		}
	case "restore":
		var errs []error
		for _, id := range ids {
			if err := h.comments.Restore(r.Context(), id, now); err != nil {
				errs = append(errs, err)
			}
		}
		if err := errors.Join(errs...); err != nil {
			h.setFlash(w, "Bulk action completed with errors: "+err.Error())
			http.Redirect(w, r, "/admin/comments", http.StatusSeeOther)
			return
		}
	case "delete":
		if !authz.Allows(user.Role, authz.DeleteComments) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		var errs []error
		for _, id := range ids {
			if err := h.comments.Delete(r.Context(), id); err != nil {
				errs = append(errs, err)
			}
		}
		if err := errors.Join(errs...); err != nil {
			h.setFlash(w, "Bulk action completed with errors: "+err.Error())
			http.Redirect(w, r, "/admin/comments", http.StatusSeeOther)
			return
		}
	default:
		http.Error(w, "Invalid bulk action", http.StatusBadRequest)
		return
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
