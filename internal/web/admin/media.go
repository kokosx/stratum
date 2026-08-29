package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/media"
)

// Upload bounds: per-file and aggregate. Aggregate must accommodate multiple files
// each up to per-file limit plus multipart overhead.
const (
	maxMediaImageBytes       = 10 << 20 // per file, mirrors media.maxImageBytes
	maxMediaFilesPerUpload   = 20
	maxMediaMultiUploadBytes = maxMediaImageBytes*maxMediaFilesPerUpload + (5 << 20)
	maxMediaParseMemory      = 32 << 20
)

// mediaJSON is the compact shape the Media Picker and Library grid consume.
type mediaJSON struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	Original    string `json:"original"`
	SocialURL   string `json:"socialUrl"`
	Mime        string `json:"mime"`
	Size        int64  `json:"size"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Alt         string `json:"alt"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	Description string `json:"description"`
}

type variantJSON struct {
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
}

type mediaDetailJSON struct {
	Asset      mediaJSON        `json:"asset"`
	Variants   []variantJSON    `json:"variants"`
	Usage      int64            `json:"usage"`
	UsageRefs  []media.UsageRef `json:"usageRefs"`
	UsageError string           `json:"usageError,omitempty"`
}

func toMediaJSON(a *media.Asset) mediaJSON {
	return mediaJSON{
		ID:          a.ID,
		Filename:    a.OriginalFilename,
		URL:         a.ThumbURL(),
		Original:    "/media/" + a.ID + "/original",
		SocialURL:   "/media/" + a.ID + "/social",
		Mime:        a.MimeType,
		Size:        a.FileSize,
		Width:       a.Width,
		Height:      a.Height,
		Alt:         a.AltText,
		Title:       a.Title,
		Caption:     a.Caption,
		Description: a.Description,
	}
}

// mediaLibrary renders the central Media Library page with search, filters, pagination.
func (h *Handler) mediaLibrary(w http.ResponseWriter, r *http.Request) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}
	pageStr := r.URL.Query().Get("page")
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	const perPage = 40
	offset := (page - 1) * perPage

	assets, total, err := h.media.ListFiltered(r.Context(), media.ListParams{Search: search, Filter: filter, Limit: perPage, Offset: offset})
	if err != nil {
		log.Printf("list media: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	cards := make([]mediaJSON, 0, len(assets))
	for i := range assets {
		cards = append(cards, toMediaJSON(&assets[i]))
	}
	totalPages := int((total + perPage - 1) / perPage)
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	data := mediaLibraryData{
		Cards:      cards,
		CSRFToken:  token,
		Search:     search,
		Filter:     filter,
		Page:       page,
		TotalPages: totalPages,
		Total:      total,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}
	state := ResolveNav(r.URL.Path)
	layout := LayoutData{Title: "Media", ActiveMenu: state.ActiveSection, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: h.navForUser(r), Flash: h.consumeFlash(w, r), CSRFToken: token, Content: data}
	if err := h.mediaTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render media library: %v", err)
	}
}

type mediaLibraryData struct {
	Cards      []mediaJSON
	CSRFToken  string
	Search     string
	Filter     string
	Page       int
	TotalPages int
	Total      int64
	HasPrev    bool
	HasNext    bool
}

// mediaListJSON returns the assets for the Media Picker without a full page load.
// It shares the same query semantics as the library (search + pagination).
func (h *Handler) mediaListJSON(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if search == "" {
		search = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	// Picker does not expose "unused" filter; ignore if passed
	if filter == "unused" {
		filter = "all"
	}
	pageStr := r.URL.Query().Get("page")
	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	const perPage = 40
	offset := (page - 1) * perPage
	limit := perPage
	// Allow picker to request larger limit via query param? Keep 40.
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 100 {
			limit = l
			// recalc offset if needed
			offset = (page - 1) * limit
		}
	}
	assets, total, err := h.media.ListFiltered(r.Context(), media.ListParams{Search: search, Filter: filter, Limit: limit, Offset: offset})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	cards := make([]mediaJSON, 0, len(assets))
	for i := range assets {
		cards = append(cards, toMediaJSON(&assets[i]))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": cards, "total": total, "page": page, "perPage": limit})
}

// uploadMedia handles a multipart image upload and returns JSON for the picker or
// library. It supports multiple files. Both the Media Library and picker share this
// single validation pipeline so errors are consistent.
func (h *Handler) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSONError(w, http.StatusForbidden, errors.New("invalid security token"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaMultiUploadBytes)
	if err := r.ParseMultipartForm(maxMediaParseMemory); err != nil {
		writeJSONError(w, http.StatusBadRequest, errors.New("upload too large or malformed: ensure each file is under 10 MB and total request is within limits"))
		return
	}
	mf := r.MultipartForm
	if mf == nil || len(mf.File) == 0 {
		writeJSONError(w, http.StatusBadRequest, errors.New("no file provided"))
		return
	}
	// Enforce file-count bound to prevent 1000-file abuse even if Content-Length lies
	totalFiles := 0
	for _, hs := range mf.File {
		totalFiles += len(hs)
	}
	if totalFiles > maxMediaFilesPerUpload {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("too many files: maximum %d per upload", maxMediaFilesPerUpload))
		return
	}
	if totalFiles == 0 {
		writeJSONError(w, http.StatusBadRequest, errors.New("no file provided"))
		return
	}
	user, _ := h.currentUser(r)
	var successes []mediaJSON
	var failures []map[string]string
	for _, hs := range mf.File {
		for _, header := range hs {
			if header.Filename == "" {
				continue
			}
			f, err := header.Open()
			if err != nil {
				failures = append(failures, map[string]string{"filename": header.Filename, "error": err.Error()})
				continue
			}
			asset, err := h.media.Upload(r.Context(), header.Filename, user.ID, f)
			f.Close()
			if err != nil {
				msg := err.Error()
				if errors.Is(err, media.ErrTooLarge) || errors.Is(err, media.ErrUnsupportedFormat) || errors.Is(err, media.ErrMalformed) || errors.Is(err, media.ErrInvalidImage) || errors.Is(err, media.ErrDimensionsTooLarge) || errors.Is(err, media.ErrTooManyPixels) {
					// keep msg as is
				} else {
					log.Printf("upload media %s: %v", header.Filename, err)
				}
				failures = append(failures, map[string]string{"filename": header.Filename, "error": msg})
				continue
			}
			if h.runtime != nil {
				h.runtime.InvalidateMedia(asset.ID)
			}
			successes = append(successes, toMediaJSON(asset))
		}
	}
	if len(successes) == 0 && len(failures) == 0 {
		writeJSONError(w, http.StatusBadRequest, errors.New("no file provided"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if len(failures) > 0 && len(successes) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "errors": failures})
		return
	}
	if len(failures) > 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "assets": successes, "uploaded": len(successes), "failed": len(failures), "errors": failures})
		return
	}
	if len(successes) == 1 {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "asset": successes[0], "assets": successes})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "assets": successes, "uploaded": len(successes)})
}

// mediaDetailJSON returns one asset with its variants and usage refs.
func (h *Handler) mediaDetailJSON(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	asset, err := h.media.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	usageRefs, err := h.media.UsageRefs(r.Context(), id)
	if err != nil {
		log.Printf("media usageRefs %s: %v", id, err)
		// Fail-safe: do not claim 0 usages when we could not determine them.
		// Delete remains unavailable until usage can be confirmed.
		detail := mediaDetailJSON{
			Asset:      toMediaJSON(asset),
			Usage:      -1,
			UsageRefs:  nil,
			UsageError: "Could not load usage information.",
		}
		for _, v := range asset.Variants {
			detail.Variants = append(detail.Variants, variantJSON{
				Kind: v.Kind, URL: "/media/" + id + "/" + v.Kind, Width: v.Width, Height: v.Height, Size: v.FileSize,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
		return
	}
	usage := int64(len(usageRefs))
	detail := mediaDetailJSON{Asset: toMediaJSON(asset), Usage: usage, UsageRefs: usageRefs}
	for _, v := range asset.Variants {
		detail.Variants = append(detail.Variants, variantJSON{
			Kind:   v.Kind,
			URL:    "/media/" + id + "/" + v.Kind,
			Width:  v.Width,
			Height: v.Height,
			Size:   v.FileSize,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

// updateMedia edits an asset's metadata. Datastar requests get a fragment patch
// and toast; others get JSON.
func (h *Handler) updateMedia(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		writeJSONError(w, http.StatusForbidden, errors.New("invalid security token"))
		return
	}
	id := r.PathValue("id")
	_ = r.ParseForm()
	if err := h.media.UpdateMetadata(r.Context(), id,
		r.FormValue("alt_text"),
		r.FormValue("title"),
		r.FormValue("caption"),
		r.FormValue("description"),
	); err != nil {
		log.Printf("update media: %v", err)
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Could not save changes"))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateMedia(id)
		h.runtime.InvalidateContent()
	}
	if isDatastarRequest(r) {
		writeSSE(w,
			patchElementsEvent("inner", "#media-detail-alt", template.HTMLEscapeString(r.FormValue("alt_text"))),
			toastEvent("success", "Media updated"),
		)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// deleteMedia removes an asset. If it is still referenced by content the delete is
// blocked (no force delete in normal UI). It shows usage refs.
func (h *Handler) deleteMedia(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		writeJSONError(w, http.StatusForbidden, errors.New("invalid security token"))
		return
	}
	id := r.PathValue("id")
	// Use domain-safe delete
	if err := h.media.DeleteIfUnused(r.Context(), id); err != nil {
		if errors.Is(err, media.ErrInUse) {
			refs, rerr := h.media.UsageRefs(r.Context(), id)
			if rerr != nil {
				log.Printf("usage refs after ErrInUse %s: %v", id, rerr)
				refs = nil
			}
			var msg string
			if len(refs) > 0 {
				msg = "This image is used in " + strconv.Itoa(len(refs)) + " places and cannot be deleted. Remove references first or use Replace."
			} else {
				msg = "This media is still in use and cannot be deleted."
				if rerr != nil {
					msg = "Could not determine usage; deletion blocked for safety."
				}
			}
			if isDatastarRequest(r) {
				// Build usage list HTML
				var b strings.Builder
				b.WriteString(`<div id="media-delete-warning" role="alert"><p class="form-warning">` + template.HTMLEscapeString(msg) + `</p>`)
				if len(refs) > 0 {
					b.WriteString(`<ul class="media-usage-list">`)
					for _, ref := range refs {
						b.WriteString(`<li><a href="` + template.HTMLEscapeString(ref.EditURL) + `">` + template.HTMLEscapeString(ref.SourceLabel) + `</a> — ` + template.HTMLEscapeString(ref.Context))
						if ref.Public {
							b.WriteString(` (published)`)
						} else {
							b.WriteString(` (draft)`)
						}
						b.WriteString(`</li>`)
					}
					b.WriteString(`</ul>`)
				}
				b.WriteString(`</div>`)
				writeSSE(w,
					patchElementsEvent("inner", "#media-delete-warning", b.String()),
					toastEvent("error", "Media is still in use"),
				)
				return
			}
			writeJSONError(w, http.StatusConflict, errors.New(msg))
			return
		}
		log.Printf("delete media: %v", err)
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Could not delete media"))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateMedia(id)
	}
	if isDatastarRequest(r) {
		writeSSE(w,
			patchElementsEvent("remove", "[data-media-card=\""+id+"\"]", ""),
			toastEvent("success", "Media deleted"),
		)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// replaceMedia handles safe replacement of an asset's bytes while preserving its ID.
func (h *Handler) replaceMedia(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		writeJSONError(w, http.StatusForbidden, errors.New("invalid security token"))
		return
	}
	id := r.PathValue("id")
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxMediaParseMemory); err != nil {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Upload too large or malformed"))
			return
		}
		writeJSONError(w, http.StatusBadRequest, errors.New("upload too large or malformed"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		// Try alternative field name
		file, header, err = r.FormFile("image")
		if err != nil {
			if isDatastarRequest(r) {
				writeSSE(w, toastEvent("error", "No file provided"))
				return
			}
			writeJSONError(w, http.StatusBadRequest, errors.New("no file provided"))
			return
		}
	}
	defer file.Close()
	asset, err := h.media.Replace(r.Context(), id, header.Filename, file)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, media.ErrTooLarge) || errors.Is(err, media.ErrUnsupportedFormat) || errors.Is(err, media.ErrMalformed) || errors.Is(err, media.ErrInvalidImage) || errors.Is(err, media.ErrDimensionsTooLarge) || errors.Is(err, media.ErrTooManyPixels) {
			status = http.StatusBadRequest
		} else {
			log.Printf("replace media %s: %v", id, err)
			status = http.StatusInternalServerError
		}
		msg := "Could not replace image. The existing image was not changed."
		if err.Error() != "" {
			if status == http.StatusBadRequest {
				msg = err.Error()
			} else {
				msg = "Could not replace image. The existing image was not changed."
			}
		}
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", msg))
			return
		}
		writeJSONError(w, status, errors.New(msg))
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateMedia(id)
		h.runtime.InvalidateContent()
	}
	if isDatastarRequest(r) {
		writeSSE(w,
			toastEvent("success", "Image replaced."),
			patchElementsEvent("outer", "#media-detail-preview", `<div id="media-detail-preview"><img src="/media/`+template.HTMLEscapeString(asset.ID)+`/original" alt=""></div>`),
		)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "asset": toMediaJSON(asset)})
}

// regenerateMedia rebuilds responsive variants from the original.
func (h *Handler) regenerateMedia(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		writeJSONError(w, http.StatusForbidden, errors.New("invalid security token"))
		return
	}
	id := r.PathValue("id")
	if err := h.media.RegenerateVariants(r.Context(), id); err != nil {
		log.Printf("regenerate media %s: %v", id, err)
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Could not regenerate variants"))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateMedia(id)
		h.runtime.InvalidateContent()
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Variants regenerated."))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
