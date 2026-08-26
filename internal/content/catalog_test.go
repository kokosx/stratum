package content

import (
	"context"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestContentTypeConfigRoundTripAndCatalogDefinition(t *testing.T) {
	_, _, queries := newTestRepository(t)
	ctx := context.Background()
	catalog := NewCatalog(queries)
	input := ContentTypeInput{ID: "product", Name: "Product", PluralName: "Products", Public: true, Config: ContentTypeConfig{Fields: []FieldDefinition{{Key: "price", Label: "Price", Type: FieldNumber}}, Features: ContentTypeFeatures{FeaturedMedia: true, SEO: true}, Routing: ContentTypeRouting{BasePath: "/products", Archive: true}}}
	if err := catalog.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	definition, err := catalog.GetDefinition(ctx, "product")
	if err != nil {
		t.Fatal(err)
	}
	if definition.Routing.BasePath != "/products" || !definition.IsArchived() || !definition.Capabilities.HasFeatured || len(definition.Fields) != 1 || definition.Fields[0].Key != "price" {
		t.Fatalf("unexpected effective definition: %#v", definition)
	}
	row, err := queries.GetContentType(ctx, "product")
	if err != nil {
		t.Fatal(err)
	}
	config, err := DecodeContentTypeConfig(row.ConfigJson)
	if err != nil || config.Fields[0].Type != FieldNumber {
		t.Fatalf("config round trip: %#v, %v", config, err)
	}
}

func TestCatalogMergesBuiltinFieldsAndProtectsFieldType(t *testing.T) {
	_, _, queries := newTestRepository(t)
	ctx := context.Background()
	catalog := NewCatalog(queries)
	input := ContentTypeInput{ID: ContentTypePage, Name: "Pages", PluralName: "Pages", Config: ContentTypeConfig{Fields: []FieldDefinition{{Key: "subtitle", Label: "Subtitle", Type: FieldText}}}}
	if err := catalog.UpdateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	definition, err := catalog.GetDefinition(ctx, "page")
	if err != nil {
		t.Fatal(err)
	}
	if !definition.Capabilities.Hierarchical || !definition.Capabilities.Public || definition.Fields[0].Key != "subtitle" {
		t.Fatalf("builtin merge lost core capabilities: %#v", definition)
	}
	input.Config.Fields[0].Type = FieldNumber
	if err := catalog.UpdateContentType(ctx, input); err == nil {
		t.Fatal("changing a field type must fail")
	}
}

func TestCatalogRejectsInvalidAndReservedKeys(t *testing.T) {
	_, _, queries := newTestRepository(t)
	catalog := NewCatalog(queries)
	for _, key := range []ContentTypeID{"Product", "page", "_product"} {
		if err := catalog.CreateContentType(context.Background(), ContentTypeInput{ID: key, Name: "Product", PluralName: "Products", Config: ContentTypeConfig{}}); err == nil {
			t.Fatalf("key %q unexpectedly accepted", key)
		}
	}
}

func TestFieldRefResolver(t *testing.T) {
	ref, err := ParseFieldRef("fields.price")
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := ResolveEntryField(ref, "", "", "", "", "", map[string]any{"price": 99.0}); !ok || value != 99.0 {
		t.Fatalf("custom value = %#v, %v", value, ok)
	}
	if _, err := ParseFieldRef("fields.bad-key"); err == nil {
		t.Fatal("invalid ref accepted")
	}
	if ref, err = ParseFieldRef("entry.title"); err != nil {
		t.Fatal(err)
	}
	if value, ok := ResolveEntryField(ref, "MacBook", "", "", "", "", nil); !ok || value != "MacBook" {
		t.Fatalf("title = %#v, %v", value, ok)
	}
}

func TestCatalogDeleteBlocksWithDependencies(t *testing.T) {
	_, _, queries := newTestRepository(t)
	ctx := context.Background()
	catalog := NewCatalog(queries)
	// create deletable
	input := ContentTypeInput{ID: "deletable", Name: "Deletable", PluralName: "Deletables", Public: true, Config: ContentTypeConfig{Routing: ContentTypeRouting{BasePath: "/deletable", Archive: true}}}
	if err := catalog.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	// delete should succeed when no dependencies
	if err := catalog.DeleteContentType(ctx, "deletable"); err != nil {
		t.Fatalf("delete empty type failed: %v", err)
	}
	if _, err := queries.GetContentType(ctx, "deletable"); err == nil {
		t.Fatal("deletable still exists after delete")
	}
	// create product with entry
	input = ContentTypeInput{ID: "product", Name: "Product", PluralName: "Products", Public: true, Config: ContentTypeConfig{Routing: ContentTypeRouting{BasePath: "/products", Archive: true}}}
	if err := catalog.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	// create an entry for product
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "p1", ContentTypeID: "product", Slug: "p1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.DeleteContentType(ctx, "product"); err == nil || !strings.Contains(err.Error(), "contains") {
		t.Fatalf("delete with entries should be blocked, got %v", err)
	}
	// builtin blocked
	if err := catalog.DeleteContentType(ctx, "page"); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("delete page should be blocked, got %v", err)
	}
}
