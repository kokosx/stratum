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
	Public       bool // LEGACY STORAGE COMPATIBILITY ONLY. DO NOT USE FOR CONTENT TYPE ROUTING DECISIONS. For custom types, derived from Config.Routing.Single (persistence adapter: public = single)
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
	// LEGACY STORAGE COMPATIBILITY ONLY. DO NOT USE FOR ROUTING DECISIONS.
	if !isBuiltin(input.ID) {
		input.Public = input.Config.Routing.Single
	}
	if err := ValidateContentTypeInput(input, false); err != nil {
		return err
	}
	if _, err := c.queries.GetContentType(ctx, string(input.ID)); err == nil {
		return fmt.Errorf("content type key %q already exists", input.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if input.Config.Routing.BasePath != "" {
		if err := c.EnsureBasePathUnique(ctx, "", input.Config.Routing.BasePath); err != nil {
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
		// LEGACY STORAGE COMPATIBILITY ONLY
		input.Public = input.Config.Routing.Single
	}
	if err := ValidateContentTypeInput(input, true); err != nil {
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
	if err := ValidateFieldEvolution(previous.Fields, input.Config.Fields); err != nil {
		return err
	}
	if input.Config.Routing.BasePath != "" && input.Config.Routing.BasePath != previous.Routing.BasePath {
		if err := c.EnsureBasePathUnique(ctx, string(input.ID), input.Config.Routing.BasePath); err != nil {
			return err
		}
	}
	// SchemaVersion semantics: only bump when content schema meaningfully changes (field add/remove).
	// Changing label, routing, or admin presentation must not bump.
	if SchemaChanged(previous.Fields, input.Config.Fields) {
		if input.Config.SchemaVersion <= previous.SchemaVersion {
			input.Config.SchemaVersion = previous.SchemaVersion + 1
		}
	} else {
		input.Config.SchemaVersion = previous.SchemaVersion
	}
	encoded, err := EncodeContentTypeConfig(input.Config)
	if err != nil {
		return err
	}
	return c.queries.UpdateContentType(ctx, db.UpdateContentTypeParams{ID: string(input.ID), DisplayName: input.Name, PluralName: input.PluralName, Hierarchical: boolInt(input.Hierarchical), Public: boolInt(input.Public), ConfigJson: encoded, UpdatedAt: time.Now().Unix()})
}

func (c *Catalog) EnsureBasePathUnique(ctx context.Context, selfID, basePath string) error {
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

func (c *Catalog) ensureBasePathUnique(ctx context.Context, selfID, basePath string) error {
	return c.EnsureBasePathUnique(ctx, selfID, basePath)
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

// definitionFromRow is the single pipeline: raw config → decode → normalize legacy defaults → build effective definition.
// No DB write is required just to read old config. Normalization is deterministic.
func definitionFromRow(row db.ContentType) (ContentTypeDefinition, error) {
	config, err := DecodeContentTypeConfig(row.ConfigJson)
	if err != nil {
		return ContentTypeDefinition{}, err
	}
	// Normalize legacy semantics to current effective semantics
	config = normalizeContentTypeConfig(row, config)

	definition := DefinitionFor(row.ID)
	definition.ID, definition.Name, definition.PluralName = ContentTypeID(row.ID), row.DisplayName, row.PluralName
	definition.Fields, definition.SchemaVersion = config.Fields, config.SchemaVersion

	isCustom := !isBuiltin(ContentTypeID(row.ID))
	if isCustom {
		definition.Capabilities.Hierarchical = row.Hierarchical == 1
		definition.Capabilities.HasContent = config.Features.Content
		definition.Capabilities.HasExcerpt = config.Features.Excerpt
		definition.Capabilities.HasFeatured = config.Features.FeaturedMedia
		definition.Capabilities.HasSEO = config.Features.SEO
		// LEGACY STORAGE COMPATIBILITY ONLY: Public mirrors Single for custom types
		definition.Capabilities.Public = config.Routing.Single
		definition.Routing.Single = config.Routing.Single
		definition.Routing.Archive = config.Routing.Archive
		definition.Routing.BasePath = config.Routing.BasePath
		if definition.Routing.Archive {
			definition.Routing.ArchiveContentType = definition.ID
		}
	} else {
		// Built-ins: core-owned policies remain from KnownDefinitions; only fields/schema/version and labels are DB-driven.
		// Preserve code-owned routing and capabilities; do not let DB config override Single/Archive/Base.
		definition.Fields = config.Fields
		definition.SchemaVersion = config.SchemaVersion
		// Hierarchical and Public remain as per KnownDefinitions; row values are ignored for builtins to keep single source.
		// However keep DB display names already set above.
		definition.Capabilities.Public = DefinitionFor(row.ID).Capabilities.Public
		// HasContent always true for builtins
		definition.Capabilities.HasContent = true
		// Routing already correct from DefinitionFor
		if definition.Routing.Archive {
			definition.Routing.ArchiveContentType = definition.ID
		}
	}
	// Ensure Public mirrors Single for custom types (sitemap backward compat persistence adapter)
	if isCustom {
		definition.Capabilities.Public = definition.Routing.Single
	}
	return definition, nil
}

// normalizeContentTypeConfig decodes raw persisted config and normalizes legacy semantics to current effective semantics.
// Desired pipeline: raw config → decode → normalize legacy defaults → build effective definition
func normalizeContentTypeConfig(row db.ContentType, config ContentTypeConfig) ContentTypeConfig {
	isCustom := !isBuiltin(ContentTypeID(row.ID))
	if isCustom && config.SchemaVersion == 1 {
		// SchemaVersion 1 had no explicit Single/HasContent; infer from old state.
		// v1 had Public column as source of truth for routing; HasContent defaulted to true.
		// Normalize: Single = Public (row.Public == 1), HasContent = true
		// v1 public + base → Single=true, HasContent=true
		// v1 private → Single=false, preserve historical intended route-less semantics
		config.Routing.Single = row.Public == 1
		config.Features.Content = true
		// Archive and BasePath remain as decoded (v1 may have stored archive via config.Routing.Archive)
		// Keep SchemaVersion as 1 for storage; effective definition uses normalized values without mutating DB.
	}
	return config
}

// ValidateContentTypeInput is the single implementation of content type input validation.
// Exposed for reuse by contenttypes.Service to avoid duplication.
func ValidateContentTypeInput(input ContentTypeInput, updating bool) error {
	return validateContentTypeInput(input, updating)
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

// ValidateFieldEvolution is the single implementation of field evolution validation.
func ValidateFieldEvolution(previous, next []FieldDefinition) error {
	return validateFieldEvolution(previous, next)
}

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
