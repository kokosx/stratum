package admin

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
)

type EntriesData struct {
	Heading       string
	NewURL        string
	EditBase      string
	Entries       []EntryData
	Counts        content.EntryStatusCounts
	CurrentStatus string
	Search        string
	Page          int
	PerPage       int
	Total         int64
	TotalPages    int
	BasePath      string // e.g. /admin/posts
	QueryString   string // e.g. search=x&status=draft for pagination links (without page)
	CSRFToken     string
}

type EntryData struct {
	ID           string
	Title        string
	Slug         string
	Status       string
	UpdatedAt    string
	PublicURL    string
	RawStatus    string // active/private/trash + published check, for row actions
	HasPublished bool
	Depth        int
}

func (h *Handler) listPages(w http.ResponseWriter, r *http.Request) {
	h.listEntries(w, r, "page", "Pages", "pages")
}

func (h *Handler) listPosts(w http.ResponseWriter, r *http.Request) {
	h.listEntries(w, r, "post", "Posts", "posts")
}

func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request, contentType, heading, activeMenu string) {
	search := r.URL.Query().Get("search")
	if search == "" {
		search = r.URL.Query().Get("s")
	}
	statusRaw := r.URL.Query().Get("status")
	status := content.NormalizeAdminStatus(statusRaw)
	// Empty means all (already handled). If query had unknown value, Normalize returns all.
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	repo := content.NewRepository(h.queries)
	q := content.EntryAdminListQuery{
		ContentType: content.ContentTypeID(contentType),
		Search:      search,
		Status:      status,
		Page:        page,
		PerPage:     perPage,
	}
	user, userErr := h.currentUser(r)
	if userErr == nil && contentType == postContentType && !authz.Allows(user.Role, authz.EditAnyEntry) {
		q.AuthorID = user.ID
	}
	q = q.Normalized()
	result, err := repo.AdminList(r.Context(), q)
	if err != nil {
		log.Printf("list admin %s: %v", contentType, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	items := make([]EntryData, 0, len(result.Entries))
	depths := map[string]int{}
	isHierarchical := false
	if def, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), contentType); err == nil {
		isHierarchical = def.Capabilities.Hierarchical
	} else {
		isHierarchical = content.DefinitionFor(contentType).Capabilities.Hierarchical
	}
	if isHierarchical {
		if rows, hierarchyErr := h.queries.ListLatestHierarchyForContentType(r.Context(), contentType); hierarchyErr == nil {
			nodes := make([]content.HierarchyNode, 0, len(rows))
			for _, row := range rows {
				parent := ""
				if row.ParentEntryID.Valid {
					parent = row.ParentEntryID.String
				}
				nodes = append(nodes, content.HierarchyNode{EntryID: row.EntryID, ParentEntryID: parent, MenuOrder: row.MenuOrder, Title: row.Title})
			}
			if hierarchy, hierarchyErr := content.NewHierarchy(nodes); hierarchyErr == nil {
				for _, node := range nodes {
					depths[node.EntryID] = hierarchy.Depth(node.EntryID)
				}
			}
		}
	}
	for _, entry := range result.Entries {
		title := "(untitled)"
		if entry.Title != "" {
			title = entry.Title
		}
		// Private entries have no public route; never show View link.
		publicURL := ""
		if entry.Status != "trash" && entry.PublishedRevisionID != "" && entry.PublishedVisibility != "private" {
			publicURL = entry.PublicPath
		}
		items = append(items, EntryData{
			ID:           entry.ID,
			Title:        title,
			Slug:         entry.Slug,
			Status:       adminRowStatus(entry),
			UpdatedAt:    time.Unix(entry.UpdatedAt, 0).Format("2 Jan 2006, 15:04"),
			PublicURL:    publicURL,
			RawStatus:    entry.Status,
			HasPublished: entry.PublishedRevisionID != "",
			Depth:        depths[entry.ID],
		})
	}
	totalPages := int((result.Total + int64(q.PerPage) - 1) / int64(q.PerPage))
	if totalPages < 1 {
		totalPages = 1
	}
	q.Page = result.Page
	basePath := "/admin/" + activeMenu
	qs := buildEntriesQueryString(search, string(status))
	state := ResolveNav(r.URL.Path)
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := LayoutData{
		Title:         heading,
		ActiveMenu:    activeMenu,
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content: EntriesData{
			Heading:       heading,
			NewURL:        basePath + "/new",
			EditBase:      basePath,
			Entries:       items,
			Counts:        result.Counts,
			CurrentStatus: string(q.Status),
			Search:        search,
			Page:          q.Page,
			PerPage:       q.PerPage,
			Total:         result.Total,
			TotalPages:    totalPages,
			BasePath:      basePath,
			QueryString:   qs,
			CSRFToken:     token,
		},
	}
	if err := h.entriesTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render admin %s: %v", contentType, err)
	}
}

func buildEntriesQueryString(search, status string) string {
	v := url.Values{}
	if search != "" {
		v.Set("search", search)
	}
	if status != "" && status != "all" {
		v.Set("status", status)
	}
	if len(v) == 0 {
		return ""
	}
	return v.Encode()
}

func entryStatus(status string, hasPublishedRevision bool) string {
	switch status {
	case "private":
		return "Private"
	case "trash":
		return "Trash"
	}
	if hasPublishedRevision {
		return "Published"
	}
	return "Draft"
}

func adminRowStatus(e content.AdminEntry) string {
	if e.Status == "trash" {
		return "Trash"
	}
	if e.HasSchedule {
		when := time.Unix(e.ScheduledAt, 0).Format("2 Jan 15:04")
		return "Scheduled · " + when
	}
	if e.PublishedVisibility == "private" {
		if e.LatestReviewState == "pending" && e.LatestRevisionID != "" && e.LatestRevisionID != e.PublishedRevisionID {
			return "Private · Pending Review"
		}
		if e.LatestRevisionID != "" && e.LatestRevisionID != e.PublishedRevisionID {
			return "Private · Unpublished changes"
		}
		return "Private"
	}
	if e.PublishedVisibility == "password" {
		if e.LatestReviewState == "pending" && e.LatestRevisionID != "" && e.LatestRevisionID != e.PublishedRevisionID {
			return "Password Protected · Pending Review"
		}
		if e.LatestRevisionID != "" && e.LatestRevisionID != e.PublishedRevisionID {
			return "Password Protected · Unpublished changes"
		}
		return "Password Protected"
	}
	if e.PublishedRevisionID != "" {
		if e.LatestReviewState == "pending" && e.LatestRevisionID != e.PublishedRevisionID {
			return "Published · Pending Review"
		}
		if e.LatestRevisionID != "" && e.LatestRevisionID != e.PublishedRevisionID {
			return "Published · Unpublished changes"
		}
		return "Published"
	}
	if e.LatestReviewState == "pending" {
		return "Pending Review"
	}
	return "Draft"
}

func stringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// Trash/restore/delete handlers shared by pages/posts.

func (h *Handler) trashPage(w http.ResponseWriter, r *http.Request) {
	h.trashEntry(w, r, "page", "/admin/pages")
}
func (h *Handler) trashPost(w http.ResponseWriter, r *http.Request) {
	h.trashEntry(w, r, "post", "/admin/posts")
}
func (h *Handler) restorePage(w http.ResponseWriter, r *http.Request) {
	h.restoreEntry(w, r, "page", "/admin/pages")
}
func (h *Handler) restorePost(w http.ResponseWriter, r *http.Request) {
	h.restoreEntry(w, r, "post", "/admin/posts")
}
func (h *Handler) deletePagePermanently(w http.ResponseWriter, r *http.Request) {
	h.deleteEntryPermanently(w, r, "page", "/admin/pages")
}
func (h *Handler) deletePostPermanently(w http.ResponseWriter, r *http.Request) {
	h.deleteEntryPermanently(w, r, "post", "/admin/posts")
}
func (h *Handler) bulkPages(w http.ResponseWriter, r *http.Request) {
	h.bulkEntries(w, r, "page", "/admin/pages")
}
func (h *Handler) bulkPosts(w http.ResponseWriter, r *http.Request) {
	h.bulkEntries(w, r, "post", "/admin/posts")
}

func (h *Handler) trashEntry(w http.ResponseWriter, r *http.Request, contentType, listingURL string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if !h.entryHasContentType(r.Context(), id, contentType) {
		http.NotFound(w, r)
		return
	}
	svc := content.NewLifecycleService(h.database, h.queries)
	err := svc.MoveToTrash(r.Context(), id)
	if err != nil {
		if errors.Is(err, content.ErrProtectedPage) {
			h.setFlash(w, "Cannot trash: this page is configured as Homepage or Posts Page. Change Site Settings first.")
			http.Redirect(w, r, listingURL, http.StatusSeeOther)
			return
		}
		if errors.Is(err, content.ErrAlreadyTrashed) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("trash entry %s: %v", id, err)
		http.Error(w, "Could not trash entry", http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Moved to Trash.")
	http.Redirect(w, r, listingURL+"?status=trash", http.StatusSeeOther)
}

func (h *Handler) restoreEntry(w http.ResponseWriter, r *http.Request, contentType, listingURL string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if !h.entryHasContentType(r.Context(), id, contentType) {
		http.NotFound(w, r)
		return
	}
	svc := content.NewLifecycleService(h.database, h.queries)
	err := svc.Restore(r.Context(), id)
	if err != nil {
		if errors.Is(err, content.ErrNotTrashed) || errors.Is(err, content.ErrRouteOccupied) {
			h.setFlash(w, err.Error())
			http.Redirect(w, r, listingURL+"?status=trash", http.StatusSeeOther)
			return
		}
		log.Printf("restore entry %s: %v", id, err)
		http.Error(w, "Could not restore entry", http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Entry restored.")
	http.Redirect(w, r, listingURL, http.StatusSeeOther)
}

func (h *Handler) deleteEntryPermanently(w http.ResponseWriter, r *http.Request, contentType, listingURL string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if !h.entryHasContentType(r.Context(), id, contentType) {
		http.NotFound(w, r)
		return
	}
	svc := content.NewLifecycleService(h.database, h.queries)
	err := svc.DeletePermanently(r.Context(), id)
	if err != nil {
		if errors.Is(err, content.ErrProtectedPage) || errors.Is(err, content.ErrPermanentDeleteOnlyTrash) {
			h.setFlash(w, err.Error())
			http.Redirect(w, r, listingURL+"?status=trash", http.StatusSeeOther)
			return
		}
		log.Printf("delete permanently %s: %v", id, err)
		http.Error(w, "Could not delete entry", http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Entry permanently deleted.")
	http.Redirect(w, r, listingURL+"?status=trash", http.StatusSeeOther)
}

func (h *Handler) entryHasContentType(ctx context.Context, id, contentType string) bool {
	entry, err := h.queries.GetEntry(ctx, id)
	return err == nil && entry.ContentTypeID == contentType
}

func (h *Handler) bulkEntries(w http.ResponseWriter, r *http.Request, contentType, listingURL string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	action := r.FormValue("bulk_action")
	ids := r.Form["ids"]
	if len(ids) == 0 {
		// WordPress shows no-op when nothing selected; redirect back.
		h.setFlash(w, "No entries selected.")
		http.Redirect(w, r, listingURL, http.StatusSeeOther)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	for _, id := range ids {
		entry, err := h.queries.GetEntry(r.Context(), id)
		if err != nil || entry.ContentTypeID != contentType || !authz.CanAccessEntry(user.Role, user.ID, entry.AuthorID.String, contentType, authz.EntryDelete) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}
	svc := content.NewLifecycleService(h.database, h.queries)
	switch action {
	case "trash":
		err = svc.BulkTrash(r.Context(), contentType, ids)
	case "restore":
		err = svc.BulkRestore(r.Context(), contentType, ids)
	case "delete":
		err = svc.BulkDeletePermanently(r.Context(), contentType, ids)
	default:
		http.Error(w, "Unknown bulk action", http.StatusBadRequest)
		return
	}
	if err != nil {
		// Protected page or validation error causes no partial destructive operation.
		h.setFlash(w, err.Error())
		http.Redirect(w, r, listingURL, http.StatusSeeOther)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	switch action {
	case "trash":
		h.setFlash(w, "Selected entries moved to Trash.")
	case "restore":
		h.setFlash(w, "Selected entries restored.")
	case "delete":
		h.setFlash(w, "Selected entries permanently deleted.")
	}
	// Redirect to appropriate tab.
	if action == "trash" {
		http.Redirect(w, r, listingURL+"?status=trash", http.StatusSeeOther)
	} else if action == "restore" || action == "delete" {
		http.Redirect(w, r, listingURL+"?status=trash", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, listingURL, http.StatusSeeOther)
	}
}
