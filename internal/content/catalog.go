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
	Name         string
	PluralName   string
	Hierarchical bool
	Public       bool
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
		input.Config.SchemaVersion = 1
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
		input.Config.SchemaVersion = 1
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
	if !isBuiltin(ContentTypeID(row.ID)) {
		definition.Capabilities.HasExcerpt = config.Features.Excerpt
		definition.Capabilities.HasFeatured = config.Features.FeaturedMedia
		definition.Capabilities.HasSEO = config.Features.SEO
		definition.Capabilities.HasArchive = config.Routing.Archive
	}
	definition.Routing.BasePath = config.Routing.BasePath
	definition.Routing.Archive = definition.Capabilities.HasArchive
	if definition.Routing.Archive {
		definition.Routing.ArchiveContentType = definition.ID
	}
	return definition, nil
}

func validateContentTypeInput(input ContentTypeInput, updating bool) error {
	input.Name, input.PluralName = strings.TrimSpace(input.Name), strings.TrimSpace(input.PluralName)
	if input.Name == "" || input.PluralName == "" || len(input.Name) > 100 || len(input.PluralName) > 100 {
		return fmt.Errorf("content type names are required and must be at most 100 characters")
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
	if !isBuiltin(input.ID) && input.Public && input.Config.Routing.BasePath == "" {
		return fmt.Errorf("public custom content types require a URL base")
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
