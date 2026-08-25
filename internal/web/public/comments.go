package public

import (
	"context"
	"database/sql"
	"html"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/comments"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/rendering"
)

func (h *Handler) handleCommentSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	entryID := strings.TrimSpace(r.FormValue("entry_id"))
	parentID := strings.TrimSpace(r.FormValue("parent_id"))
	authorName := strings.TrimSpace(r.FormValue("author_name"))
	authorEmail := strings.TrimSpace(r.FormValue("author_email"))
	authorURL := strings.TrimSpace(r.FormValue("author_url"))
	body := r.FormValue("body")
	honeypot := strings.TrimSpace(r.FormValue("website_confirm"))
	isDatastar := r.Header.Get("Datastar-Request") == "true"

	var userID, role string
	// Anonymous for now; logged-in handling can be added later via session
	_ = userID
	_ = role

	hasUnlock := false
	if entryID != "" {
		if entry, err := h.queries.GetEntry(r.Context(), entryID); err == nil && entry.PublishedRevisionID.Valid {
			if rev, err := h.queries.GetEntryRevision(r.Context(), entry.PublishedRevisionID.String); err == nil {
				if rev.Visibility == "password" {
					cookieName := h.unlockStore.CookieName(entryID)
					if c, err := r.Cookie(cookieName); err == nil {
						if h.unlockStore.Valid(c.Value, entryID, rev.ID, time.Now().Unix()) {
							hasUnlock = true
						}
					}
				} else if rev.Visibility == "public" {
					hasUnlock = true
				}
			}
		}
	}

	ip := r.RemoteAddr
	now := time.Now().Unix()
	comment, err := h.comments.Submit(r.Context(), entryID, parentID, authorName, authorEmail, authorURL, body, honeypot, userID, role, ip, hasUnlock, now)
	if err != nil {
		if isDatastar {
			errMsg := html.EscapeString(err.Error())
			if err == comments.ErrHoneypot {
				errMsg = "Spam detected"
			}
			h.writeDatastarCommentError(w, errMsg)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if isDatastar {
		if comment.Status == comments.StatusApproved {
			h.writeDatastarCommentApproved(w, r, entryID)
		} else {
			h.writeDatastarCommentPending(w)
		}
		return
	}
	if entryID != "" {
		if route, err := h.queries.GetEntryRoute(r.Context(), sql.NullString{String: entryID, Valid: true}); err == nil {
			http.Redirect(w, r, route.Path+"#comments", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) writeDatastarCommentError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	body := "event: datastar-patch-elements\ndata: selector #comment-form-errors\ndata: mode inner\ndata: elements <div id=\"comment-form-errors\" class=\"form-error\">" + msg + "</div>\n\n"
	_, _ = w.Write([]byte(body))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (h *Handler) writeDatastarCommentPending(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	body := "event: datastar-patch-elements\ndata: selector #comment-submit-status\ndata: mode inner\ndata: elements <div id=\"comment-submit-status\" class=\"success\">Your comment is awaiting moderation.</div>\n\nevent: datastar-patch-elements\ndata: selector #comment-form\ndata: mode outer\ndata: elements <form id=\"comment-form\" method=\"post\" action=\"/comments\"><p>Form cleared</p></form>\n\n"
	_, _ = w.Write([]byte(body))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (h *Handler) writeDatastarCommentApproved(w http.ResponseWriter, r *http.Request, entryID string) {
	views := h.buildCommentViews(r.Context(), entryID)
	var b strings.Builder
	b.WriteString("<div id=\"comment-list\">")
	for _, c := range views {
		b.WriteString("<article class=\"stratum-comment\" id=\"comment-" + html.EscapeString(c.ID) + "\"><header><span>" + html.EscapeString(c.AuthorName) + "</span></header><div>" + html.EscapeString(c.Body) + "</div></article>")
	}
	b.WriteString("</div>")
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	body := "event: datastar-patch-elements\ndata: selector #comment-list\ndata: mode outer\ndata: elements " + strings.ReplaceAll(b.String(), "\n", " ") + "\n\nevent: datastar-patch-elements\ndata: selector #comment-submit-status\ndata: mode inner\ndata: elements <div id=\"comment-submit-status\" class=\"success\">Comment posted.</div>\n\n"
	_, _ = w.Write([]byte(body))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (h *Handler) buildCommentViews(ctx context.Context, entryID string) []rendering.CommentView {
	if h.comments == nil {
		return nil
	}
	// Use non-transactional context for rendering
	rows, err := h.comments.ListApproved(ctx, entryID)
	if err != nil {
		return nil
	}
	views := make([]rendering.CommentView, 0, len(rows))
	for _, c := range rows {
		bodyHTML := html.EscapeString(c.Body)
		bodyHTML = strings.ReplaceAll(bodyHTML, "\n", "<br>")
		views = append(views, rendering.CommentView{
			ID:         c.ID,
			ParentID:   c.ParentID.String,
			AuthorName: c.AuthorName,
			Body:       c.Body,
			BodyHTML:   template.HTML(bodyHTML),
			CreatedAt:  time.Unix(c.CreatedAt, 0).Format("2006-01-02 15:04"),
			CreatedISO: time.Unix(c.CreatedAt, 0).UTC().Format(time.RFC3339),
			Depth:      0,
		})
	}
	return views
}

func (h *Handler) populateCommentsContext(ctx context.Context, rc *rendering.RenderContext, entryID string, hasUnlock bool) {
	if entryID == "" || h.comments == nil {
		return
	}
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil {
		return
	}
	if !entry.PublishedRevisionID.Valid {
		return
	}
	rev, err := h.queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil {
		return
	}
	def := content.DefinitionFor(entry.ContentTypeID)
	if !def.Capabilities.SupportsComments {
		return
	}
	switch rev.Visibility {
	case "private":
		return
	case "password":
		if !hasUnlock {
			return
		}
	}
	rc.CommentsEnabled = rev.CommentsEnabled != 0
	rc.CommentsEntryID = entryID
	if rev.CommentsEnabled == 0 {
		rc.CommentsCount = 0
		return
	}
	views := h.buildCommentViews(ctx, entryID)
	rc.Comments = views
	rc.CommentsCount = len(views)
}
