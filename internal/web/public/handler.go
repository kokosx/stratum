package public

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/pagecache"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/seo"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

type Handler struct {
	queries *db.Queries
	blocks  *blocks.Registry
	themes  *themes.Runtime
	media   *media.Service
	hub     *runtimehub.Runtime
	dev     bool
	// warnNoSiteURL guards the one-time production warning about canonical
	// URLs falling back to the request Host.
	warnNoSiteURL sync.Once
}

func NewHandler(queries *db.Queries, blocksReg *blocks.Registry, runtime *themes.Runtime, mediaService *media.Service) (*Handler, error) {
	hub, err := runtimehub.New(queries, blocksReg, runtime, mediaService)
	if err != nil {
		return nil, err
	}
	return NewHandlerWithHub(hub)
}

// NewHandlerWithHub builds a public handler around a shared runtime coordinator
// so admin write paths and the public frontend share the same caches.
func NewHandlerWithHub(hub *runtimehub.Runtime) (*Handler, error) {
	return &Handler{
		queries: hub.Queries,
		blocks:  hub.Blocks,
		themes:  hub.Themes,
		media:   hub.Media,
		hub:     hub,
		dev:     os.Getenv("STRATUM_ENV") != "production",
	}, nil
}

// Hub exposes the shared runtime coordinator so callers can invalidate caches.
func (h *Handler) Hub() *runtimehub.Runtime { return h.hub }

// AssetURLs returns the current fingerprinted asset URLs.
func (h *Handler) AssetURLs() (blocksCSS, themeCSS, themeJS string) {
	return h.hub.Assets.URLs()
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

	// Fingerprinted immutable assets.
	if h.hub.Assets.Serve(w, r) {
		return
	}
	// Legacy mutable asset URLs redirect to the fingerprinted immutable URL.
	if legacy := h.hub.Assets.LegacyRedirect(r.URL.Path); legacy != "" {
		http.Redirect(w, r, legacy, http.StatusFound)
		return
	}

	switch r.URL.Path {
	case "/sitemap.xml":
		h.serveSitemap(w, r)
		return
	case "/robots.txt":
		h.serveRobots(w, r)
		return
	case "/feed.xml":
		h.serveFeed(w, r)
		return
	}

	h.serveCachedPage(w, r)
}

func (h *Handler) serveCachedPage(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	origin := requestOrigin(r)
	// In production, canonical/OG/schema URLs must come from the configured
	// Site URL, never accidentally from the request Host. The origin fallback
	// remains for development/local setups; warn once when it is doing the work.
	if siteSnap != nil && strings.TrimSpace(siteSnap.SiteURL) == "" && !h.dev {
		h.warnNoSiteURL.Do(func() {
			log.Printf("stratum: no Site URL configured; canonical, OG and schema URLs fall back to the request host (%s). Configure Site URL in Settings for production.", origin)
		})
	}
	// When canonical depends on the request origin (no configured Site URL), the
	// cache key must include the origin so HTML for the wrong host is never served.
	key := pagecache.Key("", r.URL.Path)
	if siteSnap == nil || siteSnap.SiteURL == "" {
		key = pagecache.Key(origin, r.URL.Path)
	}

	// Full-page cache HIT: serve without touching the database (including
	// redirect lookups). Redirects and renders run only on miss.
	if cached, ok := h.hub.Pages.Get(key); ok {
		if h.dev {
			w.Header().Set("Server-Timing", "cache;desc=\"hit\"")
		}
		h.writePage(w, r, cached)
		return
	}

	// Redirect routes left by slug changes (e.g. /old → /new). Checked only on
	// page-cache miss so a warm cache never pays for GetRouteByPath.
	if route, rerr := h.queries.GetRouteByPath(r.Context(), r.URL.Path); rerr == nil && route.RouteType == "redirect" && route.RedirectTo.Valid && route.RedirectTo.String != "" {
		status := http.StatusMovedPermanently
		if route.RedirectStatus.Valid && route.RedirectStatus.Int64 != 0 {
			status = int(route.RedirectStatus.Int64)
		}
		http.Redirect(w, r, route.RedirectTo.String, status)
		return
	}

	entry, err := h.hub.Pages.Do(key, func() (pagecache.Entry, error) {
		return h.renderPage(r.Context(), origin, r.URL.Path)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("render public page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if h.dev {
		w.Header().Set("Server-Timing", "cache;desc=\"miss\"")
	}
	h.writePage(w, r, entry)
}

func (h *Handler) writePage(w http.ResponseWriter, r *http.Request, entry pagecache.Entry) {
	// HTML must revalidate: the public URL is stable across Publish, so a long
	// immutable max-age would freeze stale content in browsers/CDNs. no-cache
	// still allows storing the response; clients must revalidate via ETag.
	const htmlCacheControl = "no-cache"

	if match := r.Header.Get("If-None-Match"); match != "" && match == entry.ETag {
		w.Header().Set("ETag", entry.ETag)
		w.Header().Set("Cache-Control", htmlCacheControl)
		w.Header().Set("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("ETag", entry.ETag)
	w.Header().Set("Cache-Control", htmlCacheControl)
	w.Header().Set("Vary", "Accept-Encoding")
	if entry.Robots != "" {
		w.Header().Set("X-Robots-Tag", entry.Robots)
	}

	acceptGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	if acceptGzip && len(entry.Gzip) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.Gzip)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(entry.Gzip)
		}
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(entry.HTML)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(entry.HTML)
	}
}

func (h *Handler) renderPage(ctx context.Context, origin, path string) (pagecache.Entry, error) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil {
		return pagecache.Entry{}, fmt.Errorf("site runtime not initialized")
	}

	// 1. exact route (source of truth)
	if route, rerr := h.queries.GetRouteByPath(ctx, path); rerr == nil {
		switch route.RouteType {
		case "redirect":
			// handled earlier in serve
			return pagecache.Entry{}, sql.ErrNoRows
		case "archive":
			return h.renderArchivePage(ctx, origin, path, 1, siteSnap)
		case "entry":
			return h.renderEntryByRoute(ctx, origin, path, route, siteSnap)
		case "system":
			return pagecache.Entry{}, sql.ErrNoRows
		}
	}

	// 2. pagination child: /blog/page/3 or /page/3 for home archive
	if base, pg, ok := parseArchivePagination(path); ok {
		if rt, rerr := h.queries.GetRouteByPath(ctx, base); rerr == nil && rt.RouteType == "archive" {
			if pg < 1 {
				return pagecache.Entry{}, sql.ErrNoRows
			}
			return h.renderArchivePage(ctx, origin, base, pg, siteSnap)
		}
		// if home archive and path /page/N
		if base == "/" {
			if snap := h.hub.Site.Current(); snap != nil && snap.HomepageMode == "latest_posts" {
				return h.renderArchivePage(ctx, origin, "/", pg, siteSnap)
			}
		}
	}

	// 3. fallback to old entry path (compat for direct)
	entry, err := h.queries.GetPublishedEntryByPath(ctx, path)
	if err != nil {
		return pagecache.Entry{}, err
	}
	return h.renderEntry(ctx, origin, path, entry, siteSnap)
}

func (h *Handler) renderEntryByRoute(ctx context.Context, origin, path string, route db.Route, siteSnap *site.Snapshot) (pagecache.Entry, error) {
	if !route.EntryID.Valid {
		return pagecache.Entry{}, sql.ErrNoRows
	}
	// reuse existing by loading the published via path (it joins on route)
	entry, err := h.queries.GetPublishedEntryByPath(ctx, path)
	if err != nil {
		return pagecache.Entry{}, err
	}
	return h.renderEntry(ctx, origin, path, entry, siteSnap)
}

func (h *Handler) renderEntry(ctx context.Context, origin, path string, entry db.GetPublishedEntryByPathRow, siteSnap *site.Snapshot) (pagecache.Entry, error) {
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("decode document: %w", err)
	}
	prepared, err := h.blocks.PreparedCache(entry.RevisionID, doc)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("prepare document: %w", err)
	}
	resolved := h.resolvePublishedSEO(ctx, siteSnap, &entry, path, origin)
	rc := h.entryRenderContext(siteSnap, &entry, path, resolved)
	content, err := h.blocks.RenderPrepared(ctx, prepared, rc)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("render document: %w", err)
	}
	menus := h.hub.Navigation.LocationsForPath(path)
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, resolved, siteIcon)
	head.Preloads = h.lcpPreloads(ctx, prepared, rc)

	_, themeCSS, themeJS := h.hub.Assets.URLs()
	blocksCSS := h.hub.Assets.BlocksCSSFor(prepared.UsedBlocks)
	kind := themes.PageKindSingle
	if path == "/" {
		kind = themes.PageKindHome
	}
	view := themes.PageView{
		Site:        themes.SiteView{Title: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: rc.Site.LogoURL, LogoWidth: rc.Site.LogoWidth, LogoHeight: rc.Site.LogoHeight},
		Entry:       themes.EntryView{Title: entry.Title, SEOTitle: stringValue(entry.SeoTitle), SEODescription: resolved.Description, CanonicalURL: resolved.Canonical},
		Head:        head,
		Navigation:  menus,
		Content:     content,
		ContentType: entry.ContentTypeID,
		Kind:        kind,
		Assets:      themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}
	html, err := h.themes.Render(view, nil)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("theme render: %w", err)
	}
	gz, _ := gzipBytes(html)
	return pagecache.Entry{
		HTML:        html,
		Gzip:        gz,
		ETag:        pagecache.ComputeETag(html),
		Robots:      resolved.Robots,
		ContentType: "text/html; charset=utf-8",
	}, nil
}

func (h *Handler) renderArchivePage(ctx context.Context, origin, archivePath string, pageNum int, siteSnap *site.Snapshot) (pagecache.Entry, error) {
	if pageNum < 1 {
		return pagecache.Entry{}, sql.ErrNoRows
	}
	postsBase := siteSnap.PostsBasePath
	if postsBase == "" {
		postsBase = seo.DefaultPostsBase
	}
	perPage := int(siteSnap.PostsPerPage)
	if perPage <= 0 {
		perPage = 10
	}
	offset := (pageNum - 1) * perPage

	total, err := h.queries.CountPublishedEntriesByContentType(ctx, "post")
	if err != nil {
		return pagecache.Entry{}, err
	}
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if pageNum > totalPages && totalPages > 0 {
		return pagecache.Entry{}, sql.ErrNoRows
	}
	if pageNum == 1 && strings.HasSuffix(archivePath, "/page/1") {
		// will be handled by redirect higher? return normal for /blog
	}

	rows, err := h.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{
		ContentTypeID: "post",
		Limit:         int64(perPage),
		Offset:        int64(offset),
	})
	if err != nil {
		return pagecache.Entry{}, err
	}

	// Build archive entries + batch media (simple loop ok for small perPage)
	entries := make([]themes.ArchiveEntryView, 0, len(rows))
	featIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.FeaturedMediaID.Valid && r.FeaturedMediaID.String != "" {
			featIDs = append(featIDs, r.FeaturedMediaID.String)
		}
	}
	mediaCache := map[string]rendering.MediaView{}
	for _, id := range featIDs {
		if v, ok := h.media.MediaView(ctx, id); ok {
			mediaCache[id] = v
		}
	}

	for _, r := range rows {
		u := seo.EntryPath("post", r.Slug, postsBase)
		// if this archive is mounted at / (latest home), still use post base for singles
		ev := themes.ArchiveEntryView{
			ID:           r.ID,
			Title:        r.Title,
			Excerpt:      stringValue(r.Excerpt),
			URL:          u,
			PublishedAt:  formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, false),
			PublishedISO: formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, true),
		}
		if r.FeaturedMediaID.Valid {
			if mv, ok := mediaCache[r.FeaturedMediaID.String]; ok {
				ev.FeaturedImage = mv
			}
		}
		entries = append(entries, ev)
	}

	// Intro from posts page published revision if set
	var intro template.HTML
	archiveTitle := "Blog"
	archiveDesc := ""
	if siteSnap.PostsPageEntryID != "" {
		ppath := ""
		if pentry, perr := h.queries.GetEntry(ctx, siteSnap.PostsPageEntryID); perr == nil && pentry.PublishedRevisionID.Valid {
			if prt, prerr := h.queries.GetEntryRoute(ctx, sql.NullString{String: pentry.ID, Valid: true}); prerr == nil {
				ppath = prt.Path
			}
			if ppath != "" {
				if pfull, perr := h.queries.GetPublishedEntryByPath(ctx, ppath); perr == nil {
					archiveTitle = pfull.Title
					archiveDesc = stringValue(pfull.Excerpt)
					if pfull.DocumentJson != "" {
						if d, derr := document.Decode([]byte(pfull.DocumentJson)); derr == nil {
							if prep, perr := h.blocks.PreparedCache(pfull.RevisionID, d); perr == nil {
								irc := rendering.RenderContext{Site: rendering.SiteContext{Name: siteSnap.SiteTitle, URL: siteSnap.SiteURL}}
								if c, cerr := h.blocks.RenderPrepared(ctx, prep, irc); cerr == nil {
									intro = c
								}
							}
						}
					}
				}
			}
		}
	}

	// pagination urls
	prev, next := "", ""
	if pageNum > 1 {
		prev = seo.PaginatedPath(archivePath, pageNum-1)
	}
	if pageNum < totalPages {
		next = seo.PaginatedPath(archivePath, pageNum+1)
	}

	archView := themes.ArchiveView{
		ContentTypeID: "post",
		Title:         archiveTitle,
		Description:   archiveDesc,
		Intro:         intro,
		Entries:       entries,
		Pagination: themes.PaginationView{
			Current:     pageNum,
			TotalPages:  totalPages,
			TotalItems:  total,
			PreviousURL: prev,
			NextURL:     next,
		},
	}

	// SEO for archive
	resolved := h.resolveArchiveSEO(ctx, siteSnap, archivePath, pageNum, archiveTitle, archiveDesc, origin)

	menus := h.hub.Navigation.LocationsForPath(archivePath)
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, resolved, siteIcon)
	// no LCP preload for archive listing by default (first card not auto high)

	_, themeCSS, themeJS := h.hub.Assets.URLs()
	blocksCSS := h.hub.Assets.BlocksCSSFor(nil)
	kind := themes.PageKindArchive
	if archivePath == "/" {
		kind = themes.PageKindHome
	}
	view := themes.PageView{
		Site:        themes.SiteView{Title: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: siteIconURL(siteSnap, h.media), LogoWidth: 0, LogoHeight: 0},
		Head:        head,
		Navigation:  menus,
		ContentType: "post",
		Kind:        kind,
		Archive:     archView,
		Assets:      themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}

	html, err := h.themes.Render(view, nil)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("theme render archive: %w", err)
	}
	gz, _ := gzipBytes(html)
	return pagecache.Entry{
		HTML:        html,
		Gzip:        gz,
		ETag:        pagecache.ComputeETag(html),
		Robots:      resolved.Robots,
		ContentType: "text/html; charset=utf-8",
	}, nil
}

func siteIconURL(snap *site.Snapshot, m *media.Service) string {
	if snap == nil || snap.LogoMediaID == "" || m == nil {
		return ""
	}
	if v, ok := m.MediaView(context.Background(), snap.LogoMediaID); ok {
		return v.Src
	}
	return ""
}

// RSS types (simple, no external dep)
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

func (h *Handler) serveFeed(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil || strings.TrimSpace(siteSnap.SiteURL) == "" {
		http.NotFound(w, r)
		return
	}
	body, gz, etag, ok := h.hub.Feed.Get()
	if !ok {
		built, err := h.buildFeed(r.Context(), siteSnap)
		if err != nil {
			log.Printf("feed build: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		gz, _ = gzipBytes(built)
		etag = pagecache.ComputeETag(built)
		h.hub.Feed.Set(built, gz, etag)
		body = built
	}
	h.writeText(w, r, body, gz, etag, "application/rss+xml; charset=utf-8", "public, max-age=300")
}

func (h *Handler) buildFeed(ctx context.Context, siteSnap *site.Snapshot) ([]byte, error) {
	base := strings.TrimRight(siteSnap.SiteURL, "/")
	// latest 20 published posts
	rows, err := h.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: "post", Limit: 20, Offset: 0})
	if err != nil {
		return nil, err
	}
	items := make([]rssItem, 0, len(rows))
	for _, r := range rows {
		link := base + seo.EntryPath("post", r.Slug, siteSnap.PostsBasePath)
		pub := ""
		if r.FirstPublishedAt.Valid {
			pub = time.Unix(r.FirstPublishedAt.Int64, 0).UTC().Format(time.RFC1123)
		}
		guid := r.ID // stable GUID by entry id
		desc := stringValue(r.Excerpt)
		items = append(items, rssItem{
			Title:       r.Title,
			Link:        link,
			Description: desc,
			PubDate:     pub,
			GUID:        guid,
		})
	}
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:       siteSnap.SiteTitle,
			Link:        base + "/",
			Description: siteSnap.SiteTagline,
			Items:       items,
		},
	}
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>\n` + string(out)), nil
}

func (h *Handler) resolveArchiveSEO(ctx context.Context, siteSnap *site.Snapshot, path string, page int, title, desc, origin string) seo.Resolved {
	seoTitle := title
	if page > 1 {
		seoTitle = fmt.Sprintf("%s – Page %d – %s", title, page, siteSnap.SiteTitle)
	}
	canonical := seo.Canonical(siteSnap.SiteURL, origin, path, "")
	if page > 1 {
		canonical = seo.Canonical(siteSnap.SiteURL, origin, seo.PaginatedPath(path, page), "")
	}
	robots := "index,follow"
	if !siteSnap.IndexingEnabled {
		robots = "noindex,nofollow"
	}
	res := seo.Resolved{
		Title:       seoTitle,
		Description: desc,
		Canonical:   canonical,
		Robots:      robots,
		OpenGraph: seo.OpenGraphView{
			Title:       seoTitle,
			Description: desc,
			URL:         canonical,
			Type:        "website",
			SiteName:    siteSnap.SiteTitle,
			Locale:      siteSnap.Language,
		},
		Twitter: seo.TwitterView{
			Card:  "summary_large_image",
			Title: seoTitle,
		},
	}
	// social image from global
	if siteSnap.GlobalSocialMediaID != "" {
		if v, ok := h.media.MediaView(ctx, siteSnap.GlobalSocialMediaID); ok {
			abs := seo.Canonical(siteSnap.SiteURL, origin, "", v.Src) // ensure abs? media already?
			res.OpenGraph.Image = abs
			res.OpenGraph.ImageWidth = v.Width
			res.OpenGraph.ImageHeight = v.Height
			res.Twitter.Image = abs
		}
	}
	feed := seo.Canonical(siteSnap.SiteURL, origin, "/feed.xml", "")
	res.Alternates = append(res.Alternates, seo.AlternateView{Href: feed, Type: "application/rss+xml"})
	return res
}

func (h *Handler) resolvePublishedSEO(ctx context.Context, siteSnap *site.Snapshot, entry *db.GetPublishedEntryByPathRow, path, origin string) seo.Resolved {
	resolver := seo.New()
	resolved := resolver.Resolve(seo.Input{
		Site: seo.SiteSEO{
			Title:               siteSnap.SiteTitle,
			TitleSeparator:      siteSnap.TitleSeparator,
			SiteURL:             siteSnap.SiteURL,
			Language:            siteSnap.Language,
			IndexingEnabled:     siteSnap.IndexingEnabled,
			GlobalSocialMediaID: siteSnap.GlobalSocialMediaID,
			TwitterSite:         siteSnap.TwitterSite,
		},
		ContentType:   nil, // content-type defaults inherit from site until typed defaults are added
		ContentTypeID: entry.ContentTypeID,
		Revision: seo.RevisionSEO{
			Title:           entry.Title,
			Excerpt:         stringValue(entry.Excerpt),
			SeoTitle:        stringValue(entry.SeoTitle),
			SeoDescription:  stringValue(entry.SeoDescription),
			CanonicalURL:    stringValue(entry.CanonicalUrl),
			FeaturedMediaID: stringValue(entry.FeaturedMediaID),
			SocialMediaID:   stringValue(entry.SocialMediaID),
			RobotsIndex:     seo.NullIntToBoolPtr(entry.SeoRobotsIndex.Valid, entry.SeoRobotsIndex.Int64),
			RobotsFollow:    seo.NullIntToBoolPtr(entry.SeoRobotsFollow.Valid, entry.SeoRobotsFollow.Int64),
		},
		Path:   path,
		Origin: origin,
	})
	resolved = h.enrichSocialImage(ctx, siteSnap, origin, resolved)
	// Structured data is generated from the same resolved model so the JSON-LD
	// always agrees with the meta tags rendered from it.
	resolved.StructuredData = h.buildStructuredData(ctx, siteSnap, entry, path, origin, resolved)
	feed := seo.Canonical(siteSnap.SiteURL, origin, "/feed.xml", "")
	resolved.Alternates = append(resolved.Alternates, seo.AlternateView{Href: feed, Type: "application/rss+xml"})
	return resolved
}

func (h *Handler) headView(siteSnap *site.Snapshot, resolved seo.Resolved, siteIcon *rendering.FaviconView) themes.HeadView {
	head := themes.HeadView{
		Title:       resolved.Title,
		Description: resolved.Description,
		Canonical:   resolved.Canonical,
		Robots:      resolved.Robots,
		Speculation: themes.SpeculationView{
			Enabled:   siteSnap.SpeculationRulesJSON != "",
			Mode:      siteSnap.SpeculationMode,
			Eagerness: siteSnap.SpeculationEagerness,
			RulesJSON: template.JS(siteSnap.SpeculationRulesJSON),
		},
		SiteIcon: siteIcon,
			OpenGraph: themes.OpenGraphView{
				Title:       resolved.OpenGraph.Title,
				Description: resolved.OpenGraph.Description,
				URL:         resolved.OpenGraph.URL,
				Type:        resolved.OpenGraph.Type,
				Image:       resolved.OpenGraph.Image,
				ImageSecure: resolved.OpenGraph.ImageSecure,
				ImageWidth:  resolved.OpenGraph.ImageWidth,
				ImageHeight: resolved.OpenGraph.ImageHeight,
				ImageType:   resolved.OpenGraph.ImageType,
				ImageAlt:    resolved.OpenGraph.ImageAlt,
				SiteName:    resolved.OpenGraph.SiteName,
				Locale:      resolved.OpenGraph.Locale,
			},
			Twitter: themes.TwitterView{
				Card:        resolved.Twitter.Card,
				Title:       resolved.Twitter.Title,
				Description: resolved.Twitter.Description,
				Image:       resolved.Twitter.Image,
				ImageAlt:    resolved.Twitter.ImageAlt,
				Site:        resolved.Twitter.Site,
			},
			StructuredData: template.JS(resolved.StructuredData),
			Alternates: func() []themes.AlternateView {
				out := make([]themes.AlternateView, 0, len(resolved.Alternates))
				for _, a := range resolved.Alternates {
					out = append(out, themes.AlternateView{Href: a.Href, HrefLang: a.HrefLang, Type: a.Type})
				}
				return out
			}(),
			SEO: themes.SEOView{
				Title:       resolved.Title,
				Description: resolved.Description,
				Canonical:   resolved.Canonical,
				Robots:      resolved.Robots,
				OpenGraph: themes.OpenGraphView{
					Title:       resolved.OpenGraph.Title,
					Description: resolved.OpenGraph.Description,
					URL:         resolved.OpenGraph.URL,
					Type:        resolved.OpenGraph.Type,
					Image:       resolved.OpenGraph.Image,
					ImageSecure: resolved.OpenGraph.ImageSecure,
					ImageWidth:  resolved.OpenGraph.ImageWidth,
					ImageHeight: resolved.OpenGraph.ImageHeight,
					ImageType:   resolved.OpenGraph.ImageType,
					ImageAlt:    resolved.OpenGraph.ImageAlt,
					SiteName:    resolved.OpenGraph.SiteName,
					Locale:      resolved.OpenGraph.Locale,
				},
				Twitter: themes.TwitterView{
					Card:        resolved.Twitter.Card,
					Title:       resolved.Twitter.Title,
					Description: resolved.Twitter.Description,
					Image:       resolved.Twitter.Image,
					ImageAlt:    resolved.Twitter.ImageAlt,
					Site:        resolved.Twitter.Site,
				},
				StructuredData: template.JS(resolved.StructuredData),
				Favicon:        siteIcon,
			},
		}
		return head
	}

// --- The shared editor/preview pipeline (bypasses the page cache) ---

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

// RenderEditableDocument renders an arbitrary document through the exact same
// themed pipeline the live public frontend uses: theme layout, header/footer,
// navigation, identical stylesheet order and RenderContext. It exists so the
// block editor preview matches the published page.
func (h *Handler) RenderEditableDocument(ctx context.Context, input RenderInput) ([]byte, error) {
	siteSnap := h.hub.Site.Current()
	path := input.Path
	if path == "" {
		path = "/"
	}
	rc := rendering.RenderContext{
		Site:  rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
		Entry: rendering.EntryContext{Title: input.Title, Excerpt: input.Excerpt, Permalink: path},
	}
	if siteSnap.LogoMediaID != "" {
		if view, ok := h.media.MediaView(ctx, siteSnap.LogoMediaID); ok {
			rc.Site.LogoURL = view.Src
			rc.Site.LogoWidth = view.Width
			rc.Site.LogoHeight = view.Height
		}
	}
	if len(siteSnap.SocialLinks) > 0 {
		rc.Site.SocialLinks = siteSnap.SocialLinks
	}
	// Preview SEO uses the same resolver so the preview <head> matches the public one.
	previewResolved := h.resolvePreviewSEO(ctx, siteSnap, input, path, "")
	page, _, err := h.renderThemedDocument(ctx, siteSnap, input.Document, rc, previewResolved, path, input.Temporary, nil)
	return page, err
}

func (h *Handler) renderPath(ctx context.Context, path, origin string, temporary map[string]any, customCSS *string) ([]byte, string, error) {
	entry, err := h.queries.GetPublishedEntryByPath(ctx, path)
	if err != nil {
		return nil, "", err
	}
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		return nil, "", fmt.Errorf("decode document: %w", err)
	}
	siteSnap := h.hub.Site.Current()
	resolved := h.resolvePublishedSEO(ctx, siteSnap, &entry, path, origin)
	rc := h.entryRenderContext(siteSnap, &entry, path, resolved)
	page, robots, err := h.renderThemedDocument(ctx, siteSnap, doc, rc, resolved, path, temporary, customCSS)
	return page, robots, err
}

func (h *Handler) entryRenderContext(siteSnap *site.Snapshot, entry *db.GetPublishedEntryByPathRow, path string, resolved seo.Resolved) rendering.RenderContext {
	rc := rendering.RenderContext{
		Site: rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
		Entry: rendering.EntryContext{
			Title:         entry.Title,
			Excerpt:       stringValue(entry.Excerpt),
			Permalink:     path,
			PublishDate:   formatEntryDate(entry.PublishedAt, siteSnap.TimezoneName, false),
			PublishISO:    formatEntryDate(entry.PublishedAt, siteSnap.TimezoneName, true),
			FeaturedImage: resolved.FeaturedMediaID,
		},
	}
	if siteSnap.LogoMediaID != "" {
		if view, ok := h.media.MediaView(context.Background(), siteSnap.LogoMediaID); ok {
			rc.Site.LogoURL = view.Src
			rc.Site.LogoWidth = view.Width
			rc.Site.LogoHeight = view.Height
		}
	}
	if len(siteSnap.SocialLinks) > 0 {
		rc.Site.SocialLinks = siteSnap.SocialLinks
	}
	return rc
}

func (h *Handler) resolvePreviewSEO(ctx context.Context, siteSnap *site.Snapshot, input RenderInput, path, origin string) seo.Resolved {
	resolver := seo.New()
	resolved := resolver.Resolve(seo.Input{
		Site: seo.SiteSEO{
			Title:          siteSnap.SiteTitle,
			TitleSeparator: siteSnap.TitleSeparator,
			SiteURL:        siteSnap.SiteURL,
			Language:       siteSnap.Language,
			// Previews are never indexable: the admin preview routes also send
			// X-Robots-Tag: noindex, nofollow, and forcing it here keeps the
			// rendered <meta name="robots"> consistent with that header.
			IndexingEnabled:     false,
			GlobalSocialMediaID: siteSnap.GlobalSocialMediaID,
			TwitterSite:         siteSnap.TwitterSite,
		},
		Revision: seo.RevisionSEO{
			Title:          input.Title,
			Excerpt:        input.Excerpt,
			SeoTitle:       input.SEOTitle,
			SeoDescription: input.SEODescription,
			CanonicalURL:   "",
		},
		Path:   path,
		Origin: origin,
	})
	return h.enrichSocialImage(ctx, siteSnap, origin, resolved)
}

func (h *Handler) enrichSocialImage(ctx context.Context, siteSnap *site.Snapshot, origin string, resolved seo.Resolved) seo.Resolved {
	if resolved.OGImageID == "" {
		resolved.OpenGraph.Image = ""
		resolved.OpenGraph.ImageWidth = 0
		resolved.OpenGraph.ImageHeight = 0
		resolved.OpenGraph.ImageType = ""
		resolved.OpenGraph.ImageAlt = ""
		resolved.Twitter.Image = ""
		return resolved
	}
	if h.media == nil {
		// No media service in tests: keep provisional absolute URL from resolver.
		return resolved
	}
	view, ok := h.media.SocialView(ctx, resolved.OGImageID)
	if !ok {
		resolved.OpenGraph.Image = ""
		resolved.OpenGraph.ImageWidth = 0
		resolved.OpenGraph.ImageHeight = 0
		resolved.OpenGraph.ImageType = ""
		resolved.OpenGraph.ImageAlt = ""
		resolved.Twitter.Image = ""
		return resolved
	}
	base := seo.BaseURL(siteSnap.SiteURL, origin)
	absURL := base + view.URL
	resolved.OpenGraph.Image = absURL
	if strings.HasPrefix(absURL, "https://") {
		resolved.OpenGraph.ImageSecure = absURL
	}
	resolved.OpenGraph.ImageWidth = view.Width
	resolved.OpenGraph.ImageHeight = view.Height
	resolved.OpenGraph.ImageType = view.Type
	resolved.OpenGraph.ImageAlt = view.Alt
	resolved.Twitter.Image = absURL
	resolved.Twitter.ImageAlt = view.Alt
	return resolved
}

// lcpPreloads emits at most one image preload for the FINAL LCP candidate
// chosen the same way as the renderer: explicit high that exists, then first
// auto that exists. It supports both core/image and core/featured-image
// (the latter resolved via rc.Entry.FeaturedImage).
func (h *Handler) lcpPreloads(ctx context.Context, prepared *rendering.PreparedDocument, rc rendering.RenderContext) []themes.ImagePreload {
	if prepared == nil || h.media == nil {
		return nil
	}
	hasFeatured := rc.Entry.FeaturedImage != ""
	candID := prepared.ResolveLCP(hasFeatured)
	if candID == "" {
		return nil
	}
	var find func([]rendering.PreparedNode) *rendering.PreparedNode
	find = func(nodes []rendering.PreparedNode) *rendering.PreparedNode {
		for i := range nodes {
			if nodes[i].ID == candID {
				return &nodes[i]
			}
			if child := find(nodes[i].Children); child != nil {
				return child
			}
		}
		return nil
	}
	node := find(prepared.Nodes)
	if node == nil {
		return nil
	}
	var mediaID string
	if node.Block == "core/featured-image" {
		mediaID = rc.Entry.FeaturedImage
	} else if node.Block == "core/image" {
		mediaID, _ = node.Props["mediaId"].(string)
	}
	if mediaID == "" {
		return nil
	}
	view, ok := h.media.MediaView(ctx, mediaID)
	if !ok || view.Src == "" {
		return nil
	}
	sizes, _ := node.Settings["sizes"].(string)
	if strings.TrimSpace(sizes) == "" {
		sizes = "(min-width: 768px) min(100vw, 1200px), 100vw"
	}
	return []themes.ImagePreload{{
		Href:   view.Src,
		SrcSet: view.SrcSet,
		Sizes:  sizes,
	}}
}

// renderThemedDocument renders a document as a fully themed page: it prepares
// the document, loads navigation, assembles the PageView and runs the theme
// runtime. The live public frontend and the editor previews share this exact
// path, so they cannot drift apart.
func (h *Handler) renderThemedDocument(ctx context.Context, siteSnap *site.Snapshot, doc *document.Document, rc rendering.RenderContext, resolved seo.Resolved, path string, temporary map[string]any, customCSS *string) ([]byte, string, error) {
	prepared, err := h.blocks.Prepare(doc)
	if err != nil {
		return nil, "", fmt.Errorf("prepare document: %w", err)
	}
	content, err := h.blocks.RenderPrepared(ctx, prepared, rc)
	if err != nil {
		return nil, "", fmt.Errorf("render document: %w", err)
	}
	menus := h.hub.Navigation.LocationsForPath(path)
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, resolved, siteIcon)
	head.Preloads = h.lcpPreloads(ctx, prepared, rc)

	_, themeCSS, themeJS := h.hub.Assets.URLs()
	blocksCSS := h.hub.Assets.BlocksCSSFor(prepared.UsedBlocks)
	view := themes.PageView{
		Site:       themes.SiteView{Title: rc.Site.Name, Tagline: rc.Site.Tagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: rc.Site.LogoURL, LogoWidth: rc.Site.LogoWidth, LogoHeight: rc.Site.LogoHeight},
		Entry:      themes.EntryView{Title: rc.Entry.Title, SEOTitle: resolved.OpenGraph.Title, SEODescription: resolved.Description, CanonicalURL: resolved.Canonical},
		Head:       head,
		Navigation: menus,
		Content:    content,
		Assets:     themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}
	if customCSS != nil {
		page, err := h.themes.Preview(view, temporary, *customCSS)
		return page, resolved.Robots, err
	}
	page, err := h.themes.Render(view, temporary)
	return page, resolved.Robots, err
}

func (h *Handler) serveSitemap(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil || siteSnap.SitemapEnabled == false || strings.TrimSpace(siteSnap.SiteURL) == "" {
		http.NotFound(w, r)
		return
	}
	body, gz, etag, ok := h.hub.Sitemap.Get()
	if !ok {
		built, err := h.buildSitemap(r.Context(), siteSnap)
		if err != nil {
			log.Printf("sitemap build: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		gz, _ = gzipBytes(built)
		etag = pagecache.ComputeETag(built)
		h.hub.Sitemap.Set(built, gz, etag)
		body = built
	}
	h.writeText(w, r, body, gz, etag, "application/xml; charset=utf-8", "public, max-age=300")
}

func (h *Handler) buildSitemap(ctx context.Context, siteSnap *site.Snapshot) ([]byte, error) {
	urlset := sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	// When site-wide indexing is disabled every page resolves to noindex, so
	// the sitemap contains no indexable URLs at all.
	if !siteSnap.IndexingEnabled {
		return h.marshalSitemap(urlset)
	}
	entries, err := h.queries.ListSitemapEntries(ctx)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(siteSnap.SiteURL, "/")
	for _, entry := range entries {
		urlset.URLs = append(urlset.URLs, sitemapURL{
			Loc:     base + entry.RoutePath,
			Lastmod: time.Unix(entry.Lastmod, 0).UTC().Format(time.RFC3339),
		})
	}
	// Add the main posts archive (page 1 only)
	if siteSnap.IndexingEnabled {
		arch := seo.PostsArchivePath(siteSnap.PostsBasePath)
		if siteSnap.HomepageMode == "latest_posts" {
			arch = "/"
		}
		urlset.URLs = append(urlset.URLs, sitemapURL{
			Loc:     base + arch,
			Lastmod: time.Now().UTC().Format(time.RFC3339), // archive lastmod is soft; posts inside dominate
		})
	}
	return h.marshalSitemap(urlset)
}

func (h *Handler) marshalSitemap(urlset sitemapURLSet) ([]byte, error) {
	body, err := xml.Marshal(urlset)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func (h *Handler) serveRobots(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	body, gz, etag, ok := h.hub.Robots.Get()
	if !ok {
		built := site.BuildRobots(site.RobotsInput{
			Mode:            siteSnap.RobotsMode,
			IndexingEnabled: siteSnap.IndexingEnabled,
			SitemapEnabled:  siteSnap.SitemapEnabled,
			SiteURL:         siteSnap.SiteURL,
			Custom:          siteSnap.RobotsCustom,
		})
		gz, _ = gzipBytes([]byte(built))
		etag = pagecache.ComputeETag([]byte(built))
		h.hub.Robots.Set([]byte(built), gz, etag)
		body = []byte(built)
	}
	h.writeText(w, r, body, gz, etag, "text/plain; charset=utf-8", "public, max-age=300")
}

func (h *Handler) writeText(w http.ResponseWriter, r *http.Request, body, gz []byte, etag, ctype, cacheControl string) {
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", cacheControl)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	if acceptGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"); acceptGzip && len(gz) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(gz)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(gz)
		}
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// serveMedia streams a stored media derivative using Range-capable serving.
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
	f, size, mime, err := h.media.OpenVariant(r.Context(), id, kind)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, id+kind, time.Time{}, f)
	_ = size
}

// serveFavicon redirects the legacy /favicon.ico to the immutable favicon media
// variant, so the mutable URL is never cached as immutable.
func (h *Handler) serveFavicon(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil || siteSnap.SiteIconMediaID == "" {
		http.NotFound(w, r)
		return
	}
	target := "/media/" + siteSnap.SiteIconMediaID + "/favicon-32"
	if r.URL.Path != target {
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	h.serveMedia(w, r)
}

// siteIconView resolves the configured Site Icon into favicon links, or nil.
func (h *Handler) siteIconView(ctx context.Context, siteSnap *site.Snapshot) *rendering.FaviconView {
	if siteSnap == nil || siteSnap.SiteIconMediaID == "" {
		return nil
	}
	view, ok := h.media.FaviconView(ctx, siteSnap.SiteIconMediaID)
	if !ok {
		return nil
	}
	return &view
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// RenderInput describes an arbitrary document to render through the public
// pipeline outside of a published entry (the block editor preview).
type RenderInput struct {
	Document       *document.Document
	Title          string
	Excerpt        string
	SEOTitle       string
	SEODescription string
	Path           string
	Temporary      map[string]any
	CustomCSS      string
}

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := gw.Write(b); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func stringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

// parseArchivePagination returns the base archive path and page number for
// paths like /blog/page/3 or /page/2 . Returns ok=false for non pagination.
func parseArchivePagination(path string) (base string, page int, ok bool) {
	path = strings.TrimSuffix(path, "/")
	if strings.HasSuffix(path, "/page/1") {
		base = strings.TrimSuffix(path, "/page/1")
		if base == "" {
			base = "/"
		}
		return base, 1, true
	}
	if idx := strings.LastIndex(path, "/page/"); idx != -1 {
		suffix := path[idx+6:]
		if n, err := strconv.Atoi(suffix); err == nil && n > 1 {
			base = path[:idx]
			if base == "" {
				base = "/"
			}
			return base, n, true
		}
	}
	return "", 0, false
}

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

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod"`
}
