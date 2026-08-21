package admin

import (
	"database/sql"
	"log"
	"net/http"
	"time"
)

type EntriesData struct {
	Heading string
	Entries []EntryData
}

type EntryData struct {
	ID        string
	Title     string
	Slug      string
	Status    string
	UpdatedAt string
	PublicURL string
}

func (h *Handler) listPages(w http.ResponseWriter, r *http.Request) {
	h.listEntries(w, r, "page", "Pages", "pages")
}

func (h *Handler) listPosts(w http.ResponseWriter, r *http.Request) {
	h.listEntries(w, r, "post", "Posts", "posts")
}

func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request, contentType, heading, activeMenu string) {
	entries, err := h.queries.ListEntriesByContentType(r.Context(), contentType)
	if err != nil {
		log.Printf("list admin %s: %v", contentType, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	items := make([]EntryData, 0, len(entries))
	for _, entry := range entries {
		title := "(untitled)"
		if entry.Title.Valid && entry.Title.String != "" {
			title = entry.Title.String
		}
		publicURL := ""
		if entry.Status == "active" && entry.PublishedRevisionID.Valid {
			publicURL = stringValue(entry.PublicPath)
		}
		items = append(items, EntryData{
			ID:        entry.ID,
			Title:     title,
			Slug:      entry.Slug,
			Status:    entry.Status,
			UpdatedAt: time.Unix(entry.UpdatedAt, 0).Format("2 Jan 2006, 15:04"),
			PublicURL: publicURL,
		})
	}

	data := LayoutData{
		Title:      heading,
		ActiveMenu: activeMenu,
		Content: EntriesData{
			Heading: heading,
			Entries: items,
		},
	}
	if err := h.entriesTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render admin %s: %v", contentType, err)
	}
}

func stringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
