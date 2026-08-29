package admin

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/health"
	"github.com/kokosx/stratum/internal/notfound"
	"github.com/kokosx/stratum/internal/redirects"
)

// Tools data structs

type redirectsData struct {
	Redirects []redirectRow
	CSRFToken string
	Flash     string
	Error     string
}

type redirectRow struct {
	ID        string
	Source    string
	Target    string
	Status    int64
	UpdatedAt string
}

type redirectFormData struct {
	ID        string
	Source    string
	Target    string
	Status    int64
	CSRFToken string
	Error     string
	BackURL   string
	IsEdit    bool
}

type notFoundData struct {
	Records   []notFoundRow
	CSRFToken string
	Flash     string
	Total     int64
}

type notFoundRow struct {
	Path      string
	Hits      int64
	LastSeen  string
	FirstSeen string
	Age       string
}

type siteHealthData struct {
	Results   []health.CheckResult
	Issues    []health.IntegrityIssue
	CSRFToken string
	Flash     string
	Generated string
}

func (h *Handler) toolsSiteHealth(w http.ResponseWriter, r *http.Request) {
	svc := health.New(h.database, h.queries)
	results, issues, err := svc.Run(r.Context())
	if err != nil {
		http.Error(w, "Health check failed", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Site Health",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-site-health",
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content: siteHealthData{
			Results:   results,
			Issues:    issues,
			CSRFToken: token,
			Generated: time.Now().Format("2 Jan 2006, 15:04"),
		},
	}
	if err := h.toolsHealthTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render site health: %v", err)
	}
}

func (h *Handler) toolsRedirectsList(w http.ResponseWriter, r *http.Request) {
	svc := redirects.New(h.database, h.queries)
	rows, err := svc.List(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	dataRows := make([]redirectRow, 0, len(rows))
	for _, row := range rows {
		status := int64(301)
		if row.RedirectStatus.Valid {
			status = row.RedirectStatus.Int64
		}
		target := ""
		if row.RedirectTo.Valid {
			target = row.RedirectTo.String
		}
		dataRows = append(dataRows, redirectRow{
			ID: row.ID, Source: row.Path, Target: target, Status: status,
			UpdatedAt: time.Unix(row.UpdatedAt, 0).Format("2 Jan 2006, 15:04"),
		})
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Redirects",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-redirects",
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content:       redirectsData{Redirects: dataRows, CSRFToken: token},
	}
	if err := h.toolsRedirectsTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render redirects: %v", err)
	}
}

func (h *Handler) toolsRedirectsNew(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	prefill := r.URL.Query().Get("source")
	data := LayoutData{
		Title:         "Add redirect",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-redirects",
		Nav:           h.navForUser(r),
		CSRFToken:     token,
		Content: redirectFormData{
			Source: prefill, Status: 301, CSRFToken: token, BackURL: "/admin/tools/redirects",
		},
	}
	_ = h.toolsRedirectFormTemplate.ExecuteTemplate(w, "layout.html", data)
}

func (h *Handler) toolsRedirectsCreate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	source := strings.TrimSpace(r.FormValue("source"))
	target := strings.TrimSpace(r.FormValue("destination"))
	if target == "" {
		target = strings.TrimSpace(r.FormValue("target"))
	}
	statusStr := r.FormValue("status")
	status := int64(301)
	if statusStr == "302" {
		status = 302
	}
	svc := redirects.New(h.database, h.queries)
	_, err := svc.Create(r.Context(), source, target, int(status), time.Now().Unix())
	if err != nil {
		token, _ := h.csrfToken(w, r)
		data := LayoutData{
			Title:         "Add redirect",
			ActiveMenu:    "tools",
			ActiveSection: "tools",
			ActiveItem:    "tools-redirects",
			Nav:           h.navForUser(r),
			CSRFToken:     token,
			Content: redirectFormData{
				Source: source, Target: target, Status: status, CSRFToken: token, Error: err.Error(), BackURL: "/admin/tools/redirects",
			},
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = h.toolsRedirectFormTemplate.ExecuteTemplate(w, "layout.html", data)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
		h.runtime.InvalidateContent()
	}
	// If created from 404, remove the notfound record
	if source != "" {
		_ = notfound.New(h.database).Delete(r.Context(), source)
	}
	h.setFlash(w, "Redirect created.")
	http.Redirect(w, r, "/admin/tools/redirects", http.StatusSeeOther)
}

func (h *Handler) toolsRedirectsEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	svc := redirects.New(h.database, h.queries)
	route, err := svc.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status := int64(301)
	if route.RedirectStatus.Valid {
		status = route.RedirectStatus.Int64
	}
	target := ""
	if route.RedirectTo.Valid {
		target = route.RedirectTo.String
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Edit redirect",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-redirects",
		Nav:           h.navForUser(r),
		CSRFToken:     token,
		Content: redirectFormData{
			ID: id, Source: route.Path, Target: target, Status: status, CSRFToken: token, BackURL: "/admin/tools/redirects", IsEdit: true,
		},
	}
	_ = h.toolsRedirectFormTemplate.ExecuteTemplate(w, "layout.html", data)
}

func (h *Handler) toolsRedirectsUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	source := strings.TrimSpace(r.FormValue("source"))
	target := strings.TrimSpace(r.FormValue("destination"))
	if target == "" {
		target = strings.TrimSpace(r.FormValue("target"))
	}
	statusStr := r.FormValue("status")
	status := int64(301)
	if statusStr == "302" {
		status = 302
	}
	svc := redirects.New(h.database, h.queries)
	_, err := svc.Update(r.Context(), id, source, target, int(status), time.Now().Unix())
	if err != nil {
		token, _ := h.csrfToken(w, r)
		data := LayoutData{
			Title:         "Edit redirect",
			ActiveMenu:    "tools",
			ActiveSection: "tools",
			ActiveItem:    "tools-redirects",
			Nav:           h.navForUser(r),
			CSRFToken:     token,
			Content: redirectFormData{
				ID: id, Source: source, Target: target, Status: status, CSRFToken: token, Error: err.Error(), BackURL: "/admin/tools/redirects", IsEdit: true,
			},
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = h.toolsRedirectFormTemplate.ExecuteTemplate(w, "layout.html", data)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
		h.runtime.InvalidateContent()
	}
	h.setFlash(w, "Redirect updated.")
	http.Redirect(w, r, "/admin/tools/redirects", http.StatusSeeOther)
}

func (h *Handler) toolsRedirectsDelete(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	svc := redirects.New(h.database, h.queries)
	if err := svc.Delete(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
		h.runtime.InvalidateContent()
	}
	h.setFlash(w, "Redirect deleted.")
	http.Redirect(w, r, "/admin/tools/redirects", http.StatusSeeOther)
}

func (h *Handler) toolsNotFoundList(w http.ResponseWriter, r *http.Request) {
	store := notfound.New(h.database)
	records, err := store.List(r.Context(), 100)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	rows := make([]notFoundRow, 0, len(records))
	for _, rec := range records {
		rows = append(rows, notFoundRow{
			Path: rec.Path, Hits: rec.HitCount,
			LastSeen:  time.Unix(rec.LastSeenAt, 0).Format("2 Jan 2006, 15:04"),
			FirstSeen: time.Unix(rec.FirstSeenAt, 0).Format("2 Jan 2006, 15:04"),
			Age:       timeAgo(time.Unix(rec.LastSeenAt, 0)),
		})
	}
	var total int64
	if c, err := store.Count(r.Context()); err == nil {
		total = c
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Not Found",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-not-found",
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content:       notFoundData{Records: rows, CSRFToken: token, Total: total},
	}
	_ = h.toolsNotFoundTemplate.ExecuteTemplate(w, "layout.html", data)
}

func (h *Handler) toolsNotFoundDelete(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	path := r.FormValue("path")
	store := notfound.New(h.database)
	if path == "" {
		// Clear all?
		_ = store.ClearAll(r.Context())
		h.setFlash(w, "All not-found records cleared.")
	} else {
		_ = store.Delete(r.Context(), path)
		h.setFlash(w, "Record deleted.")
	}
	http.Redirect(w, r, "/admin/tools/not-found", http.StatusSeeOther)
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return strconv.Itoa(int(d.Minutes())) + " minutes ago"
	}
	if d < 24*time.Hour {
		return strconv.Itoa(int(d.Hours())) + " hours ago"
	}
	if d < 30*24*time.Hour {
		return strconv.Itoa(int(d.Hours()/24)) + " days ago"
	}
	return t.Format("2 Jan 2006")
}

// Ensure tools templates exist
var _ = template.New
