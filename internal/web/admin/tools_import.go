package admin

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wordpress "github.com/kokosx/stratum/internal/importer/wordpress"
)

const maxWXRBytes = 256 << 20 // 256 MB

func (h *Handler) handleImportPage(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	users := h.listUsersForImport(r)
	data := LayoutData{
		Title:         "Import",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-import",
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content: struct {
			CSRFToken string
			Users     []importUserOption
		}{CSRFToken: token, Users: users},
	}
	if err := h.toolsImportTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render import: %v", err)
	}
}

type importUserOption struct {
	ID    string
	Email string
}

func (h *Handler) listUsersForImport(r *http.Request) []importUserOption {
	rows, err := h.database.QueryContext(r.Context(), `SELECT id, email FROM users ORDER BY email`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []importUserOption
	for rows.Next() {
		var id, email string
		if err := rows.Scan(&id, &email); err == nil {
			out = append(out, importUserOption{ID: email, Email: email})
		}
	}
	return out
}

func (h *Handler) handleImportAnalyze(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	// Bound request body
	r.Body = http.MaxBytesReader(w, r.Body, maxWXRBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "Upload too large (max 256 MB)", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Invalid upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("wxr_file")
	if err != nil {
		http.Error(w, "WordPress export (.xml) required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxWXRBytes {
		http.Error(w, "Upload too large (max 256 MB)", http.StatusRequestEntityTooLarge)
		return
	}
	downloadMedia := r.FormValue("download_media") != "false" && r.FormValue("download_media") != "0"
	// Checkbox default checked: if not present, treat as true
	if r.FormValue("download_media") == "" {
		downloadMedia = true
	}
	fallbackAuthor := strings.TrimSpace(r.FormValue("fallback_author"))

	// Validate extension is .xml (do not trust filename as path)
	filename := header.Filename
	if !strings.HasSuffix(strings.ToLower(filename), ".xml") {
		http.Error(w, "Only .xml files are allowed", http.StatusBadRequest)
		return
	}

	// Stream to temp file with restrictive perms
	tmp, err := os.CreateTemp("", "wxr-*.xml")
	if err != nil {
		http.Error(w, "Cannot create temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Chmod(0600)
	n, err := io.Copy(tmp, io.LimitReader(file, maxWXRBytes+1))
	tmp.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		http.Error(w, "Failed to save upload", http.StatusInternalServerError)
		return
	}
	if n > maxWXRBytes {
		_ = os.Remove(tmpPath)
		http.Error(w, "Upload too large (max 256 MB)", http.StatusRequestEntityTooLarge)
		return
	}

	// Run dry-run analysis via manager
	if h.importManager == nil {
		_ = os.Remove(tmpPath)
		http.Error(w, "Import manager not configured", http.StatusInternalServerError)
		return
	}
	job, err := h.importManager.Analyze(r.Context(), tmpPath, downloadMedia, fallbackAuthor)
	if err != nil {
		_ = os.Remove(tmpPath)
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusBadRequest)
		return
	}
	// Redirect to review
	http.Redirect(w, r, "/admin/tools/import/wordpress/"+job.ID+"/review", http.StatusSeeOther)
}

func (h *Handler) handleImportReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = r.PathValue("job")
	}
	job := h.importManager.Get(id)
	if job == nil {
		http.NotFound(w, r)
		return
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Review import",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-import",
		Nav:           h.navForUser(r),
		CSRFToken:     token,
		Content: struct {
			Job       *wordpress.Job
			CSRFToken string
		}{Job: job, CSRFToken: token},
	}
	if err := h.toolsImportReviewTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render import review: %v", err)
	}
}

func (h *Handler) handleImportStart(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		id = r.PathValue("job")
	}
	job, err := h.importManager.StartImport(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "already running") {
			if isDatastarRequest(r) {
				writeSSE(w, toastEvent("error", err.Error()))
				return
			}
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Import started"), patchElementsEvent("inner", "#import-status", `<div>Import in progress… Phase: `+template.HTMLEscapeString(job.Phase)+`</div>`))
		return
	}
	http.Redirect(w, r, "/admin/tools/import/wordpress/"+id+"/status", http.StatusSeeOther)
}

func (h *Handler) handleImportStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = r.PathValue("job")
	}
	job := h.importManager.Get(id)
	if job == nil {
		http.NotFound(w, r)
		return
	}
	// If Datastar request, send SSE fragment
	if isDatastarRequest(r) {
		html := renderImportStatusHTML(job, h)
		writeSSE(w, patchElementsEvent("inner", "#import-status", html))
		if job.Done {
			if job.Error != "" {
				writeSSE(w, toastEvent("error", "Import failed: "+job.Error))
			} else {
				writeSSE(w, toastEvent("success", "Import complete"))
			}
		}
		return
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Import progress",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-import",
		Nav:           h.navForUser(r),
		CSRFToken:     token,
		Content: struct {
			Job       *wordpress.Job
			CSRFToken string
		}{Job: job, CSRFToken: token},
	}
	if err := h.toolsImportStatusTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render import status: %v", err)
	}
}

func renderImportStatusHTML(job *wordpress.Job, h *Handler) string {
	if job == nil {
		return `<div>Job not found</div>`
	}
	if job.Done {
		if job.Error != "" {
			return `<div style="padding:12px;border:1px solid #fecaca;background:#fef2f2;"><strong>Import failed:</strong> ` + template.HTMLEscapeString(job.Error) + `</div>`
		}
		rep := job.Report
		if rep == nil {
			return `<div>Import complete</div>`
		}
		return fmt.Sprintf(`<div style="padding:12px;border:1px solid #bbf7d0;background:#f0fdf4;"><strong>Import complete</strong><div class="muted" style="margin-top:6px;">Posts: %d, Pages: %d, Media: %d, Categories: %d, Tags: %d, Warnings: %d</div><div style="margin-top:8px;"><a class="button button-small" href="/admin/posts">View posts</a> <a class="button button-small" href="/admin/pages">View pages</a> <a class="button button-small" href="/admin/media">View media</a> <a class="button button-small" href="/">View site</a></div></div>`, rep.Posts, rep.Pages, rep.MediaImported, rep.Categories, rep.Tags, rep.Warnings)
	}
	// In progress
	return `<div style="padding:12px;border:1px solid #e5e7eb;background:#fff;"><strong>Import in progress…</strong><div class="muted" style="margin-top:6px;">Phase: ` + template.HTMLEscapeString(job.Phase) + `</div><div class="muted" style="font-size:12px;margin-top:4px;">This may take a while. Do not close the browser.</div></div>`
}

func (h *Handler) handleImportComplete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = r.PathValue("job")
	}
	job := h.importManager.Get(id)
	if job == nil {
		http.NotFound(w, r)
		return
	}
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "WordPress import complete",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-import",
		Nav:           h.navForUser(r),
		CSRFToken:     token,
		Content: struct {
			Job       *wordpress.Job
			CSRFToken string
		}{Job: job, CSRFToken: token},
	}
	if err := h.toolsImportCompleteTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render import complete: %v", err)
	}
}

func (h *Handler) handleImportCancel(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	if err := h.importManager.Cancel(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/tools/import", http.StatusSeeOther)
}

// Helpers for backup path sanitization (not used here)
func safeJoin(base, name string) string {
	_ = strconv.Itoa(0)
	return filepath.Join(base, name)
}
