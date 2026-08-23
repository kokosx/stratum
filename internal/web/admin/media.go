package admin

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/kokosx/stratum/internal/media"
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
	Asset    mediaJSON     `json:"asset"`
	Variants []variantJSON `json:"variants"`
	Usage    int64         `json:"usage"`
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

// mediaLibrary renders the central Media Library page.
func (h *Handler) mediaLibrary(w http.ResponseWriter, r *http.Request) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	assets, err := h.media.List(r.Context(), 60, 0)
	if err != nil {
		log.Printf("list media: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	cards := make([]mediaJSON, 0, len(assets))
	for i := range assets {
		cards = append(cards, toMediaJSON(&assets[i]))
	}
	data := mediaLibraryData{Cards: cards, CSRFToken: token}
	layout := LayoutData{Title: "Media", ActiveMenu: "media", CSRFToken: token, Content: data}
	if err := h.mediaTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render media library: %v", err)
	}
}

type mediaLibraryData struct {
	Cards     []mediaJSON
	CSRFToken string
}

// mediaListJSON returns the assets for the Media Picker without a full page load.
func (h *Handler) mediaListJSON(w http.ResponseWriter, r *http.Request) {
	assets, err := h.media.List(r.Context(), 100, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	cards := make([]mediaJSON, 0, len(assets))
	for i := range assets {
		cards = append(cards, toMediaJSON(&assets[i]))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": cards})
}

// uploadMedia handles a multipart image upload and returns JSON for the picker or
// library. It is CSRF-protected. Both the Media Library and picker share this
// single validation pipeline so errors are consistent.
func (h *Handler) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSONError(w, http.StatusForbidden, errors.New("invalid security token"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeJSONError(w, http.StatusBadRequest, errors.New("upload too large or malformed: ensure the file is under 10 MB"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errors.New("no file provided"))
		return
	}
	defer file.Close()

	user, _ := h.currentUser(r)
	asset, err := h.media.Upload(r.Context(), header.Filename, user.ID, file)
	if err != nil {
		status := http.StatusBadRequest
		// All known domain errors are user-actionable (400); only storage failures are 500.
		if errors.Is(err, media.ErrTooLarge) ||
			errors.Is(err, media.ErrUnsupportedFormat) ||
			errors.Is(err, media.ErrMalformed) ||
			errors.Is(err, media.ErrInvalidImage) ||
			errors.Is(err, media.ErrDimensionsTooLarge) ||
			errors.Is(err, media.ErrTooManyPixels) ||
			errors.Is(err, media.ErrSVGUnsafe) ||
			errors.Is(err, media.ErrDerivativeFailed) {
			status = http.StatusBadRequest
		} else {
			log.Printf("upload media: %v", err)
			status = http.StatusInternalServerError
		}
		// Ensure the frontend receives the useful backend message (e.g. SVG unsafe).
		writeJSONError(w, status, err)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateMedia(asset.ID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "asset": toMediaJSON(asset)})
}

// mediaDetailJSON returns one asset with its variants and usage count.
func (h *Handler) mediaDetailJSON(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	asset, err := h.media.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	usage, err := h.media.CountUsage(r.Context(), id)
	if err != nil {
		usage = 0
	}
	detail := mediaDetailJSON{Asset: toMediaJSON(asset)}
	for _, v := range asset.Variants {
		detail.Variants = append(detail.Variants, variantJSON{
			Kind:   v.Kind,
			URL:    "/media/" + id + "/" + v.Kind,
			Width:  v.Width,
			Height: v.Height,
			Size:   v.FileSize,
		})
	}
	detail.Usage = usage
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
// blocked unless force=1 is supplied.
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
	usage, _ := h.media.CountUsage(r.Context(), id)
	if usage > 0 && r.FormValue("force") != "1" {
		msg := "This media is used by " + strconv.FormatInt(usage, 10) + " piece(s) of content. Force delete to remove it everywhere."
		if isDatastarRequest(r) {
			writeSSE(w,
				patchElementsEvent("inner", "#media-delete-warning", `<p class="form-warning" id="media-delete-warning" role="alert">`+template.HTMLEscapeString(msg)+`</p>`),
				toastEvent("error", "Media is still in use"),
			)
			return
		}
		writeJSONError(w, http.StatusConflict, errors.New(msg))
		return
	}
	if err := h.media.Delete(r.Context(), id); err != nil {
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
