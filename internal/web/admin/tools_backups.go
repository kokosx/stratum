package admin

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/backup"
	"github.com/kokosx/stratum/internal/storage"
)

type backupRow struct {
	Name       string
	Created    string
	CreatedRaw time.Time
	Size       string
	SizeBytes  int64
}

type backupsData struct {
	Backups   []backupRow
	CSRFToken string
	Flash     string
	Error     string
}

func (h *Handler) handleBackupsList(w http.ResponseWriter, r *http.Request) {
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	backupDir := filepath.Join(dataDir, "backups")
	entries, _ := os.ReadDir(backupDir)
	var rows []backupRow
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".zip") {
			continue
		}
		if !strings.HasPrefix(name, "stratum-backup-") && !strings.HasPrefix(name, "pre-import-") && !strings.HasPrefix(name, "pre-restore-") {
			// still show if .zip but prefer those
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		rows = append(rows, backupRow{
			Name:       name,
			Created:    info.ModTime().Format("2 Jan 2006, 15:04"),
			CreatedRaw: info.ModTime(),
			Size:       formatBytes(info.Size()),
			SizeBytes:  info.Size(),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedRaw.After(rows[j].CreatedRaw) })
	token, _ := h.csrfToken(w, r)
	data := LayoutData{
		Title:         "Backups",
		ActiveMenu:    "tools",
		ActiveSection: "tools",
		ActiveItem:    "tools-backups",
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content: backupsData{
			Backups:   rows,
			CSRFToken: token,
		},
	}
	if err := h.toolsBackupsTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render backups: %v", err)
	}
}

func (h *Handler) handleBackupsCreate(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	// Ensure backup dir exists
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Generate safe filename
	ts := time.Now().UTC().Format("20060102-150405")
	outputPath := filepath.Join(backupDir, fmt.Sprintf("stratum-backup-%s.zip", ts))
	// Ensure not exists, add suffix if needed
	for i := 1; i < 100; i++ {
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			break
		}
		outputPath = filepath.Join(backupDir, fmt.Sprintf("stratum-backup-%s-%d.zip", ts, i))
	}
	// Use storage.Database wrapper
	dbWrapper := &storage.Database{DB: h.database}
	result, err := backup.CreateResult(r.Context(), dbWrapper, h.queries, dataDir, outputPath)
	if err != nil {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", fmt.Sprintf("Backup failed: %v", err)))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", fmt.Sprintf("Backup created: %s", filepath.Base(result.Path))))
		return
	}
	h.setFlash(w, fmt.Sprintf("Backup created: %s", filepath.Base(result.Path)))
	http.Redirect(w, r, "/admin/tools/backups", http.StatusSeeOther)
}

func (h *Handler) handleBackupsDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	// Validate name is base only, no path traversal
	if name != filepath.Base(name) || strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.Error(w, "Invalid backup name", http.StatusBadRequest)
		return
	}
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	backupDir := filepath.Join(dataDir, "backups")
	cleanPath := filepath.Join(backupDir, name)
	// Ensure cleaned path is still under backupDir (prevent symlink escape where applicable)
	absBackupDir, _ := filepath.Abs(backupDir)
	absPath, _ := filepath.Abs(cleanPath)
	rel, err := filepath.Rel(absBackupDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." && name == "" {
		http.Error(w, "Invalid backup path", http.StatusBadRequest)
		return
	}
	// Check file exists and is regular
	info, err := os.Stat(cleanPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !info.Mode().IsRegular() {
		http.Error(w, "Not a regular file", http.StatusBadRequest)
		return
	}
	// Also check symlink
	if fi, err := os.Lstat(cleanPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "Symlinks not allowed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, template.JSEscapeString(name)))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	http.ServeFile(w, r, cleanPath)
}

func (h *Handler) handleBackupsVerify(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if name != filepath.Base(name) || strings.Contains(name, "..") {
		http.Error(w, "Invalid backup name", http.StatusBadRequest)
		return
	}
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	backupDir := filepath.Join(dataDir, "backups")
	cleanPath := filepath.Join(backupDir, name)
	absBackupDir, _ := filepath.Abs(backupDir)
	absPath, _ := filepath.Abs(cleanPath)
	rel, err := filepath.Rel(absBackupDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "Invalid backup path", http.StatusBadRequest)
		return
	}
	if err := backup.Verify(cleanPath); err != nil {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", fmt.Sprintf("Backup verification failed: %v", err)))
			return
		}
		http.Error(w, fmt.Sprintf("Backup verification failed: %v", err), http.StatusBadRequest)
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Backup verified successfully"))
		return
	}
	h.setFlash(w, "Backup verified successfully")
	http.Redirect(w, r, "/admin/tools/backups", http.StatusSeeOther)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
