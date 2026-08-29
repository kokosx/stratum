package admin

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

// revisionHistoryData is presentation-only for the shared revisions.html view.
// Templates and Site Parts share this; no new domain abstraction is introduced.
type revisionHistoryData struct {
	Heading    string
	BackURL    string
	EntityName string
	EntityKind string
	Revisions  []revisionHistoryRow
	CSRFToken  string
	Flash      string
}

type revisionHistoryRow struct {
	ID         string
	Number     int64
	CreatedAt  string
	Author     string
	Status     string // "Published" | "Current draft" | ""
	PreviewURL string
	RestoreURL string
	CanRestore bool
}

func (h *Handler) renderRevisions(w http.ResponseWriter, r *http.Request, data revisionHistoryData) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.revisionsTemplate.ExecuteTemplate(w, "layout.html", LayoutData{
		Title:         data.Heading + " — " + data.EntityName,
		ActiveMenu:    ResolveNav(r.URL.Path).ActiveSection,
		ActiveSection: ResolveNav(r.URL.Path).ActiveSection,
		ActiveItem:    ResolveNav(r.URL.Path).ActiveItem,
		Nav:           h.navForUser(r),
		CSRFToken:     data.CSRFToken,
		Flash:         data.Flash,
		Content:       data,
	}); err != nil {
		log.Printf("render revisions: %v", err)
	}
}

func formatRevisionTime(unix int64) string {
	return time.Unix(unix, 0).Format("2 Jan 2006, 15:04")
}

var _ = template.HTMLEscapeString
