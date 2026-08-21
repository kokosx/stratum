package admin

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/themes"
)

type previewPage struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

type appearanceData struct {
	ThemeName        string
	ThemeVersion     int
	ThemeDescription string
	BootstrapJSON    template.JS
	PreviewPages     []previewPage
}

type appearanceBootstrap struct {
	Customization themes.Customization `json:"customization"`
	CSRFToken     string               `json:"csrfToken"`
	PreviewPath   string               `json:"previewPath"`
}

type appearanceRequest struct {
	Settings    map[string]any `json:"settings"`
	CustomCSS   string         `json:"customCSS"`
	PreviewPath string         `json:"previewPath"`
}

func (h *Handler) appearance(w http.ResponseWriter, r *http.Request) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	customization := h.themes.Current()
	bootstrap, err := json.Marshal(appearanceBootstrap{Customization: customization, CSRFToken: token, PreviewPath: "/"})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var previewPages []previewPage
	entries, err := h.queries.ListEntriesByContentType(r.Context(), "page")
	if err == nil {
		for _, entry := range entries {
			if entry.Status == "active" && entry.PublishedRevisionID.Valid && entry.PublicPath.Valid && entry.PublicPath.String != "" {
				title := "(untitled)"
				if entry.Title.Valid && entry.Title.String != "" {
					title = entry.Title.String
				}
				previewPages = append(previewPages, previewPage{Title: title, Path: entry.PublicPath.String})
			}
		}
	}

	data := appearanceData{
		ThemeName:        customization.Name,
		ThemeVersion:     customization.Version,
		ThemeDescription: customization.Description,
		BootstrapJSON:    template.JS(bootstrap),
		PreviewPages:     previewPages,
	}
	layout := LayoutData{Title: "Appearance", ActiveMenu: "appearance", CSRFToken: token, Content: data}
	if err := h.appearanceTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render appearance: %v", err)
	}
}

func (h *Handler) saveAppearance(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	request, ok := decodeAppearanceRequest(w, r)
	if !ok {
		return
	}
	if err := h.themes.Save(r.Context(), request.Settings, request.CustomCSS); err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "customization": h.themes.Current()})
}

func (h *Handler) previewAppearance(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	request, ok := decodeAppearanceRequest(w, r)
	if !ok {
		return
	}
	if h.previewRenderer == nil {
		http.Error(w, "Preview renderer is unavailable", http.StatusServiceUnavailable)
		return
	}
	validated, err := h.themes.ValidateSettings(request.Settings)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	path := strings.TrimSpace(request.PreviewPath)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\r\n") {
		writeJSONError(w, http.StatusBadRequest, errors.New("invalid preview path"))
		return
	}
	page, err := h.previewRenderer(r.Context(), path, requestOrigin(r), validated, request.CustomCSS)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func decodeAppearanceRequest(w http.ResponseWriter, r *http.Request) (appearanceRequest, bool) {
	var request appearanceRequest
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Settings == nil {
		if err == nil {
			err = errors.New("settings must be an object")
		}
		writeJSONError(w, http.StatusBadRequest, err)
		return appearanceRequest{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values are not allowed")
		}
		writeJSONError(w, http.StatusBadRequest, err)
		return appearanceRequest{}, false
	}
	if len(request.CustomCSS) > themes.MaxCustomCSSBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, errors.New("custom CSS is too large"))
		return appearanceRequest{}, false
	}
	return request, true
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
