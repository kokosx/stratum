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
	return hub, nil
}

// --- Invalidation: each method maps a domain event to reload + cache drops ---

// ReloadSite reloads the site settings snapshot and invalidates everything that
// depends on it (full-page HTML, sitemap, robots).
func (h *Runtime) ReloadSite(ctx context.Context) error {
	if err := h.Site.Reload(ctx); err != nil {
		return err
	}
	h.Pages.InvalidateAll()
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
	h.Pages.InvalidateAll()
	return nil
}

// ReloadTheme reloads the theme snapshot, rebuilds asset fingerprints, and
// invalidates the page cache (theme styles/custom CSS changed).
func (h *Runtime) ReloadTheme(ctx context.Context) error {
	if err := h.Themes.Reload(ctx); err != nil {
		return err
	}
	h.Assets.Rebuild(h.Blocks, h.Themes)
	h.Pages.InvalidateAll()
	return nil
}

// ReloadBlocks reloads the block registry (which also clears its prepared
// document cache), rebuilds asset fingerprints, and invalidates the page cache.
func (h *Runtime) ReloadBlocks(ctx context.Context) error {
	if err := h.Blocks.Reload(ctx); err != nil {
		return err
	}
	h.Assets.Rebuild(h.Blocks, h.Themes)
	h.Pages.InvalidateAll()
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
// robots and feed must all be rebuilt.
func (h *Runtime) InvalidateContent() {
	h.Pages.InvalidateAll()
	h.Sitemap.Invalidate()
	h.Robots.Invalidate()
	h.Feed.Invalidate()
}

// InvalidateLayoutTemplates is called after publishing a Layout Template.
// Only the page cache is invalidated; routes/sitemap/robots are unaffected.
func (h *Runtime) InvalidateLayoutTemplates() {
	h.Pages.InvalidateAll()
}
