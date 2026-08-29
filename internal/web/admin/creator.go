package admin

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/creator"
)

type creatorPageData struct {
	Presets         []creator.Preset
	Selected        creator.PresetID
	SiteName        string
	Tagline         string
	Error           string
	Result          *creator.Result
	Palettes        []creator.Palette
	Headers         []creator.HeaderOption
	Footers         []creator.FooterOption
	SelectedPalette creator.PaletteID
	SelectedHeader  creator.HeaderStyleID
	SelectedFooter  creator.FooterStyleID
}

func (h *Handler) siteCreator(w http.ResponseWriter, r *http.Request) {
	completed, err := h.queries.GetOnboardingCompleted(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if completed != 0 {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	settings, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := creatorPageData{
		Presets:         creator.Presets(),
		Palettes:        creator.Palettes(),
		Headers:         creator.HeaderOptions(),
		Footers:         creator.FooterOptions(),
		Selected:        creator.PresetBlog,
		SiteName:        settings.SiteTitle,
		Tagline:         settings.SiteTagline,
		SelectedPalette: creator.DefaultPaletteForPreset(creator.PresetBlog),
		SelectedHeader:  creator.DefaultHeaderForPreset(creator.PresetBlog),
		SelectedFooter:  creator.DefaultFooterForPreset(creator.PresetBlog),
	}
	if r.Method == http.MethodPost {
		data.Selected = creator.PresetID(strings.TrimSpace(r.FormValue("preset")))
		data.SelectedPalette = creator.PaletteID(strings.TrimSpace(r.FormValue("palette")))
		data.SelectedHeader = creator.HeaderStyleID(strings.TrimSpace(r.FormValue("header_style")))
		data.SelectedFooter = creator.FooterStyleID(strings.TrimSpace(r.FormValue("footer_style")))
		data.SiteName = strings.TrimSpace(r.FormValue("site_name"))
		data.Tagline = strings.TrimSpace(r.FormValue("tagline"))
		if !h.validCSRF(r) {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}
		plan, previewErr := h.creator.Preview(creator.Input{PresetID: data.Selected, SiteTitle: data.SiteName, Tagline: data.Tagline, PaletteID: data.SelectedPalette, HeaderStyleID: data.SelectedHeader, FooterStyleID: data.SelectedFooter})
		if previewErr == nil {
			user, userErr := h.currentUser(r)
			if userErr != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			result, createErr := h.creator.Create(r.Context(), plan, user.ID)
			if createErr == nil {
				data.Result = &result
				h.renderCreator(w, r, data, http.StatusOK)
				return
			}
			previewErr = createErr
		}
		if errors.Is(previewErr, creator.ErrCompleted) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		log.Printf("create starter site: %v", previewErr)
		data.Error = previewErr.Error()
		if data.SelectedPalette == "" {
			data.SelectedPalette = creator.DefaultPaletteForPreset(data.Selected)
		}
		if data.SelectedHeader == "" {
			data.SelectedHeader = creator.DefaultHeaderForPreset(data.Selected)
		}
		if data.SelectedFooter == "" {
			data.SelectedFooter = creator.DefaultFooterForPreset(data.Selected)
		}
		h.renderCreator(w, r, data, http.StatusUnprocessableEntity)
		return
	}
	h.renderCreator(w, r, data, http.StatusOK)
}

func (h *Handler) skipSiteCreator(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := h.creator.Skip(r.Context()); err != nil && !errors.Is(err, creator.ErrCompleted) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.setFlash(w, "Starter site skipped. You can build your site from the admin tools.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) renderCreator(w http.ResponseWriter, r *http.Request, content creatorPageData, status int) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := h.layoutData(r, "Create your site")
	data.CSRFToken = token
	data.Content = content
	w.WriteHeader(status)
	if err := h.creatorTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render site creator: %v", err)
	}
}
