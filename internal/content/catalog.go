package content

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Catalog is the database-backed source of effective content type definitions.
// It has no mutable process cache: a committed change is visible to the next read.
type Catalog struct{ queries *db.Queries }

func NewCatalog(queries *db.Queries) *Catalog { return &Catalog{queries: queries} }

type ContentTypeInput struct {
	ID           ContentTypeID
	Name         string // ItemLabel (optional singular)
	PluralName   string // Label (required plural/neutral)
	Hierarchical bool
	Public       bool // deprecated; derived from Config.Routing.Single for custom types
	Config       ContentTypeConfig
}

func (c *Catalog) GetDefinition(ctx context.Context, id string) (ContentTypeDefinition, error) {
	row, err := c.queries.GetContentType(ctx, id)
	if err != nil {
		return ContentTypeDefinition{}, err
	}
	return definitionFromRow(row)
}

func (c *Catalog) ListDefinitions(ctx context.Context) ([]ContentTypeDefinition, error) {
	rows, err := c.queries.ListContentTypes(ctx)
	if err != nil {
		return nil, err
	}
	definitions := make([]ContentTypeDefinition, 0, len(rows))
	for _, row := range rows {
		definition, err := definitionFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("content type %s: %w", row.ID, err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func (c *Catalog) CreateContentType(ctx context.Context, input ContentTypeInput) error {
	if input.Config.SchemaVersion == 0 {
		input.Config.SchemaVersion = 2
	}
	// Derive Public column from Single for backward compat (sitemap, old queries)
	if !isBuiltin(input.ID) {
		input.Public = input.Config.Routing.Single
	}
	if err := validateContentTypeInput(input, false); err != nil {
		return err
	}
	if _, err := c.queries.GetContentType(ctx, string(input.ID)); err == nil {
		return fmt.Errorf("content type key %q already exists", input.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if input.Config.Routing.BasePath != "" {
		if err := c.ensureBasePathUnique(ctx, "", input.Config.Routing.BasePath); err != nil {
			return err
		}
	}
	encoded, err := EncodeContentTypeConfig(input.Config)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	return c.queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: string(input.ID), DisplayName: input.Name, PluralName: input.PluralName, Hierarchical: boolInt(input.Hierarchical), Public: boolInt(input.Public), ConfigJson: encoded, CreatedAt: now, UpdatedAt: now})
}

// UpdateContentType keeps identity stable. Existing field keys and field types
// are immutable because historical revisions and SDT references use them.
func (c *Catalog) UpdateContentType(ctx context.Context, input ContentTypeInput) error {
	if input.Config.SchemaVersion == 0 {
		input.Config.SchemaVersion = 2
	}
	if !isBuiltin(input.ID) {
		input.Public = input.Config.Routing.Single
	}
	if err := validateContentTypeInput(input, true); err != nil {
		return err
	}
	previous, err := c.GetDefinition(ctx, string(input.ID))
	if err != nil {
		return err
	}
	if isBuiltin(input.ID) {
		input.Hierarchical = previous.Capabilities.Hierarchical
		input.Public = previous.Capabilities.Public
		// Preserve core-owned routing: Page/Post base and archive are code-owned
		input.Config.Routing.Single = previous.Routing.Single
		input.Config.Routing.Archive = previous.Routing.Archive
		input.Config.Routing.BasePath = previous.Routing.BasePath
	}
	if err := validateFieldEvolution(previous.Fields, input.Config.Fields); err != nil {
		return err
	}
	if input.Config.Routing.BasePath != "" && input.Config.Routing.BasePath != previous.Routing.BasePath {
		if err := c.ensureBasePathUnique(ctx, string(input.ID), input.Config.Routing.BasePath); err != nil {
			return err
		}
	}
	if input.Config.SchemaVersion <= previous.SchemaVersion {
		input.Config.SchemaVersion = previous.SchemaVersion + 1
	}
	encoded, err := EncodeContentTypeConfig(input.Config)
	if err != nil {
		return err
	}
	return c.queries.UpdateContentType(ctx, db.UpdateContentTypeParams{ID: string(input.ID), DisplayName: input.Name, PluralName: input.PluralName, Hierarchical: boolInt(input.Hierarchical), Public: boolInt(input.Public), ConfigJson: encoded, UpdatedAt: time.Now().Unix()})
}

func (c *Catalog) ensureBasePathUnique(ctx context.Context, selfID, basePath string) error {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		return nil
	}
	definitions, err := c.ListDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		if string(definition.ID) == selfID {
			continue
		}
		if definition.Routing.BasePath == basePath {
			return fmt.Errorf("URL base %q is already used by content type %q", basePath, definition.ID)
		}
	}
	return nil
}

// DeleteContentType removes a custom content type only when it has no
// dependent data. Built-in types, types with entries, layout templates or
// taxonomies are protected. The caller should run this inside a transaction
// that also cleans up the archive route.
func (c *Catalog) DeleteContentType(ctx context.Context, id string) error {
	if isBuiltin(ContentTypeID(id)) {
		return fmt.Errorf("cannot delete built-in content type %q", id)
	}
	if _, err := c.queries.GetContentType(ctx, id); err != nil {
		return err
	}
	if entries, err := c.queries.ListEntriesByContentType(ctx, id); err == nil && len(entries) > 0 {
		return fmt.Errorf("cannot delete %q because it contains %d entries", id, len(entries))
	}
	if templates, err := c.queries.ListLayoutTemplatesByContentType(ctx, id); err == nil && len(templates) > 0 {
		return fmt.Errorf("cannot delete %q because it has %d layout templates", id, len(templates))
	}
	if taxonomies, err := c.queries.ListTaxonomiesByContentType(ctx, id); err == nil && len(taxonomies) > 0 {
		return fmt.Errorf("cannot delete %q because it has %d taxonomies", id, len(taxonomies))
	}
	return c.queries.DeleteContentType(ctx, id)
}

func definitionFromRow(row db.ContentType) (ContentTypeDefinition, error) {
	config, err := DecodeContentTypeConfig(row.ConfigJson)
	if err != nil {
		return ContentTypeDefinition{}, err
	}
	definition := DefinitionFor(row.ID)
	definition.ID, definition.Name, definition.PluralName = ContentTypeID(row.ID), row.DisplayName, row.PluralName
	definition.Fields, definition.SchemaVersion = config.Fields, config.SchemaVersion
	definition.Capabilities.Hierarchical, definition.Capabilities.Public = row.Hierarchical == 1, row.Public == 1
	// Backward compat: SchemaVersion 1 had no Single/HasContent; infer from old state
	isCustom := !isBuiltin(ContentTypeID(row.ID))
	if isCustom {
		// Normalize config defaults for v1 -> v2 migration
		if config.SchemaVersion == 1 {
			// v1 had Public column as source of truth for routing
			// Map: Single = Public (unless config already has Single explicit weirdly)
			// HasContent defaults to true for backward compat (preserve editor)
			if !config.Routing.Single && row.Public == 1 && config.Routing.BasePath != "" {
				// Historical public type without explicit Single: infer Single true
				config.Routing.Single = true
			} else if config.Routing.Single && row.Public == 0 {
				// Config says Single but DB says private – honor config? Use config
			}
			// Archive already in config.Routing.Archive
			// HasContent: old types implicitly had rich content
			if !config.Features.Content {
				// Distinguish: if SchemaVersion 1 and Features.Content false, it was default
				// We preserve true as effective default unless explicitly stored false in v2
				// Since v1 never stored Content, treat as true
				definition.Capabilities.HasContent = true
			} else {
				definition.Capabilities.HasContent = config.Features.Content
			}
		} else {
			definition.Capabilities.HasContent = config.Features.Content
		}
		definition.Capabilities.HasExcerpt = config.Features.Excerpt
		definition.Capabilities.HasFeatured = config.Features.FeaturedMedia
		definition.Capabilities.HasSEO = config.Features.SEO
		definition.Capabilities.HasArchive = config.Routing.Archive
		definition.Capabilities.Single = config.Routing.Single
		// Normalize routing Single from config, but fallback to row.Public for truly old empty configs
		if config.SchemaVersion == 1 && config.Routing.BasePath == "" && !config.Routing.Archive && !config.Routing.Single {
			// Empty/legacy config: if row.Public ==1 but no base/archive, it was likely a public type that lost base? Keep Single = public
			if row.Public == 1 {
				// But without base, Single true would be invalid per new validation; so only infer true if we can
				// Keep Single false to allow BasePath empty; effective Single will be false
				// For compatibility, treat as Single = public && basePath != "" or archive? Actually historical private types remain Single false
				definition.Capabilities.Single = false
			}
		}
		// If config explicitly has Single true, honor it even if row.Public mismatched (config is source of truth post-migration)
		if config.SchemaVersion >= 2 {
			definition.Capabilities.Single = config.Routing.Single
		} else if row.Public == 1 && config.Routing.Single {
			definition.Capabilities.Single = true
		} else if row.Public == 1 && !config.Routing.Single && config.Routing.BasePath != "" {
			// Legacy v1 public with base but Single false due to zero value – fix
			definition.Capabilities.Single = true
		}
	} else {
		// Built-ins: already set via KnownDefinitions but ensure sync
		definition.Capabilities.Single = definition.Routing.Single
		definition.Capabilities.HasContent = true
	}
	definition.Routing.BasePath = config.Routing.BasePath
	definition.Routing.Single = definition.Capabilities.Single
	definition.Routing.Archive = definition.Capabilities.HasArchive
	if definition.Routing.Archive {
		definition.Routing.ArchiveContentType = definition.ID
	}
	// Ensure Public mirrors Single for custom types (sitemap backward compat)
	if isCustom {
		definition.Capabilities.Public = definition.Capabilities.Single
	}
	return definition, nil
}

func validateContentTypeInput(input ContentTypeInput, updating bool) error {
	input.Name, input.PluralName = strings.TrimSpace(input.Name), strings.TrimSpace(input.PluralName)
	// New model: Label (PluralName) required, ItemLabel (Name) optional
	if input.PluralName == "" || len(input.PluralName) > 100 || len(input.Name) > 100 {
		return fmt.Errorf("content type label is required and must be at most 100 characters")
	}
	if input.Name != "" && strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("content type item label must not be empty if provided")
	}
	if !contentTypeKeyPattern.MatchString(string(input.ID)) {
		return fmt.Errorf("invalid content type key %q", input.ID)
	}
	if !updating && isBuiltin(input.ID) {
		return fmt.Errorf("content type key %q is reserved", input.ID)
	}
	if !updating {
		if reservedContentTypeKeys[input.ID] {
			return fmt.Errorf("content type key %q is reserved", input.ID)
		}
	}
	// Routing validation: BasePath required when Single or Archive enabled
	if !isBuiltin(input.ID) {
		if (input.Config.Routing.Single || input.Config.Routing.Archive) && input.Config.Routing.BasePath == "" {
			return fmt.Errorf("URL base is required when single or archive routing is enabled")
		}
	}
	return ValidateContentTypeConfig(input.Config)
}

var reservedContentTypeKeys = map[ContentTypeID]bool{"core": true, "admin": true, "system": true, "media": true, "search": true, "taxonomy": true, "layout": true}

func validateFieldEvolution(previous, next []FieldDefinition) error {
	old := make(map[string]FieldDefinition, len(previous))
	for _, field := range previous {
		old[field.Key] = field
	}
	for _, field := range next {
		if prior, ok := old[field.Key]; ok && prior.Type != field.Type {
			return fmt.Errorf("field %q type is immutable", field.Key)
		}
	}
	return nil
}

func isBuiltin(id ContentTypeID) bool { return id == ContentTypePage || id == ContentTypePost }
func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
