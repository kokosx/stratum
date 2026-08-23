package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const (
	seedHomeEntryID    = "seed-home"
	seedHomeRevisionID = "seed-home-r1"
	seedBlogEntryID    = "seed-blog"
	seedBlogRevisionID = "seed-blog-r1"
	seedAboutEntryID   = "seed-about"
	seedAboutRevisionID = "seed-about-r1"
)

const seedHomeDocument = `{"version":1,"nodes":[{"id":"hero","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"xl","horizontalPadding":"md","align":"center","background":"muted","minHeight":"auto"},"children":[{"id":"hero-heading","block":"core/heading","version":1,"props":{"text":"Welcome to StratumCMS","level":1},"settings":{"align":"center","visualSize":"xl","tone":"default","maxWidth":"none"}},{"id":"hero-intro","block":"core/text","version":1,"props":{"text":"A modern, single-binary CMS with WordPress familiarity. Pages, posts, revisions, media — all as structured documents, not HTML blobs."},"settings":{"align":"center","tone":"muted","size":"lg","maxWidth":"none"}},{"id":"hero-buttons","block":"core/button-group","version":1,"props":{},"settings":{"direction":"horizontal","gap":"md","align":"center","wrap":true},"children":[{"id":"hero-cta-primary","block":"core/button","version":1,"props":{"label":"Explore the Blog","url":"/blog"},"settings":{"variant":"primary","size":"lg","width":"auto","align":"left","openInNewTab":false}},{"id":"hero-cta-secondary","block":"core/button","version":1,"props":{"label":"Learn More","url":"/about"},"settings":{"variant":"outline","size":"lg","width":"auto","align":"left","openInNewTab":false}}]}]},{"id":"features-section","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto"},"children":[{"id":"features-heading","block":"core/heading","version":1,"props":{"text":"Everything you need to publish","level":2},"settings":{"align":"center","visualSize":"md","tone":"default","maxWidth":"none"}},{"id":"features-text","block":"core/text","version":1,"props":{"text":"Create pages, publish posts, manage media and menus — with revisions, routes and a theme that makes it all look good."},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"narrow"}},{"id":"features-grid","block":"core/grid","version":1,"props":{},"settings":{"columns":3,"gap":"lg","align":"stretch","equalHeight":false},"children":[{"id":"card-pages","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[{"id":"card-pages-heading","block":"core/heading","version":1,"props":{"text":"Pages & Posts","level":3},"settings":{"align":"left","visualSize":"sm","tone":"default","maxWidth":"none"}},{"id":"card-pages-text","block":"core/text","version":1,"props":{"text":"Structured content with revisions and routes. Editing never touches the live site until you publish."},"settings":{"align":"left","tone":"muted","size":"sm","maxWidth":"none"}}]},{"id":"card-blocks","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[{"id":"card-blocks-heading","block":"core/heading","version":1,"props":{"text":"Blocks, not HTML","level":3},"settings":{"align":"left","visualSize":"sm","tone":"default","maxWidth":"none"}},{"id":"card-blocks-text","block":"core/text","version":1,"props":{"text":"Composable blocks keep content independent from presentation. Themes control the final markup."},"settings":{"align":"left","tone":"muted","size":"sm","maxWidth":"none"}}]},{"id":"card-themes","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[{"id":"card-themes-heading","block":"core/heading","version":1,"props":{"text":"Themes & Tokens","level":3},"settings":{"align":"left","visualSize":"sm","tone":"default","maxWidth":"none"}},{"id":"card-themes-text","block":"core/text","version":1,"props":{"text":"Semantic design tokens drive the whole site. Change a color once — it flows everywhere."},"settings":{"align":"left","tone":"muted","size":"sm","maxWidth":"none"}}]}]}]},{"id":"divider-1","block":"core/divider","version":1,"props":{},"settings":{"style":"solid","width":"content","spacing":"md"}},{"id":"latest-section","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto"},"children":[{"id":"latest-heading","block":"core/heading","version":1,"props":{"text":"Latest posts","level":2},"settings":{"align":"left","visualSize":"md","tone":"default","maxWidth":"none"}},{"id":"latest-collection","block":"core/collection","version":1,"props":{},"settings":{"contentType":"post","limit":3,"offset":0,"order":"published_desc","source":"query","excludeCurrent":false},"children":[{"id":"latest-item-stack","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"sm","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[{"id":"latest-item-date","block":"core/entry-publish-date","version":1,"props":{},"settings":{"format":"long","align":"left"}},{"id":"latest-item-title","block":"core/entry-title","version":1,"props":{},"settings":{"level":3,"align":"left","visualSize":"md","tone":"default","maxWidth":"none"}},{"id":"latest-item-excerpt","block":"core/entry-excerpt","version":1,"props":{},"settings":{"align":"left","tone":"muted","size":"md"}},{"id":"latest-item-link","block":"core/entry-link","version":1,"props":{"text":"Continue reading"},"settings":{"openInNewTab":false}}]}]},{"id":"latest-cta","block":"core/button","version":1,"props":{"label":"View all posts","url":"/blog"},"settings":{"variant":"secondary","size":"md","width":"auto","align":"center","openInNewTab":false}}]}]}`

const seedBlogDocument = `{"version":1,"nodes":[{"id":"blog-header","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"lg","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto"},"children":[{"id":"blog-title","block":"core/heading","version":1,"props":{"text":"Blog","level":1},"settings":{"align":"left","visualSize":"lg","tone":"default","maxWidth":"none"}},{"id":"blog-intro","block":"core/text","version":1,"props":{"text":"Thoughts, updates and stories from the Stratum team. New posts appear here automatically as you publish."},"settings":{"align":"left","tone":"muted","size":"lg","maxWidth":"narrow"}}]},{"id":"blog-collection","block":"core/collection","version":1,"props":{},"settings":{"contentType":"post","limit":10,"offset":0,"order":"published_desc","source":"context","excludeCurrent":false},"children":[{"id":"blog-item","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"sm","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[{"id":"blog-item-date","block":"core/entry-publish-date","version":1,"props":{},"settings":{"format":"long","align":"left"}},{"id":"blog-item-title","block":"core/entry-title","version":1,"props":{},"settings":{"level":2,"align":"left","visualSize":"md","tone":"default","maxWidth":"none"}},{"id":"blog-item-excerpt","block":"core/entry-excerpt","version":1,"props":{},"settings":{"align":"left","tone":"muted","size":"md"}},{"id":"blog-item-link","block":"core/entry-link","version":1,"props":{"text":"Read more"},"settings":{"openInNewTab":false}}]}]}]}`

const seedAboutDocument = `{"version":1,"nodes":[{"id":"about-heading","block":"core/heading","version":1,"props":{"text":"About Stratum","level":1},"settings":{"align":"left","visualSize":"lg","tone":"default","maxWidth":"none"}},{"id":"about-intro","block":"core/text","version":1,"props":{"text":"StratumCMS is a focused, self-hosted CMS built around structured content. It gives you WordPress familiarity with a modern architecture — one binary, SQLite, server-rendered HTML."},"settings":{"align":"left","tone":"default","size":"lg","maxWidth":"narrow"}},{"id":"about-grid","block":"core/grid","version":1,"props":{},"settings":{"columns":2,"gap":"md","align":"stretch","equalHeight":false},"children":[{"id":"about-card-1","block":"core/card","version":1,"props":{},"settings":{"variant":"muted","padding":"md","radius":"md","align":"start"},"children":[{"id":"about-card-1-title","block":"core/heading","version":1,"props":{"text":"One binary","level":3},"settings":{"align":"left","visualSize":"sm","tone":"default","maxWidth":"none"}},{"id":"about-card-1-text","block":"core/text","version":1,"props":{"text":"Go, SQLite and a single deployable. No external dependencies for the core."},"settings":{"align":"left","tone":"muted","size":"sm","maxWidth":"none"}}]},{"id":"about-card-2","block":"core/card","version":1,"props":{},"settings":{"variant":"muted","padding":"md","radius":"md","align":"start"},"children":[{"id":"about-card-2-title","block":"core/heading","version":1,"props":{"text":"Structured Documents","level":3},"settings":{"align":"left","visualSize":"sm","tone":"default","maxWidth":"none"}},{"id":"about-card-2-text","block":"core/text","version":1,"props":{"text":"Content is a document tree with stable block IDs, versioned schemas and real revisions."},"settings":{"align":"left","tone":"muted","size":"sm","maxWidth":"none"}}]}]},{"id":"about-cta","block":"core/button","version":1,"props":{"label":"Back to home","url":"/"},"settings":{"variant":"secondary","size":"md","width":"auto","align":"left","openInNewTab":false}}]}`

const seedPostDocument = `{"version":1,"nodes":[{"id":"post-intro","block":"core/text","version":1,"props":{"text":"Stratum stores content as a structured document tree — not HTML. Each block has a stable ID, a versioned schema and clean props."},"settings":{"align":"left","tone":"muted","size":"lg","maxWidth":"narrow"}},{"id":"post-heading-1","block":"core/heading","version":1,"props":{"text":"Why documents, not HTML","level":2},"settings":{"align":"left","visualSize":"md","tone":"default","maxWidth":"none"}},{"id":"post-text-1","block":"core/text","version":1,"props":{"text":"HTML mixes content with presentation. By storing a document tree, themes stay in control of markup, layouts and CSS — and the editor can evolve without migrations."},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}},{"id":"post-callout","block":"core/callout","version":1,"props":{"title":"Tip","text":"Try changing the Primary color in Appearance → Site Styles. Every button, link and highlight updates — no CSS hunting."},"settings":{"variant":"info","icon":true}},{"id":"post-heading-2","block":"core/heading","version":1,"props":{"text":"Blocks you can trust","level":2},"settings":{"align":"left","visualSize":"md","tone":"default","maxWidth":"none"}},{"id":"post-list","block":"core/list","version":1,"props":{"items":"Headings, text and buttons\nSections, stacks, grids and cards\nCallouts, quotes, dividers and code\nCollections that render live entries"},"settings":{"ordered":false,"marker":"check","start":1}},{"id":"post-divider","block":"core/divider","version":1,"props":{},"settings":{"style":"solid","width":"content","spacing":"md"}},{"id":"post-closing","block":"core/text","version":1,"props":{"text":"This is a seeded example post. Edit it, publish a new revision, or delete it — your site is yours from here."},"settings":{"align":"left","tone":"muted","size":"md","maxWidth":"none"}},{"id":"post-cta","block":"core/button","version":1,"props":{"label":"Back to blog","url":"/blog"},"settings":{"variant":"secondary","size":"md","width":"auto","align":"left","openInNewTab":false}}]}`

// Seed adds a coherent, published starter site without replacing existing content.
// It is idempotent within a setup transaction and never overwrites user content.
// On a genuinely fresh installation (no entries yet) it creates:
//   - a polished homepage at /
//   - a Blog archive shell at /blog
//   - an About page
//   - one example post below /blog/
//   - sensible primary/footer navigation
//   - reading settings (homepage + posts page + /blog base)
func (d *Database) Seed(ctx context.Context) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer tx.Rollback()

	queries := db.New(tx)

	// Fresh-install guard: if any entry exists, the site is not fresh.
	// This keeps a second Seed call during setup idempotent (ON CONFLICT DO NOTHING)
	// and prevents re-creation after the user deletes starter content when Seed
	// is not invoked automatically on every boot (which we guarantee).
	if count, err := queries.CountEntries(ctx); err == nil && count > 0 {
		// Still ensure site settings are at least coherent if they were never set.
		// But do not recreate entries/routes/menus.
		_ = tx.Commit()
		return nil
	}

	now := time.Now().Unix()

	// --- Entries ---
	type seedEntry struct {
		id, revisionID, routeID, contentType, slug, path, title, excerpt, document string
		routeType string // entry or archive
	}
	entries := []seedEntry{
		{seedHomeEntryID, seedHomeRevisionID, "seed-home-route", "page", "home", "/", "Welcome to StratumCMS", "A modern, single-binary CMS with WordPress familiarity.", seedHomeDocument, "entry"},
		{seedBlogEntryID, seedBlogRevisionID, "seed-blog-route", "page", "seed-blog", "/blog", "Blog", "Thoughts, updates and stories.", seedBlogDocument, "archive"},
		{seedAboutEntryID, seedAboutRevisionID, "seed-about-route", "page", "about", "/about", "About Stratum", "Learn about this example site.", seedAboutDocument, "entry"},
		{"seed-post-hello", "seed-post-hello-r1", "seed-post-hello-route", "post", "hello-world", "/blog/hello-world", "Hello World — Your First Post", "An example post to show how Stratum renders structured documents.", seedPostDocument, "entry"},
		// Keep two additional lightweight posts for collection demo (optional, no junk)
		{"seed-post-blocks", "seed-post-blocks-r1", "seed-post-blocks-route", "post", "content-blocks", "/blog/content-blocks", "Content Blocks, Not HTML", "Why documents are stored as structured data.", `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"Content Blocks, Not HTML","level":1},"settings":{"align":"left","visualSize":"lg"}},{"id":"t1","block":"core/text","version":1,"props":{"text":"Content remains independent from presentation, while themes control the final markup."},"settings":{"tone":"muted","size":"lg"}}]}`, "entry"},
		{"seed-post-basics", "seed-post-basics-r1", "seed-post-basics-route", "post", "building-the-basics", "/blog/building-the-basics", "Building the Basics", "Pages, posts, revisions, routes and rendering.", `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"Building the Basics","level":1},"settings":{"align":"left","visualSize":"lg"}},{"id":"t1","block":"core/text","version":1,"props":{"text":"Pages, posts, revisions, routes and rendering form the first useful vertical slice."},"settings":{"tone":"muted","size":"lg"}}]}`, "entry"},
	}

	for _, e := range entries {
		if err := queries.SeedEntry(ctx, db.SeedEntryParams{
			ID: e.id, ContentTypeID: e.contentType, Slug: e.slug,
			CreatedAt: now, UpdatedAt: now, PublishedAt: sql.NullInt64{Int64: now, Valid: true},
		}); err != nil {
			return fmt.Errorf("seed %s entry: %w", e.slug, err)
		}
		if err := queries.SeedEntryRevision(ctx, db.SeedEntryRevisionParams{
			ID: e.revisionID, EntryID: e.id, Title: e.title,
			Excerpt: sql.NullString{String: e.excerpt, Valid: true}, DocumentJson: e.document,
			SeoTitle: sql.NullString{String: e.title, Valid: true}, SeoDescription: sql.NullString{String: e.excerpt, Valid: true}, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed %s revision: %w", e.slug, err)
		}
		if err := queries.SeedPublishedRevision(ctx, db.SeedPublishedRevisionParams{
			PublishedRevisionID: sql.NullString{String: e.revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, FirstPublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: e.id,
		}); err != nil {
			return fmt.Errorf("publish seeded %s revision: %w", e.slug, err)
		}
		if e.routeType == "archive" {
			if err := queries.SeedArchiveRoute(ctx, db.SeedArchiveRouteParams{
				ID: e.routeID, Path: e.path, EntryID: sql.NullString{String: e.id, Valid: true}, ContentTypeID: sql.NullString{String: "post", Valid: true}, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return fmt.Errorf("seed %s archive route: %w", e.slug, err)
			}
		} else {
			if err := queries.SeedRoute(ctx, db.SeedRouteParams{
				ID: e.routeID, Path: e.path, EntryID: sql.NullString{String: e.id, Valid: true}, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return fmt.Errorf("seed %s route: %w", e.slug, err)
			}
		}
	}

	if err := queries.SeedSiteSettings(ctx, db.SeedSiteSettingsParams{
		HomepageEntryID:  sql.NullString{String: seedHomeEntryID, Valid: true},
		PostsPageEntryID: sql.NullString{String: seedBlogEntryID, Valid: true},
		PostsBasePath:    "/blog",
		UpdatedAt:        now,
	}); err != nil {
		return fmt.Errorf("seed site settings: %w", err)
	}

	// --- Navigation (reuse default menus created by migration) ---
	// Ensure default menus exist (idempotent) and populate with sensible starter items.
	// Primary: Home, Blog, About — Footer: About, Blog
	if err := queries.SeedNavigationMenu(ctx, db.SeedNavigationMenuParams{ID: "default-main-navigation", Name: "Main Navigation", Slug: "main-navigation", CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("seed primary menu: %w", err)
	}
	if err := queries.SeedNavigationMenu(ctx, db.SeedNavigationMenuParams{ID: "default-footer-navigation", Name: "Footer", Slug: "footer", CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("seed footer menu: %w", err)
	}
	primaryItems := []db.SeedNavigationItemParams{
		{ID: "seed-nav-home", MenuID: "default-main-navigation", ParentID: sql.NullString{}, Position: 0, Label: "Home", TargetType: "entry", EntryID: sql.NullString{String: seedHomeEntryID, Valid: true}, Url: sql.NullString{}, CreatedAt: now, UpdatedAt: now},
		{ID: "seed-nav-blog", MenuID: "default-main-navigation", ParentID: sql.NullString{}, Position: 1, Label: "Blog", TargetType: "entry", EntryID: sql.NullString{String: seedBlogEntryID, Valid: true}, Url: sql.NullString{}, CreatedAt: now, UpdatedAt: now},
		{ID: "seed-nav-about", MenuID: "default-main-navigation", ParentID: sql.NullString{}, Position: 2, Label: "About", TargetType: "entry", EntryID: sql.NullString{String: seedAboutEntryID, Valid: true}, Url: sql.NullString{}, CreatedAt: now, UpdatedAt: now},
	}
	for _, it := range primaryItems {
		if err := queries.SeedNavigationItem(ctx, it); err != nil {
			return fmt.Errorf("seed nav item %s: %w", it.Label, err)
		}
	}
	footerItems := []db.SeedNavigationItemParams{
		{ID: "seed-nav-footer-about", MenuID: "default-footer-navigation", ParentID: sql.NullString{}, Position: 0, Label: "About", TargetType: "entry", EntryID: sql.NullString{String: seedAboutEntryID, Valid: true}, Url: sql.NullString{}, CreatedAt: now, UpdatedAt: now},
		{ID: "seed-nav-footer-blog", MenuID: "default-footer-navigation", ParentID: sql.NullString{}, Position: 1, Label: "Blog", TargetType: "entry", EntryID: sql.NullString{String: seedBlogEntryID, Valid: true}, Url: sql.NullString{}, CreatedAt: now, UpdatedAt: now},
	}
	for _, it := range footerItems {
		if err := queries.SeedNavigationItem(ctx, it); err != nil {
			return fmt.Errorf("seed footer item %s: %w", it.Label, err)
		}
	}
	if err := queries.SeedNavigationLocation(ctx, db.SeedNavigationLocationParams{Location: "primary", MenuID: "default-main-navigation"}); err != nil {
		return fmt.Errorf("seed primary location: %w", err)
	}
	if err := queries.SeedNavigationLocation(ctx, db.SeedNavigationLocationParams{Location: "footer", MenuID: "default-footer-navigation"}); err != nil {
		return fmt.Errorf("seed footer location: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}
	return nil
}
