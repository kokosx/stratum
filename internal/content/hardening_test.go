package content

import (
	"context"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSchemaVersionOnlyBumpsOnFieldAddRemove(t *testing.T) {
	_, _, queries := newTestRepository(t)
	ctx := context.Background()
	cat := NewCatalog(queries)
	// Create product with one field
	input := ContentTypeInput{
		ID:         "product",
		PluralName: "Products",
		Config: ContentTypeConfig{
			Fields:   []FieldDefinition{{Key: "price", Label: "Price", Type: FieldNumber}},
			Features: ContentTypeFeatures{Content: true},
			Routing:  ContentTypeRouting{Single: true, BasePath: "/products"},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	def, _ := cat.GetDefinition(ctx, "product")
	v1 := def.SchemaVersion
	// Update label only (PluralName) – should NOT bump
	input.PluralName = "Produkty"
	// Need to fetch full definition to get fields
	def2, _ := cat.GetDefinition(ctx, "product")
	input.Config.Fields = def2.Fields
	// Keep same routing (explicit mapping)
	input.Config.Routing = ContentTypeRouting{Single: def2.Routing.Single, BasePath: def2.Routing.BasePath, Archive: def2.Routing.Archive}
	input.Config.Features = ContentTypeFeatures{Content: true}
	if err := cat.UpdateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	defAfterLabel, _ := cat.GetDefinition(ctx, "product")
	if defAfterLabel.SchemaVersion != v1 {
		t.Fatalf("label change should not bump schema version: got %d want %d", defAfterLabel.SchemaVersion, v1)
	}
	// Change base path – should NOT bump
	input.Config.Routing.BasePath = "/produkty"
	if err := cat.UpdateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	defAfterBase, _ := cat.GetDefinition(ctx, "product")
	if defAfterBase.SchemaVersion != v1 {
		t.Fatalf("routing change should not bump schema version: got %d want %d", defAfterBase.SchemaVersion, v1)
	}
	// Add field – SHOULD bump
	input.Config.Fields = append(defAfterBase.Fields, FieldDefinition{Key: "sku", Label: "SKU", Type: FieldText})
	input.Config.Routing.BasePath = "/products" // revert
	if err := cat.UpdateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	defAfterAdd, _ := cat.GetDefinition(ctx, "product")
	if defAfterAdd.SchemaVersion != v1+1 {
		t.Fatalf("field add should bump schema version: got %d want %d", defAfterAdd.SchemaVersion, v1+1)
	}
	// Remove field – SHOULD bump again
	input.Config.Fields = []FieldDefinition{{Key: "price", Label: "Price", Type: FieldNumber}}
	if err := cat.UpdateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	defAfterRemove, _ := cat.GetDefinition(ctx, "product")
	if defAfterRemove.SchemaVersion != v1+2 {
		t.Fatalf("field remove should bump again: got %d want %d", defAfterRemove.SchemaVersion, v1+2)
	}
}

func TestNormalizeV1PublicAndPrivate(t *testing.T) {
	_, _, queries := newTestRepository(t)
	ctx := context.Background()
	// Simulate v1 row: config json with schemaVersion 1, empty routing, public column =1
	// Insert directly via queries
	rawV1Public := `{"schemaVersion":1,"fields":[]}`
	// Create content type row manually to simulate legacy v1
	// Use raw SQL via queries? Use CreateContentType with v1 config but public=1
	// Instead create via direct DB insert
	// Use queries.CreateContentType with config json v1 and public 1
	if err := queries.CreateContentType(ctx, db.CreateContentTypeParams{
		ID: "legacy_public", DisplayName: "Legacy", PluralName: "Legacies", Hierarchical: 0, Public: 1, ConfigJson: rawV1Public, CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	def, err := NewCatalog(queries).GetDefinition(ctx, "legacy_public")
	if err != nil {
		t.Fatal(err)
	}
	if !def.Routing.Single {
		t.Fatalf("v1 public should normalize to Single=true, got %v", def.Routing.Single)
	}
	if !def.Capabilities.HasContent {
		t.Fatalf("v1 should have HasContent true")
	}
	// v1 private
	rawV1Private := `{"schemaVersion":1}`
	if err := queries.CreateContentType(ctx, db.CreateContentTypeParams{
		ID: "legacy_private", DisplayName: "Private", PluralName: "Privates", Hierarchical: 0, Public: 0, ConfigJson: rawV1Private, CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	def2, err := NewCatalog(queries).GetDefinition(ctx, "legacy_private")
	if err != nil {
		t.Fatal(err)
	}
	if def2.Routing.Single {
		t.Fatalf("v1 private should be Single=false, got true")
	}
	// v2 explicit
	rawV2 := `{"schemaVersion":2,"fields":[],"features":{"content":false},"routing":{"single":false,"archive":false}}`
	if err := queries.CreateContentType(ctx, db.CreateContentTypeParams{
		ID: "v2_test", DisplayName: "V2", PluralName: "V2s", Hierarchical: 0, Public: 0, ConfigJson: rawV2, CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	def3, _ := NewCatalog(queries).GetDefinition(ctx, "v2_test")
	if def3.Routing.Single {
		t.Fatalf("v2 explicit false should be false")
	}
	if def3.Capabilities.HasContent {
		t.Fatalf("v2 HasContent false should be false")
	}
}

func TestFieldCatalogPermalink(t *testing.T) {
	defSingle := ContentTypeDefinition{
		ID: "test", Routing: RoutingPolicy{Single: true}, Capabilities: Capabilities{HasContent: true},
	}
	opts := FieldCatalog(defSingle)
	found := false
	for _, o := range opts {
		if o.Value == "entry.permalink" {
			found = true
		}
	}
	if !found {
		t.Fatalf("single should have permalink")
	}
	defNoSingle := ContentTypeDefinition{
		ID: "test2", Routing: RoutingPolicy{Single: false}, Capabilities: Capabilities{HasContent: true},
	}
	opts = FieldCatalog(defNoSingle)
	for _, o := range opts {
		if o.Value == "entry.permalink" {
			t.Fatalf("route-less should not have permalink, got %v", opts)
		}
	}
}

func TestHasSingleAndIsArchived(t *testing.T) {
	d := ContentTypeDefinition{Routing: RoutingPolicy{Single: true, Archive: false}}
	if !d.HasSingle() {
		t.Fatalf("HasSingle true")
	}
	if d.IsArchived() {
		t.Fatalf("IsArchived false")
	}
	d2 := ContentTypeDefinition{Routing: RoutingPolicy{Single: false, Archive: true}}
	if d2.HasSingle() {
		t.Fatalf("HasSingle false")
	}
	if !d2.IsArchived() {
		t.Fatalf("IsArchived true")
	}
	// Ensure Capabilities.Single no longer influences
	d3 := ContentTypeDefinition{Capabilities: Capabilities{Public: true}, Routing: RoutingPolicy{Single: false}}
	if d3.HasSingle() {
		t.Fatalf("HasSingle should be false when Routing.Single false regardless of Capabilities.Public")
	}
}

func TestFallbackArchiveTitle(t *testing.T) {
	d := ContentTypeDefinition{PluralName: "Products", Name: "Product", ID: "product"}
	if got := FallbackArchiveTitle(d); got != "Products" {
		t.Fatalf("fallback should be Products, got %q", got)
	}
	if !strings.Contains(FallbackArchiveTitle(d), "Products") {
		t.Fatalf("unexpected")
	}
}
