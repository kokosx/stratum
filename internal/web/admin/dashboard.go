package admin

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/kokosx/stratum/internal/content"
)

type dashboardEntry struct {
	ID               string
	Title            string
	ContentTypeID    string
	ContentTypeLabel string
	Status           string
	StatusTone       string
	UpdatedAt        time.Time
	EditURL          string
}

type dashboardData struct {
	OnboardingIncomplete bool
	NewSubmissionCount   int64
	PendingCommentCount  int64
	HasAttention         bool

	RecentEntries []dashboardEntry

	PageCount      int64
	PostCount      int64
	FormCount      int64
	FormNewCount   int64
	MediaCount     int64
	CommentPending int64
	CustomCounts   []contentTypeCount
}

type contentTypeCount struct {
	ID    string
	Label string
	Count int64
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	incomplete := false
	if completed, err := h.queries.GetOnboardingCompleted(ctx); err == nil {
		incomplete = completed == 0
	}

	// New form submissions
	var newSubmissionCount int64
	var formCount int64
	if forms, err := h.queries.ListForms(ctx); err == nil {
		formCount = int64(len(forms))
		for _, f := range forms {
			newSubmissionCount += f.NewCount
		}
	}

	// Pending comments
	var pendingCommentCount int64
	if rows, err := h.queries.CountCommentsByStatus(ctx); err == nil {
		for _, row := range rows {
			if row.Status == "pending" {
				pendingCommentCount = row.Count
				break
			}
		}
	}

	hasAttention := incomplete || newSubmissionCount > 0 || pendingCommentCount > 0

	// Recent entries across all content types (limit 8)
	recent := h.listRecentEntries(ctx, 8)

	// Counts for shortcuts
	var pageCount, postCount, mediaCount int64
	if c, err := h.queries.CountEntries(ctx); err == nil {
		_ = c // not used, but keep for reference
	}
	// Use raw counts filtered by content_type and not trash
	if h.database != nil {
		_ = h.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE content_type_id='page' AND status!='trash'`).Scan(&pageCount)
		_ = h.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE content_type_id='post' AND status!='trash'`).Scan(&postCount)
		_ = h.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM media`).Scan(&mediaCount)
	}
	var customCounts []contentTypeCount
	if defs, err := content.NewCatalog(h.queries).ListDefinitions(ctx); err == nil {
		for _, d := range defs {
			if d.ID == content.ContentTypePage || d.ID == content.ContentTypePost {
				continue
			}
			var cnt int64
			if h.database != nil {
				_ = h.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE content_type_id=? AND status!='trash'`, string(d.ID)).Scan(&cnt)
			}
			customCounts = append(customCounts, contentTypeCount{ID: string(d.ID), Label: d.Label(), Count: cnt})
		}
	}

	state := ResolveNav(r.URL.Path)
	data := LayoutData{
		Title:         "Dashboard",
		ActiveMenu:    state.ActiveSection,
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		Content: dashboardData{
			OnboardingIncomplete: incomplete,
			NewSubmissionCount:   newSubmissionCount,
			PendingCommentCount:  pendingCommentCount,
			HasAttention:         hasAttention,
			RecentEntries:        recent,
			PageCount:            pageCount,
			PostCount:            postCount,
			FormCount:            formCount,
			FormNewCount:         newSubmissionCount,
			MediaCount:           mediaCount,
			CommentPending:       pendingCommentCount,
			CustomCounts:         customCounts,
		},
	}
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.CSRFToken = token

	if err := h.dashboardTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render admin dashboard: %v", err)
	}
}

func (h *Handler) listRecentEntries(ctx context.Context, limit int) []dashboardEntry {
	if h.database == nil {
		return nil
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	query := `
SELECT e.id, e.content_type_id, COALESCE(ct.plural_name, e.content_type_id) AS ct_label,
       COALESCE(latest.title, e.slug) AS title,
       e.status, e.published_revision_id, COALESCE(latest.review_state,'') AS review_state,
       e.updated_at, r.path AS public_path
FROM entries e
LEFT JOIN content_types ct ON ct.id = e.content_type_id
LEFT JOIN entry_revisions latest ON latest.entry_id = e.id AND latest.revision_number = (SELECT MAX(revision_number) FROM entry_revisions WHERE entry_id=e.id)
LEFT JOIN routes r ON r.entry_id = e.id AND r.route_type='entry'
WHERE e.status!='trash'
ORDER BY e.updated_at DESC, e.id DESC
LIMIT ?`
	rows, err := h.database.QueryContext(ctx, query, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []dashboardEntry
	for rows.Next() {
		var id, ctID, ctLabel, title, status, pubRev sql.NullString
		var reviewState sql.NullString
		var updatedAt int64
		var publicPath sql.NullString
		if err := rows.Scan(&id, &ctID, &ctLabel, &title, &status, &pubRev, &reviewState, &updatedAt, &publicPath); err != nil {
			continue
		}
		// Derive admin status display
		display, tone := deriveEntryStatus(status.String, pubRev.String, reviewState.String)
		ct := ctID.String
		label := ctLabel.String
		if label == "" {
			label = ct
		}
		entryTitle := title.String
		if entryTitle == "" {
			entryTitle = id.String
		}
		editURL := "/admin/pages/" + id.String + "/edit"
		if ct == "post" {
			editURL = "/admin/posts/" + id.String + "/edit"
		} else if ct != "page" && ct != "" {
			editURL = "/admin/content/" + ct + "/" + id.String + "/edit"
		}
		out = append(out, dashboardEntry{
			ID:               id.String,
			Title:            entryTitle,
			ContentTypeID:    ct,
			ContentTypeLabel: label,
			Status:           display,
			StatusTone:       tone,
			UpdatedAt:        time.Unix(updatedAt, 0),
			EditURL:          editURL,
		})
	}
	return out
}

func deriveEntryStatus(status, pubRev, reviewState string) (string, string) {
	if status == "trash" {
		return "Trash", "danger"
	}
	if pubRev != "" {
		return "Published", "success"
	}
	if reviewState == "pending" {
		return "Pending", "warning"
	}
	if reviewState == "draft" || reviewState == "" {
		return "Draft", "muted"
	}
	return "Draft", "muted"
}
