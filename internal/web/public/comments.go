package public

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/comments"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/rendering"
)

var commentFragments = template.Must(template.New("comments").Parse(`{{define "errors"}}<div id="comment-form-errors" class="form-error">{{.Message}}</div>{{end}}{{define "status"}}<div id="comment-submit-status" class="success">{{.Message}}</div>{{end}}{{define "form"}}<form id="comment-form" method="post" action="/comments" class="stratum-comment-form" data-on:submit__prevent="@post('/comments', {contentType: 'form'})"><input type="hidden" name="entry_id" value="{{.EntryID}}"><input type="hidden" name="parent_id" value=""><div id="comment-form-errors"></div><div class="stratum-comment-field"><label for="comment-author-name">Name</label><input id="comment-author-name" name="author_name" required maxlength="100"></div><div class="stratum-comment-field"><label for="comment-author-email">Email</label><input id="comment-author-email" name="author_email" type="email" required maxlength="254"></div><div class="stratum-comment-field"><label for="comment-author-url">Website (optional)</label><input id="comment-author-url" name="author_url" type="url" maxlength="2048"></div><p class="stratum-comment-honeypot"><label for="comment-website-confirm">Leave empty</label><input id="comment-website-confirm" name="website_confirm" autocomplete="off" tabindex="-1"></p><div class="stratum-comment-field"><label for="comment-body">Comment</label><textarea id="comment-body" name="body" required maxlength="5000"></textarea></div><div class="stratum-comment-actions"><button type="submit">Post Comment</button></div><div id="comment-submit-status"></div></form>{{end}}{{define "list"}}<div id="comment-list">{{range .}}<article class="stratum-comment comment-depth-{{.Depth}}" id="comment-{{.ID}}"><header class="stratum-comment-header"><span class="stratum-comment-author">{{.AuthorName}}</span> <time datetime="{{.CreatedISO}}">{{.CreatedAt}}</time>{{if .ParentID}} <a href="#comment-{{.ParentID}}">in reply</a>{{end}}</header><div class="stratum-comment-body">{{.Body}}</div></article>{{else}}<p class="stratum-comments-empty">No comments yet. Be the first to comment.</p>{{end}}</div>{{end}}`))

func renderCommentFragment(name string, data any) string {
	var b bytes.Buffer
	if err := commentFragments.ExecuteTemplate(&b, name, data); err != nil {
		return ""
	}
	return b.String()
}

func (h *Handler) handleCommentSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if !h.sameOriginCommentPost(r) {
		http.Error(w, "Cross-origin comment submission rejected", http.StatusForbidden)
		return
	}
	entryID, parentID := strings.TrimSpace(r.FormValue("entry_id")), strings.TrimSpace(r.FormValue("parent_id"))
	isDatastar := r.Header.Get("Datastar-Request") == "true"
	var userID, role string
	if h.auth != nil {
		if c, err := r.Cookie(auth.CookieName); err == nil {
			if u, err := h.auth.UserForToken(r.Context(), c.Value); err == nil {
				userID, role = u.ID, u.Role
			}
		}
	}
	hasUnlock := false
	if entryID != "" {
		if entry, err := h.queries.GetEntry(r.Context(), entryID); err == nil && entry.PublishedRevisionID.Valid {
			if rev, err := h.queries.GetEntryRevision(r.Context(), entry.PublishedRevisionID.String); err == nil {
				if rev.Visibility == "password" {
					if c, err := r.Cookie(h.unlockStore.CookieName(entryID)); err == nil {
						hasUnlock = h.unlockStore.Valid(c.Value, entryID, rev.ID, time.Now().Unix())
					}
				} else if rev.Visibility == "public" {
					hasUnlock = true
				}
			}
		}
	}
	comment, err := h.comments.Submit(r.Context(), entryID, parentID, strings.TrimSpace(r.FormValue("author_name")), strings.TrimSpace(r.FormValue("author_email")), strings.TrimSpace(r.FormValue("author_url")), r.FormValue("body"), strings.TrimSpace(r.FormValue("website_confirm")), userID, role, r.RemoteAddr, hasUnlock, time.Now().Unix())
	if err != nil {
		if isDatastar {
			if err == comments.ErrHoneypot {
				h.writeDatastarCommentError(w, "Spam detected")
			} else {
				h.writeDatastarCommentError(w, err.Error())
			}
			return
		}
		status := http.StatusBadRequest
		if err == comments.ErrRateLimited {
			status = http.StatusTooManyRequests
		}
		if err == comments.ErrCommentsDisabled || err == comments.ErrNotCommentable {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	if isDatastar {
		if comment.Status == comments.StatusApproved {
			h.writeDatastarCommentApproved(w, r, entryID)
		} else {
			h.writeDatastarCommentPending(w, entryID)
		}
		return
	}
	if route, err := h.queries.GetEntryRoute(r.Context(), sql.NullString{String: entryID, Valid: true}); err == nil {
		http.Redirect(w, r, route.Path+"#comments", http.StatusSeeOther)
		return
	}
	http.Error(w, "Comment submitted; its entry route is unavailable.", http.StatusOK)
}

func (h *Handler) sameOriginCommentPost(r *http.Request) bool {
	expected := ""
	if snap := h.hub.Site.Current(); snap != nil {
		expected = snap.SiteURL
	}
	if expected == "" {
		expected = "http://" + r.Host
	}
	want, err := url.Parse(expected)
	if err != nil || want.Host == "" {
		return false
	}
	provided := r.Header.Get("Origin")
	if provided == "" {
		provided = r.Referer()
	}
	if provided == "" {
		return true
	}
	got, err := url.Parse(provided)
	return err == nil && got.Scheme == want.Scheme && got.Host == want.Host
}
func (h *Handler) writeCommentSSE(w http.ResponseWriter, selector, mode, element string) {
	_, _ = fmt.Fprintf(w, "event: datastar-patch-elements\ndata: selector %s\ndata: mode %s\ndata: elements %s\n\n", selector, mode, strings.ReplaceAll(element, "\n", " "))
}
func (h *Handler) writeDatastarCommentError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	h.writeCommentSSE(w, "#comment-form-errors", "inner", renderCommentFragment("errors", map[string]string{"Message": msg}))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
func (h *Handler) writeDatastarCommentPending(w http.ResponseWriter, entryID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	h.writeCommentSSE(w, "#comment-submit-status", "inner", renderCommentFragment("status", map[string]string{"Message": "Your comment is awaiting moderation."}))
	h.writeCommentSSE(w, "#comment-form", "outer", renderCommentFragment("form", map[string]string{"EntryID": entryID}))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
func (h *Handler) writeDatastarCommentApproved(w http.ResponseWriter, r *http.Request, entryID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	views := h.buildCommentViews(r.Context(), entryID)
	h.writeCommentSSE(w, "#comment-list", "outer", renderCommentFragment("list", views))
	count := len(views)
	if n, err := h.comments.CountApproved(r.Context(), entryID); err == nil {
		count = int(n)
	}
	title := "Comments"
	if count > 0 {
		title = fmt.Sprintf("Comments (%d)", count)
	}
	h.writeCommentSSE(w, ".stratum-comments-title", "inner", title)
	h.writeCommentSSE(w, "#comment-submit-status", "inner", renderCommentFragment("status", map[string]string{"Message": "Comment posted."}))
	h.writeCommentSSE(w, "#comment-form", "outer", renderCommentFragment("form", map[string]string{"EntryID": entryID}))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (h *Handler) buildCommentViews(ctx context.Context, entryID string) []rendering.CommentView {
	if h.comments == nil || entryID == "" {
		return nil
	}
	rows, err := h.comments.ListApproved(ctx, entryID)
	if err != nil {
		return nil
	}
	parents := make(map[string]string, len(rows))
	for _, c := range rows {
		parents[c.ID] = c.ParentID.String
	}
	views := make([]rendering.CommentView, 0, len(rows))
	for _, c := range rows {
		parent, depth := c.ParentID.String, 1
		seen := map[string]bool{c.ID: true}
		for parent != "" {
			next, ok := parents[parent]
			if !ok || seen[parent] {
				parent = ""
				break
			}
			seen[parent] = true
			depth++
			parent = next
		}
		if depth > comments.MaxDepth {
			depth = comments.MaxDepth
		}
		views = append(views, rendering.CommentView{ID: c.ID, ParentID: c.ParentID.String, AuthorName: c.AuthorName, Body: c.Body, CreatedAt: time.Unix(c.CreatedAt, 0).Format("2006-01-02 15:04"), CreatedISO: time.Unix(c.CreatedAt, 0).UTC().Format(time.RFC3339), Depth: depth - 1})
	}
	return views
}

func (h *Handler) populateCommentsContext(ctx context.Context, rc *rendering.RenderContext, entryID string, hasUnlock bool) {
	if entryID == "" || h.comments == nil {
		return
	}
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil || !entry.PublishedRevisionID.Valid {
		return
	}
	rev, err := h.queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil || !content.DefinitionFor(entry.ContentTypeID).Capabilities.SupportsComments {
		return
	}
	if rev.Visibility == "private" || (rev.Visibility == "password" && !hasUnlock) {
		return
	}
	rc.CommentsEnabled, rc.CommentsEntryID = rev.CommentsEnabled != 0, entryID
	rc.Comments = h.buildCommentViews(ctx, entryID)
	if count, err := h.comments.CountApproved(ctx, entryID); err == nil {
		rc.CommentsCount = int(count)
	} else {
		rc.CommentsCount = len(rc.Comments)
	}
}
