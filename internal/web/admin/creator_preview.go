package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/creator"
)

// creatorPreview handles authenticated, read-only preview rendering.
// It never writes to the database. Inputs are validated via creator.Preview
// and rendered through the real block+theme pipeline with in-memory providers.
func (h *Handler) creatorPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Allow self-framing only for this response; do not weaken global policy.
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	// Parse inputs (support both query and form)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	input := creator.Input{
		PresetID:      creator.PresetID(strings.TrimSpace(r.FormValue("preset"))),
		PaletteID:     creator.PaletteID(strings.TrimSpace(r.FormValue("palette"))),
		HeaderStyleID: creator.HeaderStyleID(strings.TrimSpace(r.FormValue("header_style"))),
		FooterStyleID: creator.FooterStyleID(strings.TrimSpace(r.FormValue("footer_style"))),
		SiteTitle:     strings.TrimSpace(r.FormValue("site_name")),
		Tagline:       strings.TrimSpace(r.FormValue("tagline")),
		Language:      strings.TrimSpace(r.FormValue("language")),
		Timezone:      strings.TrimSpace(r.FormValue("timezone")),
		SiteRepresents: strings.TrimSpace(r.FormValue("site_represents")),
		SiteURL:       strings.TrimSpace(r.FormValue("site_url")),
	}
	if v := strings.TrimSpace(r.FormValue("product_media")); v != "" {
		input.ProductMediaPosition = v
	} else if v := strings.TrimSpace(r.FormValue("product_media_position")); v != "" {
		input.ProductMediaPosition = v
	}
	if v := strings.TrimSpace(r.FormValue("blog_latest")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			input.BlogLatestCount = iv
		}
	}
	if v := strings.TrimSpace(r.FormValue("blog_archive")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			input.BlogArchiveCount = iv
		}
	}
	if v := strings.TrimSpace(r.FormValue("portfolio_cols")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			input.PortfolioColumns = iv
		}
	}
	if v := strings.TrimSpace(r.FormValue("product_cols")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			input.ProductColumns = iv
		}
	}
	if v := strings.TrimSpace(r.FormValue("testimonials_cols")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			input.LandingTestimonialsColumns = iv
		}
	}
	if v := strings.TrimSpace(r.FormValue("service_cols")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			input.ServiceColumns = iv
		}
	}
	if strings.TrimSpace(r.FormValue("indexing_enabled")) == "on" || strings.TrimSpace(r.FormValue("indexing_enabled")) == "1" || strings.TrimSpace(r.FormValue("indexing_enabled")) == "true" {
		input.IndexingEnabled = true
	}
	// Fallbacks for site details when not provided via form (use current settings)
	if input.SiteTitle == "" {
		// Use non-empty placeholder to keep preview renderable even before typing
		input.SiteTitle = "Example Studio"
	}
	if input.Language == "" {
		input.Language = "en"
	}
	if input.Timezone == "" {
		input.Timezone = "UTC"
	}
	surface := creator.PreviewSurface(strings.TrimSpace(r.FormValue("surface")))
	if surface == "" {
		surface = creator.SurfaceHome
	}
	// Normalize surface values from toolbar
	switch strings.ToLower(string(surface)) {
	case "home", "", "homepage":
		surface = creator.SurfaceHome
	case "archive", "blog", "work", "products", "services":
		surface = creator.SurfaceArchive
	case "single", "post", "project", "product", "service":
		surface = creator.SurfaceSingle
	default:
		surface = creator.SurfaceHome
	}
	plan, err := h.creator.Preview(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	html, err := creator.RenderPreview(r.Context(), plan, surface, h.blocks, h.themes)
	if err != nil {
		http.Error(w, "Preview render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
