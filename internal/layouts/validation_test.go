package layouts

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type fakeStore struct {
	defs []db.BlockDefinition
}

func (f *fakeStore) ListBlockDefinitions(ctx context.Context) ([]db.BlockDefinition, error) {
	return f.defs, nil
}

func testRegistry(t *testing.T, defs []db.BlockDefinition) *blocks.Registry {
	t.Helper()
	store := &fakeStore{defs: defs}
	reg, err := blocks.NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func mustRegistryWithSlot(t *testing.T) *blocks.Registry {
	t.Helper()
	// include slot + basic blocks
	headingSchema := `{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""},"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":2}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"heading"}}`
	sectionSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"width":{"type":"string","enum":["content","wide"],"default":"content"}}},"children":{"mode":"any"},"editor":{"category":"layout","icon":"section"}}`
	slotSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"layout","icon":"content-slot"}}`
	textSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{"text":{"type":"string","default":""}}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"text","icon":"text"}}`
	defs := []db.BlockDefinition{
		{Namespace: "core", Name: "heading", Version: 1, DisplayName: "Heading", SchemaJson: headingSchema, RendererType: "template", Template: sql.NullString{String: `<h{{ .Props.level }}>{{ .Props.text }}</h{{ .Props.level }}>`, Valid: true}, Source: "core", Enabled: 1},
		{Namespace: "core", Name: "section", Version: 1, DisplayName: "Section", SchemaJson: sectionSchema, RendererType: "template", Template: sql.NullString{String: `<section>{{ .Children }}</section>`, Valid: true}, Source: "core", Enabled: 1},
		{Namespace: "core", Name: "content-slot", Version: 1, DisplayName: "Content", SchemaJson: slotSchema, RendererType: "template", Template: sql.NullString{String: `<div>slot</div>`, Valid: true}, Source: "core", Enabled: 1},
		{Namespace: "core", Name: "text", Version: 1, DisplayName: "Text", SchemaJson: textSchema, RendererType: "template", Template: sql.NullString{String: `<p>{{ .Props.text }}</p>`, Valid: true}, Source: "core", Enabled: 1},
	}
	return testRegistry(t, defs)
}

func docFrom(t *testing.T, s string) *document.Document {
	t.Helper()
	var d document.Document
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("doc: %v", err)
	}
	return &d
}

func TestValidateLayoutTemplate_Valid(t *testing.T) {
	reg := mustRegistryWithSlot(t)
	d := docFrom(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{"width":"content"},"children":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`)
	if err := ValidateLayoutTemplateDocument(reg, d); err != nil {
		t.Fatalf("unexpected %v", err)
	}
}

func TestValidateLayoutTemplate_Zero(t *testing.T) {
	reg := mustRegistryWithSlot(t)
	d := docFrom(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{"width":"content"},"children":[{"id":"h","block":"core/heading","version":1,"props":{"text":"hi","level":1},"settings":{}}]}]}`)
	// EPIC 2: zero slot is allowed for Single (fields-only)
	if err := ValidateLayoutTemplateDocument(reg, d); err != nil {
		t.Fatalf("unexpected error for zero slot: %v", err)
	}
	// Archive must have zero, single with slot also valid
	d2 := docFrom(t, `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	if err := ValidateTemplateDocument(reg, d2, "archive", nil); err == nil {
		t.Fatal("expected error for archive with slot")
	}
}

func TestValidateLayoutTemplate_TwoSlots(t *testing.T) {
	reg := mustRegistryWithSlot(t)
	d := docFrom(t, `{"version":1,"nodes":[{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}},{"id":"slot2","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	if err := ValidateLayoutTemplateDocument(reg, d); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateEntry_WithSlotInvalid(t *testing.T) {
	reg := mustRegistryWithSlot(t)
	d := docFrom(t, `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	if err := ValidateEntryDocument(reg, d); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateEntry_WithoutSlotValid(t *testing.T) {
	reg := mustRegistryWithSlot(t)
	d := docFrom(t, `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"hi","level":1},"settings":{}}]}`)
	if err := ValidateEntryDocument(reg, d); err != nil {
		t.Fatalf("unexpected %v", err)
	}
}

func TestCatalogFiltering(t *testing.T) {
	reg := mustRegistryWithSlot(t)
	entryCat := reg.EditorCatalogFor(blocks.EditorModeEntry)
	for _, d := range entryCat {
		if d.Block == "core/content-slot" {
			t.Fatal("entry catalog should not contain slot")
		}
	}
	layoutCat := reg.EditorCatalogFor(blocks.EditorModeLayoutTemplate)
	found := false
	for _, d := range layoutCat {
		if d.Block == "core/content-slot" {
			found = true
		}
	}
	if !found {
		t.Fatal("layout catalog should contain slot")
	}
}
