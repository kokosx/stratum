package public

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

type Handler struct {
	queries    *db.Queries
	blocks     *blocks.Registry
	navigation *navigation.Loader
	themes     *themes.Runtime
	media      *media.Service
}

func NewHandler(queries *db.Queries, blocks *blocks.Registry, runtime *themes.Runtime, mediaService *media.Service) (*Handler, error) {
	return &Handler{queries: queries, blocks: blocks, navigation: navigation.NewLoader(queries), themes: runtime, media: mediaService}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/media/") {
		h.serveMedia(w, r)
		return
	}
	if r.URL.Path == "/favicon.ico" {
		h.serveFavicon(w, r)
		return
	}
	switch r.URL.Path {
	case "/stratum/blocks.css":
		serveAsset(w, "text/css; charset=utf-8", h.blocks.Styles())
		return
	case "/stratum/theme.css":
		serveAsset(w, "text/css; charset=utf-8", h.themes.Styles())
		return
	case "/stratum/theme.js":
		serveAsset(w, "text/javascript; charset=utf-8", h.themes.JavaScript())
		return
	case "/sitemap.xml":
		h.serveSitemap(w, r)
		return
	case "/robots.txt":
		h.serveRobots(w, r)
		return
	}

	page, robots, err := h.renderPath(r.Context(), r.URL.Path, requestOrigin(r), nil, nil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("render public page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if robots != "" {
		w.Header().Set("X-Robots-Tag", robots)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func serveAsset(w http.ResponseWriter, contentType, value string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(value))
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// RenderPath is the single public and preview rendering pipeline. Passing
// temporary settings changes only this render and never the runtime snapshot.
func (h *Handler) RenderPath(ctx context.Context, path string, origin string, temporary map[string]any) ([]byte, error) {
	page, _, err := h.renderPath(ctx, path, origin, temporary, nil)
	return page, err
}

func (h *Handler) RenderPreview(ctx context.Context, path string, origin string, temporary map[string]any, customCSS string) ([]byte, error) {
	page, _, err := h.renderPath(ctx, path, origin, temporary, &customCSS)
	return page, err
}

// renderPath renders a public entry. It also returns the X-Robots-Tag value to
// apply to the HTTP response (empty when indexing is allowed).
func (h *Handler) renderPath(ctx context.Context, path, origin string, temporary map[string]any, customCSS *string) ([]byte, string, error) {
	entry, err := h.queries.GetPublishedEntryByPath(ctx, path)
	if err != nil {
		return nil, "", err
	}
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		return nil, "", fmt.Errorf("decode document: %w", err)
	}
	settings, err := h.queries.GetSiteSettings(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("get site settings: %w", err)
	}
	rc := rendering.RenderContext{
		Site: rendering.SiteContext{Name: settings.SiteTitle, Tagline: settings.SiteTagline, URL: settings.SiteUrl},
		Entry: rendering.EntryContext{
			Title:         entry.Title,
			Excerpt:       stringValue(entry.Excerpt),
			Permalink:     path,
			PublishDate:   formatEntryDate(entry.PublishedAt, settings.Timezone, false),
			PublishISO:    formatEntryDate(entry.PublishedAt, settings.Timezone, true),
			FeaturedImage: stringValue(entry.FeaturedMediaID),
		},
	}
	if settings.SiteLogoMediaID.Valid && settings.SiteLogoMediaID.String != "" {
		if view, ok := h.media.MediaView(ctx, settings.SiteLogoMediaID.String); ok {
			rc.Site.LogoURL = view.Src
		}
	}
	if settings.SocialLinks.Valid && settings.SocialLinks.String != "" {
		if links, ok := parseSocialLinks(settings.SocialLinks.String); ok {
			rc.Site.SocialLinks = links
		}
	}
	content, err := h.blocks.RenderDocumentContext(doc, rc)
	if err != nil {
		return nil, "", fmt.Errorf("render document: %w", err)
	}
	menus, err := h.navigation.LoadLocationsForPath(ctx, path)
	if err != nil {
		return nil, "", fmt.Errorf("load navigation: %w", err)
	}

	siteIcon, _ := h.siteIconView(ctx)
	canonical := buildCanonical(settings.SiteUrl, origin, path, stringValue(entry.CanonicalUrl))

	robots := ""
	if settings.IndexingEnabled == 0 {
		robots = "noindex,nofollow"
	}

	speculation := themes.SpeculationView{Mode: settings.SpeculationMode, Eagerness: settings.SpeculationEagerness}
	if settings.SpeculationMode != "off" {
		rules, rulesErr := site.BuildSpeculationRules(settings.SpeculationMode, settings.SpeculationEagerness)
		if rulesErr == nil && rules != "" {
			speculation.Enabled = true
			speculation.RulesJSON = template.JS(rules)
		}
	}

	view := themes.PageView{
		Site:  themes.SiteView{Title: settings.SiteTitle, Tagline: settings.SiteTagline, Language: settings.Language, SiteURL: settings.SiteUrl},
		Entry: themes.EntryView{Title: entry.Title, SEOTitle: stringValue(entry.SeoTitle), SEODescription: stringValue(entry.SeoDescription), CanonicalURL: canonical},
		Head: themes.HeadView{
			Title:       entry.Title,
			Description: stringValue(entry.SeoDescription),
			Canonical:   canonical,
			Robots:      robots,
			Speculation: speculation,
			SiteIcon:    siteIcon,
		},
		Navigation: menus,
		Content:    content,
	}
	if customCSS != nil {
		page, err := h.themes.Preview(view, temporary, *customCSS)
		return page, robots, err
	}
	page, err := h.themes.Render(view, temporary)
	return page, robots, err
}

// serveSitemap streams the dynamic XML sitemap. When sitemaps are disabled it
// responds 404 so an empty document is never served as if the feature were on.
func (h *Handler) serveSitemap(w http.ResponseWriter, r *http.Request) {
	settings, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		log.Printf("sitemap settings: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if settings.SitemapEnabled == 0 {
		http.NotFound(w, r)
		return
	}
	entries, err := h.queries.ListSitemapEntries(r.Context())
	if err != nil {
		log.Printf("sitemap entries: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	base := joinOrigin(settings.SiteUrl, "")
	urlset := sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, entry := range entries {
		urlset.URLs = append(urlset.URLs, sitemapURL{
			Loc:     base + entry.RoutePath,
			Lastmod: time.Unix(entry.Lastmod, 0).UTC().Format(time.RFC3339),
		})
	}
	body, err := xml.Marshal(urlset)
	if err != nil {
		log.Printf("sitemap marshal: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}

// serveRobots returns the dynamic robots.txt. Managed mode emits a safe default
// derived from the indexing and sitemap settings; custom mode returns the
// administrator-provided text verbatim.
func (h *Handler) serveRobots(w http.ResponseWriter, r *http.Request) {
	settings, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		log.Printf("robots settings: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	body := site.BuildRobots(site.RobotsInput{
		Mode:            settings.RobotsMode,
		IndexingEnabled: settings.IndexingEnabled != 0,
		SitemapEnabled:  settings.SitemapEnabled != 0,
		SiteURL:         settings.SiteUrl,
		Custom:          settings.RobotsCustom,
	})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write([]byte(body))
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod"`
}

// buildCanonical derives the public canonical URL for an entry. An explicit
// revision override wins; otherwise the canonical is the site origin joined with
// the entry route path. The origin falls back to the request origin when the
// site_url setting is empty so the result stays absolute.
func buildCanonical(siteURL, origin, path, override string) string {
	if override != "" {
		if strings.HasPrefix(override, "http://") || strings.HasPrefix(override, "https://") {
			return override
		}
		if strings.HasPrefix(override, "/") {
			return joinOrigin(siteURL, origin) + override
		}
		return override
	}
	return joinOrigin(siteURL, origin) + path
}

func joinOrigin(siteURL, origin string) string {
	base := siteURL
	if base == "" {
		base = origin
	}
	return strings.TrimRight(base, "/")
}

func stringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

// parseSocialLinks decodes the site_settings social_links JSON column into the
// slice the Social Links block renders. Malformed or empty values yield an empty
// slice so the block falls back to its placeholder.
func parseSocialLinks(raw string) ([]rendering.SiteSocialLink, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var links []rendering.SiteSocialLink
	if err := json.Unmarshal([]byte(raw), &links); err != nil {
		return nil, false
	}
	return links, true
}

// formatEntryDate renders an entry's publish timestamp in the site timezone.
// When iso is true it returns an RFC3339 value suitable for <time datetime>.
func formatEntryDate(ts sql.NullInt64, tz string, iso bool) string {
	if !ts.Valid {
		return ""
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	t := time.Unix(ts.Int64, 0).In(loc)
	if iso {
		return t.Format(time.RFC3339)
	}
	return t.Format("January 2, 2006")
}

// serveMedia streams a stored media derivative. Path is /media/{id} (original) or
// /media/{id}/{kind} where kind is a responsive width or "favicon-N".
func (h *Handler) serveMedia(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/media/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	kind := ""
	if len(parts) == 2 {
		kind = parts[1]
	}
	if id == "" {
		http.NotFound(w, r)
		return
	}
	data, mime, err := h.media.ReadVariant(r.Context(), id, kind)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		return
	}
	_, _ = w.Write(data)
}

// serveFavicon serves the 32px site-icon variant as the legacy /favicon.ico.
func (h *Handler) serveFavicon(w http.ResponseWriter, r *http.Request) {
	id, err := h.queries.GetSiteIconMediaID(r.Context())
	if err != nil || !id.Valid {
		http.NotFound(w, r)
		return
	}
	data, mime, err := h.media.ReadVariant(r.Context(), id.String, "favicon-32")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		return
	}
	_, _ = w.Write(data)
}

// siteIconView resolves the configured Site Icon into favicon links, or nil.
func (h *Handler) siteIconView(ctx context.Context) (*rendering.FaviconView, error) {
	id, err := h.queries.GetSiteIconMediaID(ctx)
	if err != nil || !id.Valid {
		return nil, err
	}
	view, ok := h.media.FaviconView(ctx, id.String)
	if !ok {
		return nil, nil
	}
	return &view, nil
}
