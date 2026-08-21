package blocks

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const cardSchema = `{
  "schemaVersion": 1,
  "props": {"type":"object","required":["title","description"],"properties":{
    "title":{"type":"string","default":"Untitled","minLength":1},
    "description":{"type":"string","default":""}
  }},
  "settings": {"type":"object","properties":{
    "variant":{"type":"string","enum":["plain","featured"],"default":"plain"}
  }},
  "children":{"mode":"none"},
  "editor":{"category":"test","icon":"card","fields":{
    "props.title":{"label":"Title","control":"text"},
    "props.description":{"label":"Description","control":"textarea"},
    "settings.variant":{"label":"Variant","control":"segmented","group":"Style"}
  }}
}`

func TestParseSchemaDefaultsAndRejectsUnsupportedKeywords(t *testing.T) {
	schema, err := ParseSchema(cardSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := schema.Props.Properties["title"].Default; got != "Untitled" {
		t.Fatalf("title default = %#v, want Untitled", got)
	}
	if got := schema.Settings.Properties["variant"].Default; got != "plain" {
		t.Fatalf("variant default = %#v, want plain", got)
	}
	unsupported := strings.Replace(cardSchema, `"minLength":1`, `"oneOf":[{"type":"string"}]`, 1)
	if _, err := ParseSchema(unsupported); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unsupported schema error = %v", err)
	}
}

func TestSchemaV1SupportedTypesAndArrayRepeaterValues(t *testing.T) {
	input := `{"schemaVersion":1,"props":{"type":"object","required":["title","count","ratio","active","tags","items"],"properties":{"title":{"type":"string","minLength":2,"maxLength":8,"pattern":"^[A-Z]"},"count":{"type":"integer","minimum":1,"maximum":3},"ratio":{"type":"number","minimum":0,"maximum":1},"active":{"type":"boolean","default":false},"tags":{"type":"array","items":{"type":"string"}},"items":{"type":"array","items":{"type":"object","required":["label"],"properties":{"label":{"type":"string"},"enabled":{"type":"boolean","default":true}}}}}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"fields":{"props.title":{"control":"text"},"props.count":{"control":"range"},"props.active":{"control":"checkbox"}}}}`
	schema, err := ParseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{"title": "Card", "count": float64(2), "ratio": .5, "active": false, "tags": []any{"one", "two"}, "items": []any{map[string]any{"label": "First", "enabled": true}}}
	if err := validateValue(schema.Props, valid, "props", true); err != nil {
		t.Fatalf("valid supported values rejected: %v", err)
	}
	invalid := map[string]any{"title": "lowercase", "count": float64(4), "ratio": .5, "active": false, "tags": []any{}, "items": []any{}}
	if err := validateValue(schema.Props, invalid, "props", true); err == nil || !strings.Contains(err.Error(), "props.") {
		t.Fatalf("constraint error = %v", err)
	}
	nestedArray := strings.Replace(input, `"items":{"type":"string"}`, `"items":{"type":"array","items":{"type":"string"}}`, 1)
	if _, err := ParseSchema(nestedArray); err == nil || !strings.Contains(err.Error(), "only primitives and simple objects") {
		t.Fatalf("nested array schema error = %v", err)
	}
}

func TestDynamicDefinitionCatalogValidationAndRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{customDefinition("test", "card", 1, true, cardSchema, `<article class="card-{{ .Settings.variant }}"><h2>{{ .Props.title }}</h2><p>{{ .Props.description }}</p></article>`)}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	catalog := registry.EditorCatalog()
	if len(catalog) != 1 || catalog[0].Block != "test/card" || catalog[0].Version != 1 {
		t.Fatalf("catalog = %#v", catalog)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"card-1","block":"test/card","version":1,"props":{"title":"Hello","description":"Dynamic"},"settings":{"variant":"featured"}}]}`)
	if err := registry.ValidateDocument(doc); err != nil {
		t.Fatal(err)
	}
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(rendered), `<article class="card-featured"><h2>Hello</h2><p>Dynamic</p></article>`; got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestRegistryRejectsInvalidPropsSettingsAndVersions(t *testing.T) {
	registry, err := NewRegistry(context.Background(), &definitionStore{definitions: []db.BlockDefinition{customDefinition("test", "card", 1, true, cardSchema, `<p>{{ .Props.title }}</p>`)}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, input, want string
	}{
		{"missing props", `{"version":1,"nodes":[{"id":"x","block":"test/card","version":1,"props":{"description":"D"},"settings":{"variant":"plain"}}]}`, "nodes[0].props.title: field is required"},
		{"invalid props", `{"version":1,"nodes":[{"id":"x","block":"test/card","version":1,"props":{"title":4,"description":"D"},"settings":{"variant":"plain"}}]}`, "nodes[0].props.title: expected string"},
		{"invalid settings", `{"version":1,"nodes":[{"id":"x","block":"test/card","version":1,"props":{"title":"T","description":"D"},"settings":{"variant":"loud"}}]}`, "nodes[0].settings.variant: expected one of plain, featured"},
		{"unknown block", `{"version":1,"nodes":[{"id":"x","block":"test/missing","version":1,"props":{},"settings":{}}]}`, "nodes[0]: unknown block test/missing@1"},
		{"wrong version", `{"version":1,"nodes":[{"id":"x","block":"test/card","version":2,"props":{},"settings":{}}]}`, "nodes[0]: unknown block test/card@2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := decodeTestDocument(t, test.input)
			if err := registry.ValidateDocument(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateDocument() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRegistryValidatesChildrenAndDuplicateIDs(t *testing.T) {
	containerSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"allowed","blocks":["test/card"],"min":1,"max":1},"editor":{}}`
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("test", "container", 1, true, containerSchema, `<div>{{ .Children }}</div>`),
		customDefinition("test", "card", 1, true, cardSchema, `<p>{{ .Props.title }}</p>`),
		customDefinition("test", "other", 1, true, strings.Replace(cardSchema, `"children":{"mode":"none"}`, `"children":{"mode":"none"}`, 1), `<p>other</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	disallowed := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"parent","block":"test/container","version":1,"props":{},"settings":{},"children":[{"id":"child","block":"test/other","version":1,"props":{"title":"T","description":"D"},"settings":{"variant":"plain"}}]}]}`)
	if err := registry.ValidateDocument(disallowed); err == nil || !strings.Contains(err.Error(), "nodes[0].children[0]: test/other is not allowed inside test/container") {
		t.Fatalf("children error = %v", err)
	}
	empty := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"parent","block":"test/container","version":1,"props":{},"settings":{}}]}`)
	if err := registry.ValidateDocument(empty); err == nil || !strings.Contains(err.Error(), "requires at least 1") {
		t.Fatalf("minimum children error = %v", err)
	}
	manual := &document.Document{Version: 1, Nodes: []document.Node{{ID: "same", Block: "test/card", Version: 1}, {ID: "same", Block: "test/card", Version: 1}}}
	if err := registry.ValidateDocument(manual); err == nil || !strings.Contains(err.Error(), "duplicate node id") {
		t.Fatalf("duplicate ID error = %v", err)
	}
}

func TestDisabledOldVersionRendersButIsNotInsertable(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("test", "card", 1, false, cardSchema, `<p>old {{ .Props.title }}</p>`),
		customDefinition("test", "card", 2, true, cardSchema, `<p>new {{ .Props.title }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	catalog := registry.EditorCatalog()
	if len(catalog) != 1 || catalog[0].Version != 2 {
		t.Fatalf("enabled catalog = %#v, want only version 2", catalog)
	}
	old := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"old","block":"test/card","version":1,"props":{"title":"history","description":"D"},"settings":{"variant":"plain"}}]}`)
	historicalDefinitions := registry.EditorDefinitions(old)
	if len(historicalDefinitions) != 1 || historicalDefinitions[0].Version != 1 {
		t.Fatalf("historical editor definitions = %#v, want version 1", historicalDefinitions)
	}
	rendered, err := registry.RenderDocument(old)
	if err != nil || string(rendered) != "<p>old history</p>" {
		t.Fatalf("historical render = %q, %v", rendered, err)
	}
}

func customDefinition(namespace, name string, version int64, enabled bool, schema, blockTemplate string) db.BlockDefinition {
	enabledValue := int64(0)
	if enabled {
		enabledValue = 1
	}
	return db.BlockDefinition{
		ID: namespace + "-" + name, Namespace: namespace, Name: name, Version: version,
		DisplayName: "Card", Description: sql.NullString{String: "Dynamic card", Valid: true},
		SchemaJson: schema, RendererType: "template", Template: sql.NullString{String: blockTemplate, Valid: true},
		Styles: sql.NullString{String: ".card{display:block}", Valid: true}, Source: "test", Enabled: enabledValue,
	}
}

func decodeTestDocument(t *testing.T, input string) *document.Document {
	t.Helper()
	doc, err := document.Decode([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
