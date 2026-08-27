// Package runtimehub wires the public runtime together: the immutable snapshots
// (site settings, navigation, theme, blocks) and the rebuildable in-memory
// caches (full-page HTML, fingerprinted assets, sitemap, robots).
//
// It is the single, readable place that maps "something changed" to the runtime
// reload and cache invalidation it requires. Admin write paths call these
// methods; the public handler reads the snapshots and serves from the caches.
package runtimehub

import (
	"context"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/pagecache"
	"github.com/kokosx/stratum/internal/routing"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

// Runtime is the public runtime coordinator. It is safe for concurrent use by
// many request goroutines and by admin write paths.
type Runtime struct {
	Queries    *db.Queries
	Blocks     *blocks.Registry
	Themes     *themes.Runtime
	Media      *media.Service
	Site       *site.Runtime
	Navigation *navigation.Runtime
	Routes     *routing.Runtime

	Pages   *PageCache
	Assets  *AssetManifest
	Sitemap *SitemapCache
	Robots  *RobotsCache
	Feed    *FeedCache
}

// New builds every runtime snapshot and cache. It performs the initial reloads
// so the public handler is ready to serve immediately.
func New(queries *db.Queries, blocks *blocks.Registry, themes *themes.Runtime, mediaSvc *media.Service) (*Runtime, error) {
	hub := &Runtime{
		Queries:    queries,
		Blocks:     blocks,
		Themes:     themes,
		Media:      mediaSvc,
		Site:       site.NewRuntime(queries),
		Navigation: navigation.NewRuntime(queries),
		Routes:     routing.NewRuntime(queries),
		Pages:      pagecache.New(),
		Assets:     NewAssetManifest(blocks, themes),
		Sitemap:    NewSitemapCache(),
		Robots:     NewRobotsCache(),
		Feed:       NewFeedCache(),
	}
	if err := hub.Site.Reload(context.Background()); err != nil {
		return nil, err
	}
	if err := hub.Navigation.Reload(context.Background()); err != nil {
		return nil, err
	}
	if err := hub.Routes.Reload(context.Background()); err != nil {
		return nil, err
	}
	return hub, nil
}

// --- Invalidation: each method maps a domain event to reload + cache drops ---

// ReloadSite reloads the site settings snapshot and invalidates everything that
// depends on it (full-page HTML, sitemap, robots).
func (h *Runtime) ReloadSite(ctx context.Context) error {
	if err := h.Site.Reload(ctx); err != nil {
		return err
	}
	h.Pages.InvalidateTag("site")
	h.Sitemap.Invalidate()
	h.Robots.Invalidate()
	h.Feed.Invalidate()
	return nil
}

// ReloadNavigation reloads the navigation snapshot and invalidates the page cache.
func (h *Runtime) ReloadNavigation(ctx context.Context) error {
	if err := h.Navigation.Reload(ctx); err != nil {
		return err
	}
	h.Pages.InvalidateTag("navigation")
	return nil
}

// ReloadTheme reloads the theme snapshot, rebuilds asset fingerprints, and
// invalidates the page cache (theme styles/custom CSS changed).
func (h *Runtime) ReloadTheme(ctx context.Context) error {
	if err := h.Themes.Reload(ctx); err != nil {
		return err
	}
	h.Assets.Rebuild(h.Blocks, h.Themes)
	h.Pages.InvalidateTag("theme")
	return nil
}

// ReloadBlocks reloads the block registry (which also clears its prepared
// document cache), rebuilds asset fingerprints, and invalidates the page cache.
func (h *Runtime) ReloadBlocks(ctx context.Context) error {
	if err := h.Blocks.Reload(ctx); err != nil {
		return err
	}
	h.Assets.Rebuild(h.Blocks, h.Themes)
	// Block changes may affect any page (styles/hashes), so invalidate all via theme tag
	h.Pages.InvalidateTag("theme")
	return nil
}

// InvalidateMedia drops the cached media views and invalidates the page cache
// (a rendered image changed).
func (h *Runtime) InvalidateMedia(id string) {
	h.Media.InvalidateView(id)
	h.Pages.InvalidateAll()
}

// InvalidateMediaAll drops every cached media view and invalidates the page cache.
func (h *Runtime) InvalidateMediaAll() {
	h.Media.InvalidateAllViews()
	h.Pages.InvalidateAll()
}

// InvalidateContent is called after a publish, unpublish, route change, or
// trash. The published document and route changed, so the page cache, sitemap,
// robots and feed must all be rebuilt. Kept for compatibility; selective
// invalidation via InvalidateEntry is preferred.
func (h *Runtime) InvalidateContent() {
	h.Pages.InvalidateAll()
	h.Sitemap.Invalidate()
	h.Robots.Invalidate()
	h.Feed.Invalidate()
}

// InvalidateEntry selectively invalidates pages affected by an entry publish.
// For pages/posts it drops the entry's own page and any collection/archive that
// depends on its content type.
func (h *Runtime) InvalidateEntry(entryID, contentTypeID string) {
	// Reload routes first (entry route may have changed)
	_ = h.Routes.Reload(context.Background())
	h.Pages.InvalidateTag("entry:" + entryID)
	if contentTypeID == "post" {
		h.Pages.InvalidateTag("content-type:" + contentTypeID)
	} else {
		// For generic archived types, also check if type is archived
		// Use tag regardless; only pages with that tag will be flushed.
		h.Pages.InvalidateTag("content-type:" + contentTypeID)
	}
	h.Sitemap.Invalidate()
	h.Feed.Invalidate()
}

// InvalidateLayoutTemplates is called after publishing a Layout Template.
// Only the page cache is invalidated; routes/sitemap/robots are unaffected.
func (h *Runtime) InvalidateLayoutTemplates() {
	h.Pages.InvalidateAll()
}

// InvalidateLayoutTemplate selectively invalidates pages using the given template.
func (h *Runtime) InvalidateLayoutTemplate(templateID string) {
	if templateID != "" {
		h.Pages.InvalidateTag("layout:" + templateID)
		h.Pages.InvalidateTag("template:" + templateID)
	} else {
		h.Pages.InvalidateAll()
	}
}

// InvalidateTemplate selectively invalidates pages using the given template (generic).
func (h *Runtime) InvalidateTemplate(templateID string) {
	if templateID != "" {
		h.Pages.InvalidateTag("template:" + templateID)
		h.Pages.InvalidateTag("layout:" + templateID)
	} else {
		h.Pages.InvalidateAll()
	}
}

// InvalidateSitePart selectively invalidates pages using the given site part.
func (h *Runtime) InvalidateSitePart(sitePartID string) {
	if sitePartID != "" {
		h.Pages.InvalidateTag("site-part:" + sitePartID)
	} else {
		h.Pages.InvalidateAll()
	}
}

// InvalidateAll clears the full page cache.
func (h *Runtime) InvalidateAll() {
	h.Pages.InvalidateAll()
}

// ReloadRoutes reloads the immutable route snapshot and invalidates the page
// cache. It keeps the old snapshot active on error.
func (h *Runtime) ReloadRoutes(ctx context.Context) error {
	if err := h.Routes.Reload(ctx); err != nil {
		return err
	}
	h.Pages.InvalidateAll()
	h.Sitemap.Invalidate()
	return nil
}
