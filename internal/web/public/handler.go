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
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/comments"
	"github.com/kokosx/stratum/internal/compress"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/pagecache"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/routing"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/seo"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

var testRouteRegistry sync.Map // maps *db.Queries -> *Handler for test auto-reload

type formSuccessContextKey struct{}

type Handler struct {
	queries        *db.Queries
	blocks         *blocks.Registry
	themes         *themes.Runtime
	media          *media.Service
	hub            *runtimehub.Runtime
	layoutsService *layouts.Service
	dev            bool
	// warnNoSiteURL guards the one-time production warning about canonical
	// URLs falling back to the request Host.
	warnNoSiteURL sync.Once
	unlockStore   *publishing.UnlockStore
	unlockLimiter *publishing.UnlockLimiter
	search        *search.Service
	comments      *comments.Service
	forms         *forms.Service
	auth          *auth.Service
}

func NewHandler(queries *db.Queries, blocksReg *blocks.Registry, runtime *themes.Runtime, mediaService *media.Service) (*Handler, error) {
	hub, err := runtimehub.New(queries, blocksReg, runtime, mediaService)
	if err != nil {
		return nil, err
	}
	h, err := NewHandlerWithHub(hub)
	if err != nil {
		return nil, err
	}
	testRouteRegistry.Store(queries, h)
	return h, nil
}

// NewHandlerWithHub builds a public handler around a shared runtime coordinator
// so admin write paths and the public frontend share the same caches.
func NewHandlerWithHub(hub *runtimehub.Runtime) (*Handler, error) {
	h := &Handler{
		queries:        hub.Queries,
		blocks:         hub.Blocks,
		themes:         hub.Themes,
		media:          hub.Media,
		hub:            hub,
		layoutsService: layouts.NewService(nil, hub.Queries, hub.Blocks),
		dev:            os.Getenv("STRATUM_ENV") != "production",
		unlockStore:    publishing.NewUnlockStore(),
		unlockLimiter:  publishing.NewUnlockLimiter(),
	}
	if database, ok := hub.Queries.DB().(*sql.DB); ok {
		h.search = search.New(database, hub.Blocks)
		h.comments = comments.NewService(database, hub.Queries)
		h.comments.SetInvalidator(func(entryID string) {
			hub.Pages.InvalidateTag("entry:" + entryID)
		})
		h.forms = hub.Forms
		// Public comment submission accepts the same signed-in session as admin.
		// Failure to initialize auth only leaves public comments anonymous.
		h.auth, _ = auth.NewService(database, hub.Queries, !h.dev)
	}
	testRouteRegistry.Store(hub.Queries, h)
	// Targeted warmup: homepage and main archive page 1 if available.
	// Best effort; failures are logged inside WarmCache.
	h.WarmCache(context.Background())
	return h, nil
}

// Hub exposes the shared runtime coordinator so callers can invalidate caches.
func (h *Handler) Hub() *runtimehub.Runtime { return h.hub }

// AssetURLs returns the current fingerprinted asset URLs.
func (h *Handler) AssetURLs() (blocksCSS, themeCSS, themeJS string) {
	return h.hub.Assets.URLs()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Password-protected pages accept POST for unlock form.
	if r.Method == http.MethodPost {
		if strings.HasPrefix(r.URL.Path, "/_stratum/forms/") {
			h.handleFormSubmit(w, r)
			return
		}
		if r.URL.Path == "/comments" {
			h.handleCommentSubmit(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/media/") {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		// Only allow POST for password-protected entry routes; other POSTs remain 405.
		if h.isPasswordProtectedPath(r.Context(), r.URL.Path) {
			h.servePasswordPost(w, r)
			return
		}
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
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
	if r.URL.Path == "/search" {
		h.serveSearch(w, r)
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
	if successID := strings.TrimSpace(r.URL.Query().Get("form_success")); successID != "" {
		ctx := context.WithValue(r.Context(), formSuccessContextKey{}, successID)
		r = r.WithContext(ctx)
		entry, err := h.renderPage(ctx, origin, r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		h.writePage(w, r, entry)
		return
	}
	// When canonical depends on the request origin (no configured Site URL), the
	// cache key must include the origin so HTML for the wrong host is never served.
	key := pagecache.Key("", r.URL.Path)
	if siteSnap == nil || siteSnap.SiteURL == "" {
		key = pagecache.Key(origin, r.URL.Path)
	}

	// Canonical pagination: /blog/page/1 -> 301 /blog, /page/1 -> 301 / . Checked
	// before cache so the non-canonical URL is never cached. Use the single
	// routing.ParsePagination helper (not a duplicate).
	if base, pg, ok := routing.ParsePagination(r.URL.Path); ok && pg == 1 {
		isArchive := false
		if base == "/" {
			if siteSnap != nil && siteSnap.HomepageMode == "latest_posts" {
				isArchive = true
			}
		} else {
			if rt, ok := h.hub.Routes.Lookup(base); ok && rt.RouteType == routing.RouteTypeArchive {
				isArchive = true
			}
		}
		if isArchive {
			http.Redirect(w, r, base, http.StatusMovedPermanently)
			return
		}
	}

	// Password-protected entry routes bypass the shared full-page cache entirely.
	// The route snapshot carries the immutable visibility so we can decide before the cache lookup (zero DB for normal public).
	normalizedPath := routing.NormalizePath(r.URL.Path)
	if rt, ok := h.hub.Routes.Lookup(normalizedPath); ok && rt.RouteType == routing.RouteTypeEntry && rt.EntryID.Valid {
		if rt.Visibility == "private" {
			http.NotFound(w, r)
			return
		}
		// Snapshot is the security boundary: if snapshot says password, never serve from shared cache.
		if rt.Visibility == "password" {
			// Verify via DB (DB is source of truth) but bypass cache regardless.
			if row, err := h.queries.GetPublishedEntryByID(r.Context(), rt.EntryID.String); err == nil && row.Visibility == "password" {
				h.servePasswordPage(w, r, row, normalizedPath, origin)
				return
			}
			// Snapshot stale (now public): fall through to normal cache path.
		} else if rt.Visibility == "" {
			// Never use a shared cached response without security metadata. This
			// only occurs for a stale/malformed route row; resolve it from the DB.
			row, err := h.queries.GetPublishedEntryByID(r.Context(), rt.EntryID.String)
			if err != nil || row.Visibility == "private" {
				http.NotFound(w, r)
				return
			}
			if row.Visibility == "password" {
				h.servePasswordPage(w, r, row, normalizedPath, origin)
				return
			}
			entry, err := h.renderPage(r.Context(), origin, r.URL.Path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			h.writePage(w, r, entry)
			return
		}
		// For public visibility, proceed to cache; private has no route.
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
	// page-cache miss so a warm cache never pays for DB.
	if route, ok := h.hub.Routes.Lookup(r.URL.Path); ok && route.RouteType == "redirect" && route.RedirectTo.Valid && route.RedirectTo.String != "" {
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

func (h *Handler) isPasswordProtectedPath(ctx context.Context, path string) bool {
	normalized := routing.NormalizePath(path)
	snap := h.hub.Routes.Current()
	if snap != nil {
		if rt, ok := snap.ByPath[normalized]; ok && rt.RouteType == routing.RouteTypeEntry && rt.EntryID.Valid {
			if rt.Visibility == "password" {
				return true
			}
			if rt.Visibility != "" {
				return false
			}
			// Fallback for snapshot without visibility (stale): check DB.
			if row, err := h.queries.GetPublishedEntryByID(ctx, rt.EntryID.String); err == nil && row.Visibility == "password" {
				return true
			}
		}
		return false
	}
	// Fallback when snapshot not loaded: check DB directly
	if row, err := h.queries.GetPublishedEntryByPath(ctx, normalized); err == nil && row.Visibility == "password" {
		return true
	}
	return false
}

func (h *Handler) servePasswordPage(w http.ResponseWriter, r *http.Request, row db.GetPublishedEntryByIDRow, path, origin string) {
	// Check unlock cookie
	cookieName := h.unlockStore.CookieName(row.ID)
	now := time.Now().Unix()
	if cookie, err := r.Cookie(cookieName); err == nil {
		if h.unlockStore.Valid(cookie.Value, row.ID, row.RevisionID, now) {
			// Unlock valid: render entry content with private no-store headers (bypass global cache)
			entry := db.GetPublishedEntryByPathRow{
				ID: row.ID, ContentTypeID: row.ContentTypeID, Slug: row.Slug, Status: row.Status,
				PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID,
				Title: row.Title, Excerpt: row.Excerpt, DocumentJson: row.DocumentJson, FieldsJson: row.FieldsJson,
				SeoTitle: row.SeoTitle, SeoDescription: row.SeoDescription, CanonicalUrl: row.CanonicalUrl,
				FeaturedMediaID: row.FeaturedMediaID, SocialMediaID: row.SocialMediaID,
				SeoRobotsIndex: row.SeoRobotsIndex, SeoRobotsFollow: row.SeoRobotsFollow, SchemaMode: row.SchemaMode,
				LayoutTemplateID: row.LayoutTemplateID, Visibility: row.Visibility, PasswordHash: row.PasswordHash,
				Sticky: row.Sticky, ReviewState: row.ReviewState,
			}
			siteSnap := h.hub.Site.Current()
			if siteSnap == nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			page, err := h.renderEntry(r.Context(), origin, path, entry, siteSnap)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.NotFound(w, r)
					return
				}
				log.Printf("render password unlocked page: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			// Write with private no-store (never cache)
			w.Header().Set("Content-Type", page.ContentType)
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Vary", "Accept-Encoding")
			if page.Robots != "" {
				w.Header().Set("X-Robots-Tag", page.Robots)
			}
			// ETag still for conditional but private
			w.Header().Set("ETag", page.ETag)
			if etagWeakMatch(r.Header.Get("If-None-Match"), page.ETag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			enc, ok := compress.NegotiateEncoding(r.Header.Get("Accept-Encoding"))
			if !ok {
				http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
				return
			}
			switch enc {
			case "br":
				if len(page.Brotli) > 0 {
					w.Header().Set("Content-Encoding", "br")
					w.Header().Set("Content-Length", strconv.Itoa(len(page.Brotli)))
					if r.Method != http.MethodHead {
						_, _ = w.Write(page.Brotli)
					}
					return
				}
			case "gzip":
				if len(page.Gzip) > 0 {
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Content-Length", strconv.Itoa(len(page.Gzip)))
					if r.Method != http.MethodHead {
						_, _ = w.Write(page.Gzip)
					}
					return
				}
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(page.HTML)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(page.HTML)
			}
			return
		}
	}
	// Not unlocked: serve gate
	h.servePasswordGate(w, r, row, "", false)
}

func (h *Handler) servePasswordGate(w http.ResponseWriter, r *http.Request, row db.GetPublishedEntryByIDRow, errorMsg string, rateLimited bool) {
	siteSnap := h.hub.Site.Current()
	origin := requestOrigin(r)
	_ = siteSnap
	_ = origin
	gateContent := `<div class="stratum-password-gate" style="max-width:480px;margin:60px auto;padding:24px;border:1px solid #dcdcde;background:#fff;">` +
		`<h1 style="margin:0 0 12px;">Protected Content</h1>` +
		`<p>This content is password protected.</p>` +
		`<form method="post" action="` + template.HTMLEscapeString(r.URL.Path) + `">` +
		`<label>Password<input type="password" name="stratum_password" required style="width:100%;padding:8px;border:1px solid #8c8f94;border-radius:3px;margin:8px 0;"></label>` +
		`<button type="submit" class="button button-primary" style="padding:8px 16px;background:#2271b1;color:#fff;border:none;border-radius:3px;cursor:pointer;">Unlock</button>` +
		`</form>`
	if errorMsg != "" {
		gateContent += `<p style="color:#b42318;margin-top:12px;">` + template.HTMLEscapeString(errorMsg) + `</p>`
	}
	if rateLimited {
		gateContent += `<p style="color:#b42318;">Too many attempts. Please try again later.</p>`
	}
	gateContent += `</div>`

	html := []byte(`<!doctype html><html><head><meta charset="utf-8"><title>Protected</title></head><body>` + gateContent + `</body></html>`)
	gz, _ := compress.Gzip(html)
	br, _ := compress.Brotli(html)
	etag := pagecache.ComputeETag(html)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("ETag", etag)
	w.Header().Set("Vary", "Accept-Encoding")
	// Never cache gate in global cache
	if h.dev {
		w.Header().Set("Server-Timing", "cache;desc=\"miss\"")
	}
	enc, ok := compress.NegotiateEncoding(r.Header.Get("Accept-Encoding"))
	if !ok {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
		return
	}
	switch enc {
	case "br":
		if len(br) > 0 {
			w.Header().Set("Content-Encoding", "br")
			w.Header().Set("Content-Length", strconv.Itoa(len(br)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(br)
			}
			return
		}
	case "gzip":
		if len(gz) > 0 {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(len(gz)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(gz)
			}
			return
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(html)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(html)
	}
}

func (h *Handler) servePasswordPost(w http.ResponseWriter, r *http.Request) {
	normalized := routing.NormalizePath(r.URL.Path)
	snap := h.hub.Routes.Current()
	var entryID string
	if snap != nil {
		if rt, ok := snap.ByPath[normalized]; ok && rt.RouteType == routing.RouteTypeEntry && rt.EntryID.Valid {
			entryID = rt.EntryID.String
		}
	}
	if entryID == "" {
		// Fallback DB lookup
		if row, err := h.queries.GetPublishedEntryByPath(r.Context(), normalized); err == nil {
			entryID = row.ID
		}
	}
	if entryID == "" {
		http.NotFound(w, r)
		return
	}
	row, err := h.queries.GetPublishedEntryByID(r.Context(), entryID)
	if err != nil || row.Visibility != "password" {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	password := r.FormValue("stratum_password")
	if password == "" {
		password = r.FormValue("password")
	}
	// Use RemoteAddr only; do not trust X-Forwarded-For without trusted proxy.
	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	} else {
		// Fallback: trim without splitting (e.g. already bare IP or malformed)
		clientIP = strings.TrimSpace(clientIP)
		if idx := strings.LastIndex(clientIP, ":"); idx != -1 && strings.Count(clientIP, ":") == 1 {
			clientIP = clientIP[:idx]
		}
	}
	clientIP = strings.TrimSpace(clientIP)
	limiterKey := publishing.ClientKey(entryID, clientIP)
	now := time.Now().Unix()
	if !h.unlockLimiter.Allow(limiterKey, now) {
		h.servePasswordGate(w, r, row, "", true)
		return
	}
	// Verify password hash
	if row.PasswordHash.String == "" || !publishing.CheckPassword(row.PasswordHash.String, password) {
		h.unlockLimiter.Record(limiterKey, false, now)
		h.servePasswordGate(w, r, row, "Invalid password.", false)
		return
	}
	h.unlockLimiter.Record(limiterKey, true, now)
	// Success: create token; CSPRNG failure must not issue cookie.
	token, expires, err := h.unlockStore.Create(row.ID, row.RevisionID, now)
	if err != nil {
		log.Printf("password unlock token creation failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	cookieName := h.unlockStore.CookieName(row.ID)
	secure := r.TLS != nil
	if siteSnap := h.hub.Site.Current(); siteSnap != nil && strings.HasPrefix(strings.TrimSpace(strings.ToLower(siteSnap.SiteURL)), "https://") {
		secure = true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expires, 0),
		MaxAge:   int(expires - now),
	})
	// Redirect GET back to same canonical URL (Post/Redirect/Get)
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}

func etagWeakMatch(header, etag string) bool {
	if header == "" || etag == "" {
		return false
	}
	if header == "*" {
		return true
	}
	// Handle list of ETags: "W/\"a\", \"b\""
	// For simplicity, split by comma and trim
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimSpace(part)
		// Weak comparison: strip leading W/
		if strings.HasPrefix(p, "W/") {
			p = p[2:]
		}
		e := etag
		if strings.HasPrefix(e, "W/") {
			e = e[2:]
		}
		if p == e {
			return true
		}
	}
	return false
}

func (h *Handler) writePage(w http.ResponseWriter, r *http.Request, entry pagecache.Entry) {
	// HTML must revalidate: the public URL is stable across Publish, so a long
	// immutable max-age would freeze stale content in browsers/CDNs. no-cache
	// still allows storing the response; clients must revalidate via ETag.
	htmlCacheControl := "no-cache"
	if _, ok := r.Context().Value(formSuccessContextKey{}).(string); ok {
		htmlCacheControl = "no-store"
	}

	if etagWeakMatch(r.Header.Get("If-None-Match"), entry.ETag) {
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

	enc, ok := compress.NegotiateEncoding(r.Header.Get("Accept-Encoding"))
	if !ok {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
		return
	}
	switch enc {
	case "br":
		if len(entry.Brotli) > 0 {
			w.Header().Set("Content-Encoding", "br")
			w.Header().Set("Content-Length", strconv.Itoa(len(entry.Brotli)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(entry.Brotli)
			}
			return
		}
	case "gzip":
		if len(entry.Gzip) > 0 {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(len(entry.Gzip)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(entry.Gzip)
			}
			return
		}
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

	normalized := routing.NormalizePath(path)
	snap := h.hub.Routes.Current()

	// Pagination child: /blog/page/2 or /page/2
	if base, page, ok := routing.ParsePagination(normalized); ok {
		if page < 1 {
			return pagecache.Entry{}, sql.ErrNoRows
		}
		isArchive := false
		if base == "/" {
			if siteSnap.HomepageMode == "latest_posts" {
				isArchive = true
			}
		} else if snap != nil {
			if rt, ok2 := snap.ByPath[base]; ok2 && rt.RouteType == routing.RouteTypeArchive {
				isArchive = true
			}
		}
		if isArchive {
			// canonical /blog/page/1 already handled in serveCachedPage, but also handle here
			if page == 1 {
				return pagecache.Entry{}, sql.ErrNoRows
			}
			return h.renderArchivePage(ctx, origin, base, page, siteSnap)
		}
	}

	// Homepage latest_posts: "/" is the posts archive (snapshot may still contain old entry route, but mode wins).
	if normalized == "/" && siteSnap.HomepageMode == "latest_posts" {
		return h.renderArchivePage(ctx, origin, "/", 1, siteSnap)
	}

	// Exact route lookup via immutable snapshot (zero DB).
	if snap != nil {
		if rt, ok := snap.ByPath[normalized]; ok {
			switch rt.RouteType {
			case routing.RouteTypeRedirect:
				return pagecache.Entry{}, sql.ErrNoRows
			case routing.RouteTypeArchive:
				return h.renderArchivePage(ctx, origin, normalized, 1, siteSnap)
			case routing.RouteTypeEntry:
				return h.renderEntryByRoute(ctx, origin, normalized, rt, siteSnap)
			case routing.RouteTypeSystem:
				return pagecache.Entry{}, sql.ErrNoRows
			}
		}
	}

	// Route snapshot is authoritative when loaded (even empty). A miss means 404 without DB.
	// Only fall back to DB if the runtime is genuinely not loaded yet.
	if snap == nil {
		entry, err2 := h.queries.GetPublishedEntryByPath(ctx, normalized)
		if err2 != nil {
			return pagecache.Entry{}, err2
		}
		return h.renderEntry(ctx, origin, normalized, entry, siteSnap)
	}
	return pagecache.Entry{}, sql.ErrNoRows
}

func (h *Handler) renderEntryByRoute(ctx context.Context, origin, path string, route routing.Route, siteSnap *site.Snapshot) (pagecache.Entry, error) {
	if !route.EntryID.Valid {
		return pagecache.Entry{}, sql.ErrNoRows
	}
	row, err := h.queries.GetPublishedEntryByID(ctx, route.EntryID.String)
	if err != nil {
		return pagecache.Entry{}, err
	}
	entry := db.GetPublishedEntryByPathRow{
		ID:               row.ID,
		ContentTypeID:    row.ContentTypeID,
		Slug:             row.Slug,
		Status:           row.Status,
		PublishedAt:      row.PublishedAt,
		FirstPublishedAt: row.FirstPublishedAt,
		RevisionID:       row.RevisionID,
		Title:            row.Title,
		Excerpt:          row.Excerpt,
		DocumentJson:     row.DocumentJson,
		FieldsJson:       row.FieldsJson,
		SeoTitle:         row.SeoTitle,
		SeoDescription:   row.SeoDescription,
		CanonicalUrl:     row.CanonicalUrl,
		FeaturedMediaID:  row.FeaturedMediaID,
		SocialMediaID:    row.SocialMediaID,
		SeoRobotsIndex:   row.SeoRobotsIndex,
		SeoRobotsFollow:  row.SeoRobotsFollow,
		SchemaMode:       row.SchemaMode,
		LayoutTemplateID: row.LayoutTemplateID,
		Visibility:       row.Visibility,
		PasswordHash:     row.PasswordHash,
		Sticky:           row.Sticky,
		ReviewState:      row.ReviewState,
	}
	return h.renderEntry(ctx, origin, path, entry, siteSnap)
}

func (h *Handler) renderEntry(ctx context.Context, origin, path string, entry db.GetPublishedEntryByPathRow, siteSnap *site.Snapshot) (pagecache.Entry, error) {
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("decode document: %w", err)
	}
	fields, err := content.DecodeFieldSnapshot(entry.FieldsJson)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("decode revision fields: %w", err)
	}
	// Layout composition is validated inside the layout application boundary
	// (Service.ResolveEffectiveDocument calls blocks.ValidateDocument on the
	// composed SDT). Handlers do not replicate that logic.
	effectiveDoc, layoutRevID, err := h.layoutsService.ResolveEffectiveDocument(ctx, doc, entry.ContentTypeID, entry.LayoutTemplateID)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("resolve layout template: %w", err)
	}
	cacheKey := entry.RevisionID
	if layoutRevID != "" {
		cacheKey = entry.RevisionID + ":" + layoutRevID
	}
	prepared, err := h.blocks.PreparedCache(cacheKey, effectiveDoc)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("prepare document: %w", err)
	}
	resolved := h.resolvePublishedSEO(ctx, siteSnap, &entry, path, origin)
	rc := h.entryRenderContext(siteSnap, &entry, path, resolved, fields)
	// Generic scoped context for Collection blocks.
	rc.Route = rendering.RouteContext{Path: path, IsArchive: false}
	rc.Mode = rendering.ModePublic
	rc.ContentReader = &handlerContentReader{queries: h.queries, siteSnap: siteSnap, media: h.media}
	rc.QueryCache = make(map[string][]rendering.ArchiveEntry)
	rc.EntryID = entry.ID
	rc.LCP = &rendering.LCPState{}
	if def, err := content.NewCatalog(h.queries).GetDefinition(ctx, entry.ContentTypeID); err == nil {
		rc.Definition = def
	} else {
		rc.Definition = content.DefinitionFor(entry.ContentTypeID)
	}
	rc.SitePartReader = newHandlerSitePartReader(h.queries, h.blocks)
	rc.CollectedSiteParts = make(map[string]string)
	rc.Dependencies = &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)}
	rc.SitePartStack = make(map[string]struct{})
	rc.SitePartDepth = 0
	menus := h.hub.Navigation.LocationsForPath(path)
	rc.Navigation = menus
	// renderEntry runs only after the entry visibility gate; it is the normal
	// public rendering path and must provide comments before blocks render.
	h.populateCommentsContext(ctx, &rc, entry.ID, true)
	content, err := h.renderBlocks(ctx, prepared, rc)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("render document: %w", err)
	}
	// Header/Footer site parts (theme regions)
	headerHTML, footerHTML, hfUsed := h.renderHeaderFooter(ctx, siteSnap, path, rc)
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, resolved, siteIcon)
	head.Preloads = h.lcpPreloads(ctx, prepared, rc)

	_, themeCSS, themeJS := h.hub.Assets.URLs()
	usedBlocks := append([]rendering.BlockKey(nil), prepared.UsedBlocks...)
	usedBlocks = append(usedBlocks, hfUsed...)
	// Add used blocks discovered while resolving referenced Site Parts.
	for _, dependency := range rc.Dependencies.SiteParts {
		usedBlocks = append(usedBlocks, dependency.UsedBlocks...)
	}
	// Deduplicate used blocks
	seenBlocks := make(map[rendering.BlockKey]struct{}, len(usedBlocks))
	deduped := make([]rendering.BlockKey, 0, len(usedBlocks))
	for _, k := range usedBlocks {
		if _, ok := seenBlocks[k]; !ok {
			seenBlocks[k] = struct{}{}
			deduped = append(deduped, k)
		}
	}
	blocksCSS := h.hub.Assets.BlocksCSSFor(deduped)
	kind := themes.PageKindSingle
	isFront := path == "/"
	regions := make(map[string]template.HTML)
	if len(headerHTML) > 0 {
		regions["header"] = headerHTML
	}
	if len(footerHTML) > 0 {
		regions["footer"] = footerHTML
	}
	view := themes.PageView{
		Site:        themes.SiteView{Title: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: rc.Site.LogoURL, LogoWidth: rc.Site.LogoWidth, LogoHeight: rc.Site.LogoHeight},
		Entry:       themes.EntryView{Title: entry.Title, SEOTitle: stringValue(entry.SeoTitle), SEODescription: resolved.Description, CanonicalURL: resolved.Canonical},
		Head:        head,
		Navigation:  menus,
		Content:     content,
		ContentType: entry.ContentTypeID,
		Kind:        kind,
		IsFrontPage: isFront,
		Header:      headerHTML,
		Footer:      footerHTML,
		Regions:     regions,
		Assets:      themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}
	html, err := h.themes.Render(view, nil)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("theme render: %w", err)
	}
	gz, err := compress.Gzip(html)
	if err != nil {
		gz = nil
	}
	br, err := compress.Brotli(html)
	if err != nil {
		br = nil
	}
	tags := []string{"entry:" + entry.ID, "site", "navigation", "theme"}
	if entry.LayoutTemplateID.Valid && entry.LayoutTemplateID.String != "" {
		tags = append(tags, "layout:"+entry.LayoutTemplateID.String)
	}
	// Template tag for composed layout revision already via layout ID; for cache invalidation also include template:ID
	if layoutRevID != "" {
		tID := ""
		if entry.LayoutTemplateID.Valid && entry.LayoutTemplateID.String != "" {
			tID = entry.LayoutTemplateID.String
		} else if ct, err := h.queries.GetContentType(ctx, entry.ContentTypeID); err == nil && ct.DefaultLayoutTemplateID.Valid {
			tID = ct.DefaultLayoutTemplateID.String
		}
		if tID != "" {
			tags = append(tags, "template:"+tID)
		}
	}
	for _, ct := range collectionContentTypes(prepared) {
		tags = append(tags, "content-type:"+ct)
	}
	for sid := range rc.CollectedSiteParts {
		tags = append(tags, "site-part:"+sid)
	}
	// Deduplicate tags
	seenTags := make(map[string]struct{}, len(tags))
	uniqTags := make([]string, 0, len(tags))
	for _, t := range tags {
		if _, ok := seenTags[t]; !ok {
			seenTags[t] = struct{}{}
			uniqTags = append(uniqTags, t)
		}
	}
	return pagecache.Entry{
		HTML:        html,
		Gzip:        gz,
		Brotli:      br,
		ETag:        pagecache.ComputeETag(html),
		Robots:      resolved.Robots,
		ContentType: "text/html; charset=utf-8",
		Tags:        uniqTags,
	}, nil
}

func (h *Handler) renderHeaderFooter(ctx context.Context, siteSnap *site.Snapshot, path string, baseRC rendering.RenderContext) (template.HTML, template.HTML, []rendering.BlockKey) {
	var headerHTML, footerHTML template.HTML
	var used []rendering.BlockKey
	if baseRC.CollectedSiteParts == nil {
		baseRC.CollectedSiteParts = make(map[string]string)
	}
	if baseRC.Dependencies == nil {
		baseRC.Dependencies = &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)}
	}
	renderLoc := func(loc string) (template.HTML, *rendering.PreparedDocument, string) {
		row, err := h.queries.GetSitePartLocation(ctx, loc)
		if err != nil || !row.SitePartID.Valid || row.SitePartID.String == "" {
			return "", nil, ""
		}
		pd, revID, err := baseRC.SitePartReader.GetSitePart(ctx, row.SitePartID.String)
		if err != nil || pd == nil {
			return "", nil, ""
		}
		baseRC.CollectedSiteParts[row.SitePartID.String] = revID
		menus := h.hub.Navigation.LocationsForPath(path)
		siteCtx := rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL}
		if siteSnap.LogoMediaID != "" && h.media != nil {
			if view, ok := h.media.MediaView(ctx, siteSnap.LogoMediaID); ok {
				siteCtx.LogoURL = view.Src
				siteCtx.LogoWidth = view.Width
				siteCtx.LogoHeight = view.Height
			}
		}
		if len(siteSnap.SocialLinks) > 0 {
			siteCtx.SocialLinks = siteSnap.SocialLinks
		}
		rc := rendering.RenderContext{
			Site:               siteCtx,
			Route:              baseRC.Route,
			Mode:               rendering.ModePublic,
			ContentReader:      baseRC.ContentReader,
			QueryCache:         baseRC.QueryCache,
			SitePartReader:     baseRC.SitePartReader,
			CollectedSiteParts: baseRC.CollectedSiteParts,
			Dependencies:       baseRC.Dependencies,
			SitePartStack:      map[string]struct{}{row.SitePartID.String: {}},
			SitePartDepth:      0,
			LCP:                &rendering.LCPState{},
			Navigation:         menus,
		}
		if baseRC.Route.IsArchive && baseRC.Route.Archive != nil {
			rc.Archive = baseRC.Route.Archive
			rc.Route = baseRC.Route
		}
		html, err := h.renderBlocks(ctx, pd, rc)
		if err != nil {
			log.Printf("render site-part %s: %v", row.SitePartID.String, err)
			return "", pd, revID
		}
		return html, pd, revID
	}
	hdr, hdrPD, _ := renderLoc("header")
	if hdr != "" {
		headerHTML = hdr
		if hdrPD != nil {
			used = append(used, hdrPD.UsedBlocks...)
		}
	}
	ftr, ftrPD, _ := renderLoc("footer")
	if ftr != "" {
		footerHTML = ftr
		if ftrPD != nil {
			used = append(used, ftrPD.UsedBlocks...)
		}
	}
	return headerHTML, footerHTML, used
}

func (h *Handler) renderArchivePage(ctx context.Context, origin, archivePath string, pageNum int, siteSnap *site.Snapshot) (pagecache.Entry, error) {
	if pageNum < 1 {
		return pagecache.Entry{}, sql.ErrNoRows
	}
	// Generic archive: content type comes from the archive route's content_type_id.
	// Fallback to post for legacy routes / shell-less home archive.
	archiveContentType := "post"
	if rt, ok := h.hub.Routes.Lookup(archivePath); ok && rt.ContentTypeID.Valid && rt.ContentTypeID.String != "" {
		archiveContentType = rt.ContentTypeID.String
	} else if ct := routing.ContentTypeForArchive(archivePath, siteSnap.PostsBasePath, siteSnap.HomepageMode); ct != "" {
		archiveContentType = ct
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

	// Detect taxonomy term archive via route snapshot
	var termArchive bool
	var termID, taxonomyID, termName, termDesc string
	if rt, ok := h.hub.Routes.Lookup(archivePath); ok && rt.TaxonomyID.Valid && rt.TermID.Valid {
		termArchive = true
		termID = rt.TermID.String
		taxonomyID = rt.TaxonomyID.String
		if t, err := h.queries.GetTerm(ctx, termID); err == nil {
			termName = t.Name
			termDesc = t.Description
		}
	}

	var total int64
	var rows []db.ListPublishedEntriesByContentTypeRow
	var termRows []db.ListPublishedEntriesByTermRow
	var err error
	if termArchive {
		total, err = h.queries.ListPublishedEntriesByTermCount(ctx, db.ListPublishedEntriesByTermCountParams{TermID: termID, ContentTypeID: archiveContentType})
		if err != nil {
			return pagecache.Entry{}, err
		}
	} else {
		total, err = h.queries.CountPublishedEntriesByContentType(ctx, archiveContentType)
		if err != nil {
			return pagecache.Entry{}, err
		}
	}
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages == 0 {
		totalPages = 1
	}
	if pageNum > totalPages {
		return pagecache.Entry{}, sql.ErrNoRows
	}

	if termArchive {
		trs, err := h.queries.ListPublishedEntriesByTerm(ctx, db.ListPublishedEntriesByTermParams{TermID: termID, ContentTypeID: archiveContentType, Limit: int64(perPage), Offset: int64(offset)})
		if err != nil {
			return pagecache.Entry{}, err
		}
		termRows = trs
	} else {
		tmpRows, err := h.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{
			ContentTypeID: archiveContentType,
			Limit:         int64(perPage),
			Offset:        int64(offset),
		})
		if err != nil {
			return pagecache.Entry{}, err
		}
		rows = tmpRows
	}

	// Build archive entries using route_path from DB (source of truth)
	var archiveEntries []rendering.ArchiveEntry
	if termArchive {
		featIDs := make([]string, 0, len(termRows))
		for _, r := range termRows {
			if r.FeaturedMediaID.Valid && r.FeaturedMediaID.String != "" {
				featIDs = append(featIDs, r.FeaturedMediaID.String)
			}
		}
		mediaCache := map[string]rendering.MediaView{}
		if h.media != nil && len(featIDs) > 0 {
			mediaCache = h.media.MediaViews(ctx, featIDs)
			if mediaCache == nil {
				mediaCache = map[string]rendering.MediaView{}
			}
		}
		archiveEntries = make([]rendering.ArchiveEntry, 0, len(termRows))
		for _, r := range termRows {
			ae := rendering.ArchiveEntry{
				ID:            r.ID,
				Slug:          r.Slug,
				ContentTypeID: archiveContentType,
				Title:         r.Title,
				Excerpt:       stringValue(r.Excerpt),
				URL:           r.RoutePath,
				PublishedAt:   formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, false),
				PublishedISO:  formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, true),
			}
			if r.FeaturedMediaID.Valid {
				if mv, ok := mediaCache[r.FeaturedMediaID.String]; ok {
					ae.FeaturedImage = mv
				}
			}
			ae.Fields, _ = content.DecodeFieldSnapshot(r.FieldsJson)
			archiveEntries = append(archiveEntries, ae)
		}
	} else {
		archiveEntries = h.buildArchiveEntries(ctx, rows, siteSnap, archiveContentType)
	}
	// Also build legacy theme ArchiveView for shell-less fallback rendering
	themeEntries := make([]themes.ArchiveEntryView, 0, len(archiveEntries))
	for _, ae := range archiveEntries {
		themeEntries = append(themeEntries, themes.ArchiveEntryView{
			ID:            ae.ID,
			Title:         ae.Title,
			Excerpt:       ae.Excerpt,
			URL:           ae.URL,
			PublishedAt:   ae.PublishedAt,
			PublishedISO:  ae.PublishedISO,
			FeaturedImage: ae.FeaturedImage,
		})
	}

	// EPIC 2: Check for archive template (takes precedence over shell for applicable content types)
	var archiveTemplateDoc *document.Document
	var archiveTemplateID, archiveTemplateRevID string
	tmpl, templateErr := layouts.ResolveArchive(ctx, h.queries, archiveContentType)
	if templateErr != nil {
		return pagecache.Entry{}, fmt.Errorf("resolve configured archive template: %w", templateErr)
	}
	if tmpl != nil {
		archiveTemplateDoc = tmpl.Document
		archiveTemplateID = tmpl.TemplateID
		archiveTemplateRevID = tmpl.RevisionID
	}

	// Load shell page (Posts Page) directly by entry ID via archive route (snapshot, zero DB for route).
	var shellRow *db.GetPublishedEntryByIDRow
	var prepared *rendering.PreparedDocument
	var shellTitle, shellDesc, shellSeoTitle, shellSeoDesc, shellFeatured, shellSocial string
	var shellRobotsIndex, shellRobotsFollow *bool
	var shellCanonical string
	shellFound := false
	// For post archives, shell is fallback only if no archive template. For other types, shell is not used for product-style archives (no EntryID).
	if archiveTemplateDoc == nil {
		if rt, ok := h.hub.Routes.Lookup(archivePath); ok && rt.RouteType == "archive" && rt.EntryID.Valid {
			if s, serr := h.queries.GetPublishedEntryByID(ctx, rt.EntryID.String); serr == nil {
				tmp := s
				shellRow = &tmp
				shellFound = true
			}
		}
		// Fallback: if archive path is "/" for latest posts home with no archive route, shell remains nil
		if shellFound {
			shellTitle = shellRow.Title
			shellDesc = stringValue(shellRow.Excerpt)
			shellSeoTitle = stringValue(shellRow.SeoTitle)
			shellSeoDesc = stringValue(shellRow.SeoDescription)
			shellFeatured = stringValue(shellRow.FeaturedMediaID)
			shellSocial = stringValue(shellRow.SocialMediaID)
			shellRobotsIndex = seo.NullIntToBoolPtr(shellRow.SeoRobotsIndex.Valid, shellRow.SeoRobotsIndex.Int64)
			shellRobotsFollow = seo.NullIntToBoolPtr(shellRow.SeoRobotsFollow.Valid, shellRow.SeoRobotsFollow.Int64)
			shellCanonical = stringValue(shellRow.CanonicalUrl)
			if d, derr := document.Decode([]byte(shellRow.DocumentJson)); derr == nil {
				effectiveDoc, layoutRevID, rerr := h.layoutsService.ResolveEffectiveDocument(ctx, d, shellRow.ContentTypeID, shellRow.LayoutTemplateID)
				if rerr != nil {
					return pagecache.Entry{}, fmt.Errorf("resolve archive layout: %w", rerr)
				}
				cacheKey := shellRow.RevisionID
				if layoutRevID != "" {
					cacheKey = shellRow.RevisionID + ":" + layoutRevID
				}
				if p, perr := h.blocks.PreparedCache(cacheKey, effectiveDoc); perr == nil {
					prepared = p
				} else if p2, perr2 := h.blocks.Prepare(effectiveDoc); perr2 == nil {
					prepared = p2
				}
			}
		}
		if !shellFound {
			// No shell: default title/desc
			if archivePath == "/" {
				shellTitle = siteSnap.SiteTitle
			} else {
				shellTitle = "Blog"
				if definition, err := content.NewCatalog(h.queries).GetDefinition(ctx, archiveContentType); err == nil {
					// This is only a legacy/theme fallback. Admin Content Type labels are not canonical public content
					// and must not be used by future multilingual/template systems.
					shellTitle = content.FallbackArchiveTitle(definition)
				}
			}
		}
	} else {
		// Archive template active: set SEO title from term or content type for fallback
		if termArchive {
			shellTitle = termName
			shellDesc = termDesc
		} else if definition, err := content.NewCatalog(h.queries).GetDefinition(ctx, archiveContentType); err == nil && definition.PluralName != "" {
			shellTitle = definition.PluralName
		} else {
			shellTitle = archiveContentType
		}
		// Prepare archive template for later rendering
		prepared, err = h.blocks.PreparedCache(archiveTemplateRevID, archiveTemplateDoc)
		if err != nil {
			return pagecache.Entry{}, fmt.Errorf("prepare configured archive template: %w", err)
		}
		shellFound = false
	}

	// Build archive context for Collection(source=context) – single source for both shell and fallback
	pagination := rendering.PaginationContext{
		Current:    pageNum,
		TotalPages: totalPages,
		TotalItems: total,
	}
	if pageNum > 1 {
		pagination.PreviousURL = seo.PaginatedPath(archivePath, pageNum-1)
	}
	if pageNum < totalPages {
		pagination.NextURL = seo.PaginatedPath(archivePath, pageNum+1)
	}
	archiveTitle, archiveDescription := resolveArchivePresentation(termArchive, termName, termDesc, shellTitle, shellDesc)
	archCtx := &rendering.ArchiveContext{
		Entries:     archiveEntries,
		Pagination:  pagination,
		Permalink:   seo.PaginatedPath(archivePath, pageNum),
		Title:       archiveTitle,
		Description: archiveDescription,
	}
	if termArchive {
		archCtx.TaxonomyID = taxonomyID
		archCtx.TermID = termID
	}
	var shellContent template.HTML
	var usedBlocks []rendering.BlockKey
	var archiveRC rendering.RenderContext
	menusForArchive := h.hub.Navigation.LocationsForPath(archivePath)
	if prepared != nil {
		rc := h.archiveRenderContext(siteSnap, shellRow, archivePath, archCtx, origin)
		rc.Route = rendering.RouteContext{Path: archivePath, IsArchive: true, ContentType: archiveContentType, Archive: archCtx, Pagination: pagination, ArchiveTitle: archiveTitle, ArchiveDescription: archiveDescription}
		if termArchive {
			rc.Route.TaxonomyID = taxonomyID
			rc.Route.TermID = termID
			rc.Route.ArchiveTitle = termName
			rc.Route.ArchiveDescription = termDesc
		}
		rc.Mode = rendering.ModePublic
		rc.ContentReader = &handlerContentReader{queries: h.queries, siteSnap: siteSnap, media: h.media}
		rc.QueryCache = make(map[string][]rendering.ArchiveEntry)
		if shellRow != nil {
			rc.EntryID = shellRow.ID
		}
		rc.LCP = &rendering.LCPState{}
		if def, err := content.NewCatalog(h.queries).GetDefinition(ctx, archiveContentType); err == nil {
			rc.Definition = def
		} else {
			rc.Definition = content.DefinitionFor(archiveContentType)
		}
		rc.SitePartReader = newHandlerSitePartReader(h.queries, h.blocks)
		rc.CollectedSiteParts = make(map[string]string)
		rc.Dependencies = &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)}
		rc.SitePartStack = make(map[string]struct{})
		rc.Navigation = menusForArchive
		c, cerr := h.renderBlocks(ctx, prepared, rc)
		if cerr != nil {
			return pagecache.Entry{}, fmt.Errorf("render archive shell: %w", cerr)
		}
		shellContent = c
		usedBlocks = prepared.UsedBlocks
		archiveRC = rc
	} else {
		// Shell-less fallback: render a minimal Collection so theme can stay .Content-only.
		routeCtx := rendering.RouteContext{Path: archivePath, IsArchive: true, ContentType: archiveContentType, Archive: archCtx, Pagination: pagination, ArchiveTitle: archiveTitle, ArchiveDescription: archiveDescription}
		if termArchive {
			routeCtx.TaxonomyID = taxonomyID
			routeCtx.TermID = termID
			routeCtx.ArchiveTitle = termName
			routeCtx.ArchiveDescription = termDesc
		}
		rc := rendering.RenderContext{
			Site:          rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
			Route:         routeCtx,
			Mode:          rendering.ModePublic,
			ContentReader: &handlerContentReader{queries: h.queries, siteSnap: siteSnap, media: h.media},
			QueryCache:    make(map[string][]rendering.ArchiveEntry),
			Archive:       archCtx,
		}
		if siteSnap.LogoMediaID != "" && h.media != nil {
			if view, ok := h.media.MediaView(context.Background(), siteSnap.LogoMediaID); ok {
				rc.Site.LogoURL = view.Src
				rc.Site.LogoWidth = view.Width
				rc.Site.LogoHeight = view.Height
			}
		}
		fallbackDoc := &document.Document{Version: 1, Nodes: []document.Node{{
			ID: "fallback", Block: "core/collection", Version: 1, Settings: json.RawMessage(`{"source":"context"}`),
			Children: []document.Node{{ID: "fallback-title", Block: "core/entry-title", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`)}},
		}}}
		if p, err := h.blocks.Prepare(fallbackDoc); err == nil {
			if c, err := h.renderBlocks(ctx, p, rc); err == nil {
				shellContent = c
				usedBlocks = p.UsedBlocks
			}
		}
	}

	// Header/Footer for archive (EPIC 2)
	var headerHTML, footerHTML template.HTML
	var hfUsed []rendering.BlockKey
	var baseRCForHF rendering.RenderContext
	if prepared != nil {
		baseRCForHF = archiveRC
		if baseRCForHF.CollectedSiteParts == nil {
			baseRCForHF.CollectedSiteParts = make(map[string]string)
		}
		if baseRCForHF.SitePartReader == nil {
			baseRCForHF.SitePartReader = newHandlerSitePartReader(h.queries, h.blocks)
		}
		if baseRCForHF.Navigation == nil {
			baseRCForHF.Navigation = menusForArchive
		}
	} else {
		siteCtxHF := rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL}
		if siteSnap.LogoMediaID != "" && h.media != nil {
			if view, ok := h.media.MediaView(context.Background(), siteSnap.LogoMediaID); ok {
				siteCtxHF.LogoURL = view.Src
				siteCtxHF.LogoWidth = view.Width
				siteCtxHF.LogoHeight = view.Height
			}
		}
		routeCtxHF := rendering.RouteContext{Path: archivePath, IsArchive: true, ContentType: archiveContentType, Archive: archCtx, Pagination: pagination, ArchiveTitle: archiveTitle, ArchiveDescription: archiveDescription}
		if termArchive {
			routeCtxHF.TaxonomyID = taxonomyID
			routeCtxHF.TermID = termID
			routeCtxHF.ArchiveTitle = termName
			routeCtxHF.ArchiveDescription = termDesc
		}
		baseRCForHF = rendering.RenderContext{
			Site:               siteCtxHF,
			Route:              routeCtxHF,
			Mode:               rendering.ModePublic,
			ContentReader:      &handlerContentReader{queries: h.queries, siteSnap: siteSnap, media: h.media},
			QueryCache:         make(map[string][]rendering.ArchiveEntry),
			SitePartReader:     newHandlerSitePartReader(h.queries, h.blocks),
			CollectedSiteParts: make(map[string]string),
			Dependencies:       &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)},
			SitePartStack:      make(map[string]struct{}),
			Navigation:         menusForArchive,
		}
	}
	headerHTML, footerHTML, hfUsed = h.renderHeaderFooter(ctx, siteSnap, archivePath, baseRCForHF)
	usedBlocks = append(usedBlocks, hfUsed...)
	for _, dependency := range baseRCForHF.Dependencies.SiteParts {
		usedBlocks = append(usedBlocks, dependency.UsedBlocks...)
	}
	if prepared != nil {
		for sid := range archiveRC.CollectedSiteParts {
			if _, exists := baseRCForHF.CollectedSiteParts[sid]; !exists {
				baseRCForHF.CollectedSiteParts[sid] = archiveRC.CollectedSiteParts[sid]
			}
		}
	}
	dedupMap := make(map[rendering.BlockKey]struct{}, len(usedBlocks))
	dedupedBlocks := make([]rendering.BlockKey, 0, len(usedBlocks))
	for _, k := range usedBlocks {
		if _, ok := dedupMap[k]; !ok {
			dedupMap[k] = struct{}{}
			dedupedBlocks = append(dedupedBlocks, k)
		}
	}
	usedBlocks = dedupedBlocks

	// Pagination URLs for theme fallback
	prev, next := "", ""
	if pageNum > 1 {
		prev = seo.PaginatedPath(archivePath, pageNum-1)
	}
	if pageNum < totalPages {
		next = seo.PaginatedPath(archivePath, pageNum+1)
	}
	archView := themes.ArchiveView{
		ContentTypeID: archiveContentType,
		Title:         shellTitle,
		Description:   shellDesc,
		Intro:         "", // intro is now inside content via SDT; leave empty to avoid double rendering
		Entries:       themeEntries,
		Pagination: themes.PaginationView{
			Current:     pageNum,
			TotalPages:  totalPages,
			TotalItems:  total,
			PreviousURL: prev,
			NextURL:     next,
		},
		HasShell: shellFound,
	}
	if termArchive {
		archView.TaxonomyID = taxonomyID
		archView.TermID = termID
		archView.Title = termName
		archView.Description = termDesc
	}

	// SEO via central resolver (site → shell revision → archive context)
	resolved := h.resolveArchiveSEOWithShell(ctx, siteSnap, archivePath, pageNum, shellRow, shellTitle, shellDesc, shellSeoTitle, shellSeoDesc, shellFeatured, shellSocial, shellRobotsIndex, shellRobotsFollow, shellCanonical, origin)

	menus := h.hub.Navigation.LocationsForPath(archivePath)
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, resolved, siteIcon)
	if prepared != nil {
		head.Preloads = h.lcpPreloads(ctx, prepared, archiveRC)
	}

	_, themeCSS, themeJS := h.hub.Assets.URLs()
	blocksCSS := ""
	if len(usedBlocks) > 0 {
		blocksCSS = h.hub.Assets.BlocksCSSFor(usedBlocks)
	}
	regionsArchive := make(map[string]template.HTML)
	if len(headerHTML) > 0 {
		regionsArchive["header"] = headerHTML
	}
	if len(footerHTML) > 0 {
		regionsArchive["footer"] = footerHTML
	}
	view := themes.PageView{
		Site:        themes.SiteView{Title: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: siteIconURL(siteSnap, h.media), LogoWidth: 0, LogoHeight: 0},
		Head:        head,
		Navigation:  menus,
		Content:     shellContent,
		ContentType: archiveContentType,
		Kind:        themes.PageKindArchive,
		IsFrontPage: archivePath == "/",
		Archive:     archView,
		Header:      headerHTML,
		Footer:      footerHTML,
		Regions:     regionsArchive,
		Assets:      themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}
	// Populate Site logo for header (from site snapshot)
	if siteSnap.LogoMediaID != "" && h.media != nil {
		if mv, ok := h.media.MediaView(ctx, siteSnap.LogoMediaID); ok {
			view.Site.LogoURL = mv.Src
			view.Site.LogoWidth = mv.Width
			view.Site.LogoHeight = mv.Height
		}
	}

	html, err := h.themes.Render(view, nil)
	if err != nil {
		return pagecache.Entry{}, fmt.Errorf("theme render archive: %w", err)
	}
	gz, err := compress.Gzip(html)
	if err != nil {
		gz = nil
	}
	br, err := compress.Brotli(html)
	if err != nil {
		br = nil
	}
	tags := []string{"content-type:" + archiveContentType, "site", "navigation", "theme"}
	if termArchive {
		tags = append(tags, "taxonomy:"+taxonomyID, "term:"+termID)
	}
	if shellRow != nil && shellRow.ID != "" {
		tags = append(tags, "entry:"+shellRow.ID)
		if shellRow.LayoutTemplateID.Valid && shellRow.LayoutTemplateID.String != "" {
			tags = append(tags, "layout:"+shellRow.LayoutTemplateID.String)
		}
	}
	if archiveTemplateID != "" {
		tags = append(tags, "template:"+archiveTemplateID)
	}
	if prepared != nil {
		for _, ct := range collectionContentTypes(prepared) {
			// Archive shell may also have query collections needing same tag; dedup handled by cache
			tags = append(tags, "content-type:"+ct)
		}
	}
	for sid := range baseRCForHF.CollectedSiteParts {
		tags = append(tags, "site-part:"+sid)
	}
	// Deduplicate tags
	seen := make(map[string]struct{}, len(tags))
	uniq := make([]string, 0, len(tags))
	for _, t := range tags {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			uniq = append(uniq, t)
		}
	}
	return pagecache.Entry{
		HTML:        html,
		Gzip:        gz,
		Brotli:      br,
		ETag:        pagecache.ComputeETag(html),
		Robots:      resolved.Robots,
		ContentType: "text/html; charset=utf-8",
		Tags:        uniq,
	}, nil
}

func collectionContentTypes(prepared *rendering.PreparedDocument) []string {
	if prepared == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	var walk func([]rendering.PreparedNode)
	walk = func(nodes []rendering.PreparedNode) {
		for _, n := range nodes {
			if n.Block == "core/collection" {
				ct, _ := n.Settings["contentType"].(string)
				if ct == "" {
					ct, _ = n.Settings["content_type"].(string)
				}
				if ct == "" {
					ct = "post"
				}
				src, _ := n.Settings["source"].(string)
				if src == "" {
					src = "query"
				}
				// Only query/automatic collections depend on content-type listing
				if src == "query" || src == "automatic" {
					if _, ok := seen[ct]; !ok {
						seen[ct] = struct{}{}
						out = append(out, ct)
					}
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(prepared.Nodes)
	return out
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
type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr"`
}
type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	Description string  `xml:"description"`
	PubDate     string  `xml:"pubDate"`
	GUID        rssGUID `xml:"guid"`
}

func (h *Handler) serveFeed(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil || strings.TrimSpace(siteSnap.SiteURL) == "" {
		http.NotFound(w, r)
		return
	}
	body, gz, br, etag, ok := h.hub.Feed.GetBrotli()
	if !ok {
		built, err := h.buildFeed(r.Context(), siteSnap)
		if err != nil {
			log.Printf("feed build: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		gz, _ = compress.Gzip(built)
		br, _ = compress.Brotli(built)
		etag = pagecache.ComputeETag(built)
		h.hub.Feed.SetWithBrotli(built, gz, br, etag)
		body = built
	}
	h.writeText(w, r, body, gz, br, etag, "application/rss+xml; charset=utf-8", "public, max-age=300")
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
		link := base + r.RoutePath
		pub := ""
		if r.FirstPublishedAt.Valid {
			pub = time.Unix(r.FirstPublishedAt.Int64, 0).UTC().Format(time.RFC1123)
		}
		desc := stringValue(r.Excerpt)
		items = append(items, rssItem{
			Title:       r.Title,
			Link:        link,
			Description: desc,
			PubDate:     pub,
			GUID:        rssGUID{Value: r.ID, IsPermaLink: "false"},
		})
	}
	archivePath := seo.PostsArchivePath(siteSnap.PostsBasePath)
	if siteSnap.HomepageMode == "latest_posts" {
		archivePath = "/"
	}
	channelLink := base + archivePath
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:       siteSnap.SiteTitle,
			Link:        channelLink,
			Description: siteSnap.SiteTagline,
			Items:       items,
		},
	}
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func (h *Handler) resolveArchiveSEO(ctx context.Context, siteSnap *site.Snapshot, path string, page int, title, desc, origin string) seo.Resolved {
	// Legacy helper kept for tests; delegates to central resolver wrapper with no shell.
	return h.resolveArchiveSEOWithShell(ctx, siteSnap, path, page, nil, title, desc, "", "", "", "", nil, nil, "", origin)
}

func (h *Handler) resolveArchiveSEOWithShell(ctx context.Context, siteSnap *site.Snapshot, path string, page int, shell *db.GetPublishedEntryByIDRow, title, desc, seoTitle, seoDesc, featuredID, socialID string, robotsIndex, robotsFollow *bool, canonicalOverride, origin string) seo.Resolved {
	// Build central resolver input from shell revision (when present) or fallback title/desc
	rawTitle := strings.TrimSpace(seoTitle)
	if rawTitle == "" {
		rawTitle = strings.TrimSpace(title)
	}
	// Page suffix (avoid double site suffix)
	if page > 1 && rawTitle != "" {
		rawTitle = fmt.Sprintf("%s – Page %d", rawTitle, page)
		// Ensure the resolver sees the paginated raw title via SeoTitle
		seoTitle = rawTitle
		title = rawTitle
	}
	pathForCanonical := seo.PaginatedPath(path, page)
	canonicalForResolver := strings.TrimSpace(canonicalOverride)
	if page > 1 {
		// Pagination must be self-canonical: shell override must not bleed to page 2+
		canonicalForResolver = ""
	}
	resolver := seo.New()
	input := seo.Input{
		Site: seo.SiteSEO{
			Title:               siteSnap.SiteTitle,
			TitleSeparator:      siteSnap.TitleSeparator,
			SiteURL:             siteSnap.SiteURL,
			Language:            siteSnap.Language,
			IndexingEnabled:     siteSnap.IndexingEnabled,
			GlobalSocialMediaID: siteSnap.GlobalSocialMediaID,
			TwitterSite:         siteSnap.TwitterSite,
		},
		ContentTypeID: "",
		Revision: seo.RevisionSEO{
			Title:           title,
			Excerpt:         desc,
			SeoTitle:        seoTitle,
			SeoDescription:  seoDesc,
			CanonicalURL:    canonicalForResolver,
			FeaturedMediaID: featuredID,
			SocialMediaID:   socialID,
			RobotsIndex:     robotsIndex,
			RobotsFollow:    robotsFollow,
		},
		Path:   pathForCanonical,
		Origin: origin,
	}
	// If shell provided, fill missing title/desc from shell better? Already done via params
	_ = shell
	resolved := resolver.Resolve(input)
	resolved = h.enrichSocialImage(ctx, siteSnap, origin, resolved)
	feed := seo.Canonical(siteSnap.SiteURL, origin, "/feed.xml", "")
	resolved.Alternates = append(resolved.Alternates, seo.AlternateView{Href: feed, Type: "application/rss+xml"})
	return resolved
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
			RulesJSON: template.HTML(siteSnap.SpeculationRulesJSON),
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
		Site:      rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
		Entry:     rendering.EntryContext{ID: input.EntryID, Slug: input.Slug, ContentTypeID: input.ContentTypeID, Title: input.Title, Excerpt: input.Excerpt, Permalink: path, FeaturedImage: input.FeaturedMediaID, Fields: input.Fields},
		Archive:   input.Archive,
		IsPreview: true,
		EntryID:   input.EntryID,
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
	// If this preview is for the current Posts Page, provide a real ArchiveContext
	// (page 1 of published posts) so the drafted layout renders with live data.
	// This is preview only; it does not publish the draft.
	if input.Archive != nil {
		rc.ArchiveURL = input.Archive.Permalink
	} else if input.EntryID != "" && siteSnap.PostsPageEntryID != "" && input.EntryID == siteSnap.PostsPageEntryID {
		// Build archive entries for preview (page 1)
		perPage := int(siteSnap.PostsPerPage)
		if perPage <= 0 {
			perPage = 10
		}
		rows, _ := h.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: "post", Limit: int64(perPage), Offset: 0})
		archiveEntries := h.buildArchiveEntries(ctx, rows, siteSnap, "post")
		total, _ := h.queries.CountPublishedEntriesByContentType(ctx, "post")
		totalPages := int((total + int64(perPage) - 1) / int64(perPage))
		if totalPages == 0 {
			totalPages = 1
		}
		pagination := rendering.PaginationContext{Current: 1, TotalPages: totalPages, TotalItems: total}
		if totalPages > 1 {
			// For preview, use the derived archive path as permalink
			permalink := path
			if input.EntryID == siteSnap.PostsPageEntryID && siteSnap.PostsPageEntryID != "" {
				if entry, err := h.queries.GetEntry(ctx, siteSnap.PostsPageEntryID); err == nil {
					permalink = "/" + entry.Slug
				}
			}
			pagination.NextURL = seo.PaginatedPath(permalink, 2)
		}
		rc.Archive = &rendering.ArchiveContext{Entries: archiveEntries, Pagination: pagination, Permalink: path}
		// Also set ArchiveURL for view-all links
		archiveURL := seo.PostsArchivePath(siteSnap.PostsBasePath)
		if siteSnap.HomepageMode == "latest_posts" {
			archiveURL = "/"
		}
		rc.ArchiveURL = archiveURL
	} else {
		archiveURL := seo.PostsArchivePath(siteSnap.PostsBasePath)
		if siteSnap.HomepageMode == "latest_posts" {
			archiveURL = "/"
		}
		rc.ArchiveURL = archiveURL
	}
	// If preview specifies a layout template, compose before prepare so LCP/collections see final tree.
	// Composition is validated inside the layout service boundary.
	effectiveDoc := input.Document
	if input.LayoutTemplateID != "" {
		ct := input.ContentTypeID
		if ct == "" {
			if input.EntryID != "" {
				if e, err := h.queries.GetEntry(ctx, input.EntryID); err == nil {
					ct = e.ContentTypeID
				}
			}
		}
		if ct != "" {
			var composed *document.Document
			var cerr error
			if h.layoutsService != nil {
				composed, _, cerr = h.layoutsService.ResolveEffectiveDocument(ctx, input.Document, ct, sql.NullString{String: input.LayoutTemplateID, Valid: true})
			} else {
				composed, cerr = layouts.ResolveEffectiveDocument(ctx, h.queries, input.Document, ct, sql.NullString{String: input.LayoutTemplateID, Valid: true})
			}
			if cerr == nil {
				effectiveDoc = composed
			} else {
				return nil, cerr
			}
		}
	}
	// Provide generic collection context for preview (no DB query for latest in preview, but context for archive preview is set above)
	rc.Mode = rendering.ModePreview
	rc.ContentReader = &handlerContentReader{queries: h.queries, siteSnap: siteSnap, media: h.media}
	rc.QueryCache = make(map[string][]rendering.ArchiveEntry)
	if rc.Archive != nil {
		rc.Route = rendering.RouteContext{Path: path, IsArchive: true, ContentType: input.ContentTypeID, Archive: rc.Archive, Pagination: rc.Archive.Pagination, ArchiveTitle: rc.Archive.Title, ArchiveDescription: rc.Archive.Description}
	}
	if rc.LCP == nil {
		rc.LCP = &rendering.LCPState{}
	}
	if input.ContentTypeID != "" {
		if def, err := content.NewCatalog(h.queries).GetDefinition(ctx, input.ContentTypeID); err == nil {
			rc.Definition = def
		} else {
			rc.Definition = content.DefinitionFor(input.ContentTypeID)
		}
	}
	prepared, err := h.blocks.Prepare(effectiveDoc)
	if err != nil {
		return nil, err
	}
	// Use prepared rendering to keep collections and archive
	previewResolved := h.resolvePreviewSEO(ctx, siteSnap, input, path, "")
	rc.Navigation = h.hub.Navigation.LocationsForPath(path)
	rc.SitePartReader = newHandlerSitePartReader(h.queries, h.blocks)
	rc.CollectedSiteParts = make(map[string]string)
	rc.Dependencies = &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)}
	content, err := h.renderBlocks(ctx, prepared, rc)
	if err != nil {
		return nil, err
	}
	menus := h.hub.Navigation.LocationsForPath(path)
	headerHTML, footerHTML, regionUsed := h.renderHeaderFooter(ctx, siteSnap, path, rc)
	renderRegionOverride := func(doc *document.Document) (template.HTML, []rendering.BlockKey, error) {
		if doc == nil {
			return "", nil, nil
		}
		pd, err := h.blocks.Prepare(doc)
		if err != nil {
			return "", nil, err
		}
		regionRC := rc
		regionRC.Entry = rendering.EntryContext{}
		regionRC.EntryID = ""
		regionRC.LCP = &rendering.LCPState{}
		regionRC.Mode = rendering.ModePreview
		html, err := h.renderBlocks(ctx, pd, regionRC)
		return html, pd.UsedBlocks, err
	}
	if input.HeaderDocument != nil {
		var overrideUsed []rendering.BlockKey
		headerHTML, overrideUsed, err = renderRegionOverride(input.HeaderDocument)
		regionUsed = append(regionUsed, overrideUsed...)
		if err != nil {
			return nil, err
		}
	}
	if input.FooterDocument != nil {
		var overrideUsed []rendering.BlockKey
		footerHTML, overrideUsed, err = renderRegionOverride(input.FooterDocument)
		regionUsed = append(regionUsed, overrideUsed...)
		if err != nil {
			return nil, err
		}
	}
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, previewResolved, siteIcon)
	head.Preloads = h.lcpPreloads(ctx, prepared, rc)
	_, themeCSS, themeJS := h.hub.Assets.URLs()
	usedBlocks := append([]rendering.BlockKey(nil), prepared.UsedBlocks...)
	usedBlocks = append(usedBlocks, regionUsed...)
	for _, dependency := range rc.Dependencies.SiteParts {
		usedBlocks = append(usedBlocks, dependency.UsedBlocks...)
	}
	blocksCSS := h.hub.Assets.BlocksCSSFor(usedBlocks)
	regions := make(map[string]template.HTML)
	if headerHTML != "" {
		regions["header"] = headerHTML
	}
	if footerHTML != "" {
		regions["footer"] = footerHTML
	}
	view := themes.PageView{
		Site:       themes.SiteView{Title: rc.Site.Name, Tagline: rc.Site.Tagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: rc.Site.LogoURL, LogoWidth: rc.Site.LogoWidth, LogoHeight: rc.Site.LogoHeight},
		Entry:      themes.EntryView{Title: rc.Entry.Title, SEOTitle: previewResolved.OpenGraph.Title, SEODescription: previewResolved.Description, CanonicalURL: previewResolved.Canonical},
		Head:       head,
		Navigation: menus,
		Content:    content,
		Header:     headerHTML,
		Footer:     footerHTML,
		Regions:    regions,
		Assets:     themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}
	page, err := h.themes.Render(view, input.Temporary)
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
	fields, err := content.DecodeFieldSnapshot(entry.FieldsJson)
	if err != nil {
		return nil, "", fmt.Errorf("decode revision fields: %w", err)
	}
	// Layout composition is validated inside the layout service boundary.
	if h.layoutsService != nil {
		if effective, _, cerr := h.layoutsService.ResolveEffectiveDocument(ctx, doc, entry.ContentTypeID, entry.LayoutTemplateID); cerr == nil {
			doc = effective
		} else {
			return nil, "", fmt.Errorf("resolve layout template: %w", cerr)
		}
	} else if effective, _, cerr := layouts.ResolveEffectiveDocumentWithID(ctx, h.queries, doc, entry.ContentTypeID, entry.LayoutTemplateID); cerr == nil {
		doc = effective
	} else {
		return nil, "", fmt.Errorf("resolve layout template: %w", cerr)
	}
	siteSnap := h.hub.Site.Current()
	resolved := h.resolvePublishedSEO(ctx, siteSnap, &entry, path, origin)
	rc := h.entryRenderContext(siteSnap, &entry, path, resolved, fields)
	// RenderPath has no request-bound password unlock proof. Public callers use
	// renderEntry above; previews intentionally receive no comments by default.
	h.populateCommentsContext(ctx, &rc, entry.ID, false)
	page, robots, err := h.renderThemedDocument(ctx, siteSnap, doc, rc, resolved, path, temporary, customCSS)
	return page, robots, err
}

func (h *Handler) entryRenderContext(siteSnap *site.Snapshot, entry *db.GetPublishedEntryByPathRow, path string, resolved seo.Resolved, fields map[string]any) rendering.RenderContext {
	rc := rendering.RenderContext{
		Site: rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
		Entry: rendering.EntryContext{
			ID:            entry.ID,
			Slug:          entry.Slug,
			ContentTypeID: entry.ContentTypeID,
			Title:         entry.Title,
			Excerpt:       stringValue(entry.Excerpt),
			Permalink:     path,
			PublishDate:   formatEntryDate(entry.PublishedAt, siteSnap.TimezoneName, false),
			PublishISO:    formatEntryDate(entry.PublishedAt, siteSnap.TimezoneName, true),
			FeaturedImage: resolved.FeaturedMediaID,
			Fields:        fields,
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

func (h *Handler) archiveRenderContext(siteSnap *site.Snapshot, shell *db.GetPublishedEntryByIDRow, archivePath string, archCtx *rendering.ArchiveContext, origin string) rendering.RenderContext {
	rc := rendering.RenderContext{
		Site:    rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
		Archive: archCtx,
	}
	if shell != nil {
		fields, err := content.DecodeFieldSnapshot(shell.FieldsJson)
		if err != nil {
			fields = map[string]any{}
		}
		rc.Entry = rendering.EntryContext{
			Title:         shell.Title,
			Excerpt:       stringValue(shell.Excerpt),
			Permalink:     seo.Canonical(siteSnap.SiteURL, origin, archivePath, ""),
			PublishDate:   formatEntryDate(shell.PublishedAt, siteSnap.TimezoneName, false),
			PublishISO:    formatEntryDate(shell.PublishedAt, siteSnap.TimezoneName, true),
			FeaturedImage: stringValue(shell.FeaturedMediaID),
			Fields:        fields,
		}
	} else {
		rc.Entry = rendering.EntryContext{
			Permalink: seo.Canonical(siteSnap.SiteURL, origin, archivePath, ""),
		}
		if archCtx != nil {
			rc.Entry.Permalink = archCtx.Permalink
		}
	}
	if siteSnap.LogoMediaID != "" && h.media != nil {
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

// resolveArchivePresentation centralizes the semantic title/description passed
// to archive blocks. Taxonomy terms are canonical. For ordinary archives the
// already-resolved legacy presentation is a compatibility fallback only.
func resolveArchivePresentation(termArchive bool, termName, termDescription, fallbackTitle, fallbackDescription string) (string, string) {
	if termArchive {
		return strings.TrimSpace(termName), strings.TrimSpace(termDescription)
	}
	return strings.TrimSpace(fallbackTitle), strings.TrimSpace(fallbackDescription)
}

func (h *Handler) buildArchiveEntries(ctx context.Context, rows []db.ListPublishedEntriesByContentTypeRow, siteSnap *site.Snapshot, contentTypeID string) []rendering.ArchiveEntry {
	if len(rows) == 0 {
		return nil
	}
	featIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.FeaturedMediaID.Valid && r.FeaturedMediaID.String != "" {
			featIDs = append(featIDs, r.FeaturedMediaID.String)
		}
	}
	mediaCache := map[string]rendering.MediaView{}
	if h.media != nil && len(featIDs) > 0 {
		mediaCache = h.media.MediaViews(ctx, featIDs)
		if mediaCache == nil {
			mediaCache = map[string]rendering.MediaView{}
		}
	}
	out := make([]rendering.ArchiveEntry, 0, len(rows))
	for _, r := range rows {
		ae := rendering.ArchiveEntry{
			ID:            r.ID,
			Slug:          r.Slug,
			ContentTypeID: contentTypeID,
			Title:         r.Title,
			Excerpt:       stringValue(r.Excerpt),
			URL:           r.RoutePath,
			PublishedAt:   formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, false),
			PublishedISO:  formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, true),
		}
		ae.Fields, _ = content.DecodeFieldSnapshot(r.FieldsJson)
		if r.FeaturedMediaID.Valid {
			if mv, ok := mediaCache[r.FeaturedMediaID.String]; ok {
				ae.FeaturedImage = mv
			}
		}
		out = append(out, ae)
	}
	return out
}

func (h *Handler) latestCollections(ctx context.Context, prepared *rendering.PreparedDocument, siteSnap *site.Snapshot, archive *rendering.ArchiveContext, currentEntryID string) map[string][]rendering.ArchiveEntry {
	if prepared == nil {
		return nil
	}
	type need struct {
		id    string
		limit int
	}
	var needs []need
	maxLimit := 0
	hasArchive := archive != nil
	var walk func([]rendering.PreparedNode)
	walk = func(nodes []rendering.PreparedNode) {
		for _, n := range nodes {
			if n.Block == "core/posts" {
				source, _ := n.Settings["source"].(string)
				if source == "" {
					source = "automatic"
				}
				// Alias backward compat: "archive" means automatic
				if source == "archive" {
					source = "automatic"
				}
				shouldFetch := false
				if source == "latest" {
					shouldFetch = true
				} else if source == "automatic" && !hasArchive {
					// automatic fallback to latest when no archive context
					shouldFetch = true
				}
				if shouldFetch {
					limit := 3
					if v, ok := n.Settings["limit"]; ok {
						switch val := v.(type) {
						case float64:
							limit = int(val)
						case int:
							limit = val
						case int64:
							limit = int(val)
						}
					}
					if limit < 1 {
						limit = 1
					}
					if limit > 20 {
						limit = 20
					}
					needs = append(needs, need{id: n.ID, limit: limit})
					if limit > maxLimit {
						maxLimit = limit
					}
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(prepared.Nodes)
	if len(needs) == 0 || maxLimit == 0 {
		return nil
	}
	rows, err := h.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{
		ContentTypeID: "post",
		Limit:         int64(maxLimit),
		Offset:        0,
	})
	if err != nil || len(rows) == 0 {
		return nil
	}
	// Batch media via real batch (bounded queries).
	featIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.FeaturedMediaID.Valid && r.FeaturedMediaID.String != "" {
			featIDs = append(featIDs, r.FeaturedMediaID.String)
		}
	}
	mediaCache := map[string]rendering.MediaView{}
	if h.media != nil && len(featIDs) > 0 {
		mediaCache = h.media.MediaViews(ctx, featIDs)
		if mediaCache == nil {
			mediaCache = map[string]rendering.MediaView{}
		}
	}
	full := make([]rendering.ArchiveEntry, 0, len(rows))
	for _, r := range rows {
		if currentEntryID != "" && r.ID == currentEntryID {
			continue
		}
		ae := rendering.ArchiveEntry{
			ID:           r.ID,
			Title:        r.Title,
			Excerpt:      stringValue(r.Excerpt),
			URL:          r.RoutePath,
			PublishedAt:  formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, false),
			PublishedISO: formatEntryDate(r.FirstPublishedAt, siteSnap.TimezoneName, true),
		}
		if r.FeaturedMediaID.Valid {
			if mv, ok := mediaCache[r.FeaturedMediaID.String]; ok {
				ae.FeaturedImage = mv
			}
		}
		full = append(full, ae)
	}
	// If filtering removed the current post, we may need to fetch one more to fill limits.
	// Simple: if we filtered and still need more items beyond fetched window, fetch extra.
	// For V1 we accept slight under-fill; handler fetches maxLimit which typically covers it.
	m := make(map[string][]rendering.ArchiveEntry, len(needs))
	for _, nd := range needs {
		lim := nd.limit
		if lim > len(full) {
			lim = len(full)
		}
		slice := make([]rendering.ArchiveEntry, lim)
		copy(slice, full[:lim])
		m[nd.id] = slice
	}
	return m
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

// lcpPreloads returns the single preload recorded during rendering.
// The renderer records the actual LCP image that claimed Priority=true
// (first eligible candidate that had a real image, per-instance for
// Collection). This is the single source of truth; handler does not
// duplicate LCP selection.
func (h *Handler) lcpPreloads(ctx context.Context, prepared *rendering.PreparedDocument, rc rendering.RenderContext) []themes.ImagePreload {
	if rc.LCP != nil && rc.LCP.PreloadHref != "" {
		sizes := rc.LCP.PreloadSizes
		if strings.TrimSpace(sizes) == "" {
			sizes = "(min-width: 768px) min(100vw, 1200px), 100vw"
		}
		return []themes.ImagePreload{{
			Href:   rc.LCP.PreloadHref,
			SrcSet: rc.LCP.PreloadSrcSet,
			Sizes:  sizes,
		}}
	}
	return nil
}

// renderThemedDocument renders a document as a fully themed page: it prepares
// the document, loads navigation, assembles the PageView and runs the theme
// runtime. The live public frontend and the editor previews share this exact
// path, so they cannot drift apart.
func (h *Handler) renderThemedDocument(ctx context.Context, siteSnap *site.Snapshot, doc *document.Document, rc rendering.RenderContext, resolved seo.Resolved, path string, temporary map[string]any, customCSS *string) ([]byte, string, error) {
	rc.FormReader = h.forms
	rc.FormCache = make(map[string]forms.FormView)
	if successID, ok := ctx.Value(formSuccessContextKey{}).(string); ok {
		rc.FormResult.SuccessFormID = successID
	}
	if rc.LCP == nil {
		rc.LCP = &rendering.LCPState{}
	}
	prepared, err := h.blocks.Prepare(doc)
	if err != nil {
		return nil, "", fmt.Errorf("prepare document: %w", err)
	}
	content, err := h.renderBlocks(ctx, prepared, rc)
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

func (h *Handler) renderBlocks(ctx context.Context, prepared *rendering.PreparedDocument, rc rendering.RenderContext) (template.HTML, error) {
	rc.FormReader = h.forms
	if rc.FormCache == nil {
		rc.FormCache = make(map[string]forms.FormView)
	}
	if successID, ok := ctx.Value(formSuccessContextKey{}).(string); ok {
		rc.FormResult.SuccessFormID = successID
	}
	return h.blocks.RenderPrepared(ctx, prepared, rc)
}

func (h *Handler) handleFormSubmit(w http.ResponseWriter, r *http.Request) {
	if h.forms == nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, forms.MaxPublicBodyBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusUnprocessableEntity)
		return
	}
	formID := strings.TrimPrefix(r.URL.Path, "/_stratum/forms/")
	if formID == "" || strings.Contains(formID, "/") {
		http.NotFound(w, r)
		return
	}
	returnTo := r.PostForm.Get("return_to")
	if !forms.ValidateReturnPath(returnTo) {
		http.Error(w, "Invalid form submission", http.StatusUnprocessableEntity)
		return
	}
	values := make(map[string][]string, len(r.PostForm))
	for key, value := range r.PostForm {
		if key == "return_to" || key == "website_confirm" {
			continue
		}
		values[key] = value
	}
	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		clientIP = host
	}
	_, err := h.forms.Submit(r.Context(), formID, forms.SubmitInput{Values: values, Honeypot: r.PostForm.Get("website_confirm"), ClientIP: clientIP, Now: time.Now()})
	if err != nil && !errors.Is(err, forms.ErrHoneypot) {
		switch {
		case errors.Is(err, forms.ErrRateLimited):
			http.Error(w, "Too many submissions", http.StatusTooManyRequests)
		case errors.Is(err, forms.ErrNotFound):
			http.NotFound(w, r)
		default:
			http.Error(w, "Invalid form submission", http.StatusUnprocessableEntity)
		}
		return
	}
	target, _ := url.Parse(returnTo)
	query := target.Query()
	query.Set("form_success", formID)
	target.RawQuery = query.Encode()
	target.Fragment = "form-" + formID
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func (h *Handler) serveSitemap(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil || siteSnap.SitemapEnabled == false || strings.TrimSpace(siteSnap.SiteURL) == "" {
		http.NotFound(w, r)
		return
	}
	body, gz, br, etag, ok := h.hub.Sitemap.GetBrotli()
	if !ok {
		built, err := h.buildSitemap(r.Context(), siteSnap)
		if err != nil {
			log.Printf("sitemap build: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		gz, _ = compress.Gzip(built)
		br, _ = compress.Brotli(built)
		// compression errors are non-fatal: raw remains usable, compressed variants may be empty
		etag = pagecache.ComputeETag(built)
		h.hub.Sitemap.SetWithBrotli(built, gz, br, etag)
		body = built
	}
	h.writeText(w, r, body, gz, br, etag, "application/xml; charset=utf-8", "public, max-age=300")
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
	// Add the main posts archive (page 1 only) if indexable. Prefer routes (source of truth).
	if siteSnap.IndexingEnabled {
		archivePaths, err := h.queries.ListSitemapArchiveRoutes(ctx)
		var candidates []string
		if err == nil && len(archivePaths) > 0 {
			candidates = archivePaths
			if siteSnap.HomepageMode == "latest_posts" {
				found := false
				for _, p := range archivePaths {
					if p == "/" {
						found = true
						break
					}
				}
				if !found {
					candidates = append(candidates, "/")
				}
			}
		} else {
			arch := seo.PostsArchivePath(siteSnap.PostsBasePath)
			if siteSnap.HomepageMode == "latest_posts" {
				arch = "/"
			}
			candidates = []string{arch}
		}
		for _, p := range candidates {
			// Skip pagination children (should not be in candidates, but guard)
			if strings.Contains(p, "/page/") {
				continue
			}
			// Determine if archive is indexable: check shell robots if exists
			indexable := true
			if rt, rerr := h.queries.GetRouteByPath(ctx, p); rerr == nil && rt.RouteType == "archive" && rt.EntryID.Valid {
				if shell, serr := h.queries.GetPublishedEntryByID(ctx, rt.EntryID.String); serr == nil {
					if shell.SeoRobotsIndex.Valid && shell.SeoRobotsIndex.Int64 == 0 {
						indexable = false
					}
				}
			}
			if !indexable {
				continue
			}
			// Deduplicate when homepage is latest_posts and /blog is redirect (only canonical "/")
			// ListSitemapArchiveRoutes only returns archive type, so redirects are already excluded.
			// If both "/" and "/blog" somehow present, keep only the active mount (the one matching desiredMount)
			// For V1 we keep all indexable archive candidates, but skip adding duplicate "/" when already present.
			lastmod := h.archiveLastmod(ctx, p)
			urlset.URLs = append(urlset.URLs, sitemapURL{
				Loc:     base + p,
				Lastmod: time.Unix(lastmod, 0).UTC().Format(time.RFC3339),
			})
		}
	}
	return h.marshalSitemap(urlset)
}

func (h *Handler) archiveLastmod(ctx context.Context, archivePath string) int64 {
	var last int64
	// Shell published revision timestamp
	if rt, err := h.queries.GetRouteByPath(ctx, archivePath); err == nil && rt.RouteType == "archive" && rt.EntryID.Valid {
		if shell, err := h.queries.GetPublishedEntryByID(ctx, rt.EntryID.String); err == nil {
			if shell.PublishedAt.Valid && shell.PublishedAt.Int64 > last {
				last = shell.PublishedAt.Int64
			}
			if shell.FirstPublishedAt.Valid && shell.FirstPublishedAt.Int64 > last {
				last = shell.FirstPublishedAt.Int64
			}
			// Use revision created_at as fallback (published_at is authoritative)
		}
	}
	// Newest post affecting listing
	if rows, err := h.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: "post", Limit: 1, Offset: 0}); err == nil && len(rows) > 0 {
		if rows[0].FirstPublishedAt.Valid && rows[0].FirstPublishedAt.Int64 > last {
			last = rows[0].FirstPublishedAt.Int64
		} else if rows[0].PublishedAt.Valid && rows[0].PublishedAt.Int64 > last {
			last = rows[0].PublishedAt.Int64
		}
	}
	if last == 0 {
		// Fallback to now only if no data – but keep deterministic by using site settings updatedAt?
		// Use 0 -> will format as 1970, but we prefer not to emit empty; fallback to current is nondeterministic,
		// so we keep 0 as is and caller will format 1970 which is deterministic, though not ideal.
		// Instead use time.Now only if we want non-deterministic – we avoid.
		// For empty site, use time.Now truncated? Keep last as 0 -> will be 1970-01-01, but tests only check stability, not value.
		// Better to use time.Now only once at startup? We'll just leave last as time.Now trick but ensure stable across calls by using max of known timestamps.
		// If still 0, return time.Now stripped? But that would be nondeterministic across builds (spec 32 forbids).
		// So return 0 -> formatted as 1970, stable.
	}
	return last
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
	body, gz, br, etag, ok := h.hub.Robots.GetBrotli()
	if !ok {
		built := site.BuildRobots(site.RobotsInput{
			Mode:            siteSnap.RobotsMode,
			IndexingEnabled: siteSnap.IndexingEnabled,
			SitemapEnabled:  siteSnap.SitemapEnabled,
			SiteURL:         siteSnap.SiteURL,
			Custom:          siteSnap.RobotsCustom,
		})
		gz, _ = compress.Gzip([]byte(built))
		br, _ = compress.Brotli([]byte(built))
		etag = pagecache.ComputeETag([]byte(built))
		h.hub.Robots.SetWithBrotli([]byte(built), gz, br, etag)
		body = []byte(built)
	}
	h.writeText(w, r, body, gz, br, etag, "text/plain; charset=utf-8", "public, max-age=300")
}

func (h *Handler) writeText(w http.ResponseWriter, r *http.Request, body, gz, br []byte, etag, ctype, cacheControl string) {
	w.Header().Set("Vary", "Accept-Encoding")
	if etagWeakMatch(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", cacheControl)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	enc, ok := compress.NegotiateEncoding(r.Header.Get("Accept-Encoding"))
	if !ok {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
		return
	}
	switch enc {
	case "br":
		if len(br) > 0 {
			w.Header().Set("Content-Encoding", "br")
			w.Header().Set("Content-Length", strconv.Itoa(len(br)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(br)
			}
			return
		}
	case "gzip":
		if len(gz) > 0 {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(len(gz)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(gz)
			}
			return
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// serveMedia streams a stored media derivative using Range-capable serving.
// Warm requests use the in-memory serve metadata cache (zero SQLite).
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
	// ETag from serve metadata (content hash) for immutable variants.
	if _, _, etag, _, ok := h.media.ServeMeta(r.Context(), id, kind); ok && etag != "" {
		w.Header().Set("ETag", etag)
	}
	http.ServeContent(w, r, id+kind, time.Time{}, f)
	_ = size
}

// WarmCache pre-renders the homepage and main archive (page 1) into the page
// cache so a restart does not start cold. Failures are logged and do not
// affect the caller; the cache simply stays empty for that path.
func (h *Handler) WarmCache(ctx context.Context) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil {
		return
	}
	paths := []string{"/"}
	arch := routing.PostsArchivePath(siteSnap.PostsBasePath)
	if siteSnap.HomepageMode != "latest_posts" && arch != "/" && arch != "" {
		// Avoid duplicate "/" when homepage is latest posts
		found := false
		for _, p := range paths {
			if p == arch {
				found = true
				break
			}
		}
		if !found {
			paths = append(paths, arch)
		}
	}
	origin := strings.TrimSpace(siteSnap.SiteURL)
	if origin == "" {
		origin = "http://warmup.invalid"
	}
	for _, p := range paths {
		// Only warm if route exists or is homepage archive; otherwise skip.
		if p != "/" {
			if rt, ok := h.hub.Routes.Lookup(p); !ok || rt.RouteType != routing.RouteTypeArchive {
				// Still allow entry at that path? For safety, try anyway
			}
		}
		entry, err := h.renderPage(ctx, origin, p)
		if err != nil {
			continue
		}
		key := pagecache.Key("", p)
		if siteSnap.SiteURL == "" {
			// When Site URL is empty, cache key includes origin; use warmup origin as well.
			// Production with SiteURL set uses empty origin key, so warm with empty origin.
			// Keep both for dev compatibility: store under warmup origin key; actual request with empty SiteURL will use request origin, so it won't hit warm.
			// For correctness we store under empty origin key when SiteURL is set, else skip warming.
			if origin == "http://warmup.invalid" {
				continue
			}
		}
		h.hub.Pages.Set(key, entry, entry.Tags...)
	}
}

// serveFavicon redirects /favicon.ico to the current versioned favicon variant.
// The versioned URL includes content hash so regenerated favicons do not stay stale behind immutable cache.
func (h *Handler) serveFavicon(w http.ResponseWriter, r *http.Request) {
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil || siteSnap.SiteIconMediaID == "" {
		http.NotFound(w, r)
		return
	}
	view, ok := h.media.FaviconView(r.Context(), siteSnap.SiteIconMediaID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	target := view.Size32
	if target == "" {
		// Fallback to any available size
		switch {
		case view.Size16 != "":
			target = view.Size16
		case view.Size180 != "":
			target = view.Size180
		case view.Size192 != "":
			target = view.Size192
		case view.Size512 != "":
			target = view.Size512
		default:
			http.NotFound(w, r)
			return
		}
	}
	http.Redirect(w, r, target, http.StatusFound)
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

// RenderInput is a type alias to rendering.RenderInput so admin preview does
// not need to import the public HTTP package. Canonical definition lives in
// internal/rendering.
type RenderInput = rendering.RenderInput

func stringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

// parseArchivePagination is kept for backward compatibility; new code should use routing.ParsePagination.
func parseArchivePagination(path string) (base string, page int, ok bool) {
	return routing.ParsePagination(path)
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
