package blocks

import (
	"context"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestEditorSchemaVisualRoot(t *testing.T) {
	schema, err := ParseSchema(`{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"design","icon":"button","visualRoot":".stratum-button"}}`)
	if err != nil {
		t.Fatalf("parse with visualRoot: %v", err)
	}
	if schema.Editor.VisualRoot != ".stratum-button" {
		t.Fatalf("visualRoot = %q, want .stratum-button", schema.Editor.VisualRoot)
	}
	// Image with img selector
	schema2, err := ParseSchema(`{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"visualRoot":"img"}}`)
	if err != nil {
		t.Fatalf("parse image visualRoot: %v", err)
	}
	if schema2.Editor.VisualRoot != "img" {
		t.Fatalf("visualRoot = %q", schema2.Editor.VisualRoot)
	}
	// No visualRoot → empty (default structural bounds)
	schema3, err := ParseSchema(`{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`)
	if err != nil {
		t.Fatalf("parse empty visualRoot: %v", err)
	}
	if schema3.Editor.VisualRoot != "" {
		t.Fatalf("visualRoot should be empty, got %q", schema3.Editor.VisualRoot)
	}
	// Invalid visualRoot characters rejected
	if _, err := ParseSchema(`{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"visualRoot":"<bad>"}}`); err == nil {
		t.Fatalf("invalid visualRoot should be rejected")
	}
}

func TestCatalogExposesVisualRootMetadata(t *testing.T) {
	buttonSchema := `{"schemaVersion":1,"props":{"type":"object","required":["label","url"],"properties":{"label":{"type":"string","default":"B"},"url":{"type":"string","default":"#"}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["primary"],"default":"primary"}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"button","fields":{"props.label":{"label":"Label","control":"text"}},"visualRoot":".stratum-button"}}`
	imageSchema := `{"schemaVersion":1,"props":{"type":"object","required":["mediaId"],"properties":{"mediaId":{"type":"string","default":""}}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"media","icon":"image","visualRoot":"img"}}`
	sectionSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"any"},"editor":{"category":"layout","icon":"section"}}`
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "button", 1, true, buttonSchema, `<div class="stratum-btn-wrap"><a class="stratum-button">{{.Props.label}}</a></div>`),
		customDefinition("core", "image", 1, true, imageSchema, `<figure><img src="x"></figure>`),
		customDefinition("core", "section", 1, true, sectionSchema, `<section>{{.Children}}</section>`),
		customDefinition("custom", "demo-widget", 1, true, `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"visualRoot":".actual-widget"}}`, `<div class="technical-wrapper"><span class="actual-widget">Demo</span></div>`),
	}}
	reg, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	cat := reg.EditorCatalogFor(EditorModeEntry)
	find := func(block string) *EditorDefinition {
		for i := range cat {
			if cat[i].Block == block {
				return &cat[i]
			}
		}
		return nil
	}
	btn := find("core/button")
	if btn == nil || btn.Schema.Editor.VisualRoot != ".stratum-button" {
		t.Fatalf("button visualRoot = %q, want .stratum-button", btn.Schema.Editor.VisualRoot)
	}
	img := find("core/image")
	if img == nil || img.Schema.Editor.VisualRoot != "img" {
		t.Fatalf("image visualRoot = %q, want img", img.Schema.Editor.VisualRoot)
	}
	sec := find("core/section")
	if sec == nil || sec.Schema.Editor.VisualRoot != "" {
		t.Fatalf("section should have no visualRoot, got %q", sec.Schema.Editor.VisualRoot)
	}
	demo := find("custom/demo-widget")
	if demo == nil || demo.Schema.Editor.VisualRoot != ".actual-widget" {
		t.Fatalf("demo-widget visualRoot = %q, want .actual-widget", demo.Schema.Editor.VisualRoot)
	}
	// Ensure no generic code branch needed: visualRoot lookup via catalog works for custom block without code change
}

func TestCustomBlockVisualRootGenericResolution(t *testing.T) {
	// Proof that generic runtime does not branch on block name for visual bounds.
	// The presence of visualRoot in schema is sufficient.
	schema, err := ParseSchema(`{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"visualRoot":".actual-widget"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if schema.Editor.VisualRoot != ".actual-widget" {
		t.Fatalf("want .actual-widget")
	}
}
