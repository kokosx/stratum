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
	seedHomeRouteID    = "seed-home-route"
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

	for _, block := range []struct {
		id, name, displayName, schema, template string
	}{
		{"seed-core-heading-v1", "heading", "Heading", `{"type":"object","required":["text"],"properties":{"text":{"type":"string"},"level":{"type":"integer","minimum":1,"maximum":6}}}`, `<h2>{{ .Props.text }}</h2>`},
		{"seed-core-text-v1", "text", "Text", `{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}`, `<p>{{ .Props.text }}</p>`},
	} {
		if err := queries.SeedBlockDefinition(ctx, db.SeedBlockDefinitionParams{
			ID: block.id, Namespace: "core", Name: block.name, Version: 1,
			DisplayName: block.displayName, SchemaJson: block.schema, Template: sql.NullString{String: block.template, Valid: true}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed %s block: %w", block.name, err)
		}
	}

	if err := queries.SeedEntry(ctx, db.SeedEntryParams{ID: seedHomeEntryID, Slug: "home", CreatedAt: now, UpdatedAt: now, PublishedAt: sql.NullInt64{Int64: now, Valid: true}}); err != nil {
		return fmt.Errorf("seed home entry: %w", err)
	}
	if err := queries.SeedEntryRevision(ctx, db.SeedEntryRevisionParams{
		ID: seedHomeRevisionID, EntryID: seedHomeEntryID, Title: "Welcome to StratumCMS",
		Excerpt: sql.NullString{String: "A seeded development homepage.", Valid: true}, DocumentJson: seedHomeDocument,
		SeoTitle: sql.NullString{String: "Welcome to StratumCMS", Valid: true}, SeoDescription: sql.NullString{String: "A seeded development homepage.", Valid: true}, CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("seed home revision: %w", err)
	}
	if err := queries.SeedPublishedRevision(ctx, db.SeedPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: seedHomeRevisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: seedHomeEntryID}); err != nil {
		return fmt.Errorf("publish seeded home revision: %w", err)
	}
	if err := queries.SeedRoute(ctx, db.SeedRouteParams{ID: seedHomeRouteID, Path: "/", EntryID: sql.NullString{String: seedHomeEntryID, Valid: true}, CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("seed home route: %w", err)
	}
	if err := queries.SeedSiteSettings(ctx, db.SeedSiteSettingsParams{HomepageEntryID: sql.NullString{String: seedHomeEntryID, Valid: true}, UpdatedAt: now}); err != nil {
		return fmt.Errorf("seed site settings: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}
	return nil
}
