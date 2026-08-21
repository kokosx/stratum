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
)

const seedHomeDocument = `{"version":1,"nodes":[{"id":"welcome-heading","block":"core/heading","version":1,"props":{"text":"Welcome to StratumCMS","level":1}},{"id":"welcome-text","block":"core/text","version":1,"props":{"text":"Your site is running from a structured document, a published revision, and a route."}}]}`

// Seed adds a small, idempotent development site without replacing existing content.
func (d *Database) Seed(ctx context.Context) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer tx.Rollback()

	queries := db.New(tx)
	now := time.Now().Unix()

	document := func(title, text string) string {
		return fmt.Sprintf(`{"version":1,"nodes":[{"id":"heading","block":"core/heading","version":1,"props":{"text":%q,"level":1}},{"id":"text","block":"core/text","version":1,"props":{"text":%q}}]}`, title, text)
	}

	for _, entry := range []struct {
		id, revisionID, routeID, contentType, slug, path, title, excerpt, document string
	}{
		{seedHomeEntryID, seedHomeRevisionID, "seed-home-route", "page", "home", "/", "Welcome to StratumCMS", "A seeded development homepage.", seedHomeDocument},
		{"seed-about", "seed-about-r1", "seed-about-route", "page", "about", "/about", "About Stratum", "Learn about this example site.", document("About Stratum", "StratumCMS is a focused, self-hosted CMS built around structured content.")},
		{"seed-contact", "seed-contact-r1", "seed-contact-route", "page", "contact", "/contact", "Contact", "How to get in touch.", document("Contact", "This example contact page is ready for your own content.")},
		{"seed-post-launch", "seed-post-launch-r1", "seed-post-launch-route", "post", "introducing-stratum", "/blog/introducing-stratum", "Introducing StratumCMS", "A first look at the project.", document("Introducing StratumCMS", "A modern CMS with familiar publishing workflows and a simple deployment model.")},
		{"seed-post-blocks", "seed-post-blocks-r1", "seed-post-blocks-route", "post", "content-blocks", "/blog/content-blocks", "Content Blocks, Not HTML", "Why documents are stored as structured data.", document("Content Blocks, Not HTML", "Content remains independent from presentation, while themes control the final markup.")},
		{"seed-post-roadmap", "seed-post-roadmap-r1", "seed-post-roadmap-route", "post", "building-the-basics", "/blog/building-the-basics", "Building the Basics", "The next steps for this demo.", document("Building the Basics", "Pages, posts, revisions, routes, and rendering form the first useful vertical slice.")},
	} {
		if err := queries.SeedEntry(ctx, db.SeedEntryParams{
			ID: entry.id, ContentTypeID: entry.contentType, Slug: entry.slug,
			CreatedAt: now, UpdatedAt: now, PublishedAt: sql.NullInt64{Int64: now, Valid: true},
		}); err != nil {
			return fmt.Errorf("seed %s entry: %w", entry.slug, err)
		}
		if err := queries.SeedEntryRevision(ctx, db.SeedEntryRevisionParams{
			ID: entry.revisionID, EntryID: entry.id, Title: entry.title,
			Excerpt: sql.NullString{String: entry.excerpt, Valid: true}, DocumentJson: entry.document,
			SeoTitle: sql.NullString{String: entry.title, Valid: true}, SeoDescription: sql.NullString{String: entry.excerpt, Valid: true}, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed %s revision: %w", entry.slug, err)
		}
		if err := queries.SeedPublishedRevision(ctx, db.SeedPublishedRevisionParams{
			PublishedRevisionID: sql.NullString{String: entry.revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entry.id,
		}); err != nil {
			return fmt.Errorf("publish seeded %s revision: %w", entry.slug, err)
		}
		if err := queries.SeedRoute(ctx, db.SeedRouteParams{
			ID: entry.routeID, Path: entry.path, EntryID: sql.NullString{String: entry.id, Valid: true}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed %s route: %w", entry.slug, err)
		}
	}
	if err := queries.SeedSiteSettings(ctx, db.SeedSiteSettingsParams{HomepageEntryID: sql.NullString{String: seedHomeEntryID, Valid: true}, UpdatedAt: now}); err != nil {
		return fmt.Errorf("seed site settings: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}
	return nil
}
