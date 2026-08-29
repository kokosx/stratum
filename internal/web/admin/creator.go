package admin

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/creator"
	"github.com/kokosx/stratum/internal/site"
)

type creatorPageData struct {
	Presets          []creator.Preset
	Selected         creator.PresetID
	SiteName         string
	Tagline          string
	Error            string
	Result           *creator.Result
	Palettes         []creator.Palette
	Headers          []creator.HeaderOption
	Footers          []creator.FooterOption
	SelectedPalette  creator.PaletteID
	SelectedHeader   creator.HeaderStyleID
	SelectedFooter   creator.FooterStyleID
	Language         string
	Timezone         string
	SiteRepresents   string
	IndexingEnabled  bool
	SiteURL          string
	LanguageOptions  []site.LanguageOption
	TimezoneOptions  []site.TimezoneOption
	BlogLatest       int
	BlogArchive      int
	PortfolioCols    int
	ProductCols      int
	ProductMedia     string
	TestimonialsCols int
	ServiceCols      int
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
	// Determine defaults with safe fallbacks
	defaultPreset := creator.PresetBlog
	data := creatorPageData{
		Presets:          creator.Presets(),
		Palettes:         creator.Palettes(),
		Headers:          creator.HeaderOptions(),
		Footers:          creator.FooterOptions(),
		Selected:         defaultPreset,
		SiteName:         settings.SiteTitle,
		Tagline:          settings.SiteTagline,
		SelectedPalette:  creator.DefaultPaletteForPreset(defaultPreset),
		SelectedHeader:   creator.DefaultHeaderForPreset(defaultPreset),
		SelectedFooter:   creator.DefaultFooterForPreset(defaultPreset),
		Language:         "en",
		Timezone:         settings.Timezone,
		SiteRepresents:   creator.DefaultRepresentsForPreset(defaultPreset),
		IndexingEnabled:  true,
		SiteURL:          settings.SiteUrl,
		LanguageOptions:  site.CreatorLanguageOptions(),
		TimezoneOptions:  site.TimezoneOptions(),
		BlogLatest:       5,
		BlogArchive:      10,
		PortfolioCols:    2,
		ProductCols:      3,
		ProductMedia:     "left",
		TestimonialsCols: 2,
		ServiceCols:      3,
	}
	if data.Timezone == "" {
		data.Timezone = "UTC"
	}
	if strings.Contains(strings.ToLower(data.SiteURL), "localhost") || strings.Contains(data.SiteURL, "127.0.0.1") || strings.Contains(data.SiteURL, "192.168.") {
		data.SiteURL = ""
	}
	if r.Method == http.MethodPost {
		data.Selected = creator.PresetID(strings.TrimSpace(r.FormValue("preset")))
		data.SelectedPalette = creator.PaletteID(strings.TrimSpace(r.FormValue("palette")))
		data.SelectedHeader = creator.HeaderStyleID(strings.TrimSpace(r.FormValue("header_style")))
		data.SelectedFooter = creator.FooterStyleID(strings.TrimSpace(r.FormValue("footer_style")))
		data.SiteName = strings.TrimSpace(r.FormValue("site_name"))
		data.Tagline = strings.TrimSpace(r.FormValue("tagline"))
		data.Language = strings.TrimSpace(r.FormValue("language"))
		data.Timezone = strings.TrimSpace(r.FormValue("timezone"))
		data.SiteRepresents = strings.TrimSpace(r.FormValue("site_represents"))
		data.SiteURL = strings.TrimSpace(r.FormValue("site_url"))
		// Checkbox is "Discourage search engines" — when checked, indexing should be DISABLED.
		// So invert: checked => IndexingEnabled=false, unchecked => true.
		if v := strings.TrimSpace(r.FormValue("indexing_enabled")); v == "on" || v == "1" || v == "true" {
			data.IndexingEnabled = false
		} else {
			data.IndexingEnabled = true
		}
		if v := strings.TrimSpace(r.FormValue("blog_latest")); v != "" {
			if iv, err := strconv.Atoi(v); err == nil {
				data.BlogLatest = iv
			}
		}
		if v := strings.TrimSpace(r.FormValue("blog_archive")); v != "" {
			if iv, err := strconv.Atoi(v); err == nil {
				data.BlogArchive = iv
			}
		}
		if v := strings.TrimSpace(r.FormValue("portfolio_cols")); v != "" {
			if iv, err := strconv.Atoi(v); err == nil {
				data.PortfolioCols = iv
			}
		}
		if v := strings.TrimSpace(r.FormValue("product_cols")); v != "" {
			if iv, err := strconv.Atoi(v); err == nil {
				data.ProductCols = iv
			}
		}
		if v := strings.TrimSpace(r.FormValue("product_media")); v != "" {
			data.ProductMedia = v
		}
		if v := strings.TrimSpace(r.FormValue("testimonials_cols")); v != "" {
			if iv, err := strconv.Atoi(v); err == nil {
				data.TestimonialsCols = iv
			}
		}
		if v := strings.TrimSpace(r.FormValue("service_cols")); v != "" {
			if iv, err := strconv.Atoi(v); err == nil {
				data.ServiceCols = iv
			}
		}
		if !h.validCSRF(r) {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}
		input := creator.Input{
			PresetID:                   data.Selected,
			SiteTitle:                  data.SiteName,
			Tagline:                    data.Tagline,
			PaletteID:                  data.SelectedPalette,
			HeaderStyleID:              data.SelectedHeader,
			FooterStyleID:              data.SelectedFooter,
			Language:                   data.Language,
			Timezone:                   data.Timezone,
			SiteRepresents:             data.SiteRepresents,
			IndexingEnabled:            data.IndexingEnabled,
			SiteURL:                    data.SiteURL,
			BlogLatestCount:            data.BlogLatest,
			BlogArchiveCount:           data.BlogArchive,
			PortfolioColumns:           data.PortfolioCols,
			ProductColumns:             data.ProductCols,
			ProductMediaPosition:       data.ProductMedia,
			LandingTestimonialsColumns: data.TestimonialsCols,
			ServiceColumns:             data.ServiceCols,
		}
		plan, previewErr := h.creator.Preview(input)
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
		if data.Language == "" {
			data.Language = "en"
		}
		if data.Timezone == "" {
			data.Timezone = "UTC"
		}
		if data.SiteRepresents == "" {
			data.SiteRepresents = creator.DefaultRepresentsForPreset(data.Selected)
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
	if err := h.creatorTemplate.ExecuteTemplate(w, "creator_layout.html", data); err != nil {
		log.Printf("render site creator: %v", err)
	}
}
