package blocks

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func customDefP(ns, name string, version int64, schema string) db.BlockDefinition {
	return db.BlockDefinition{ID: ns + "-" + name, Namespace: ns, Name: name, Version: version, DisplayName: name, SchemaJson: schema, RendererType: "template", Template: sql.NullString{String: "<div>{{.Children}}</div>", Valid: true}, Source: "test", Enabled: 1}
}

func TestPlacementValidation(t *testing.T) {
	accordionSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"allowed","blocks":["core/accordion-item"],"min":1},"editor":{}}`
	accordionItemSchema := `{"schemaVersion":1,"placement":{"parents":["core/accordion"]},"props":{"type":"object","properties":{"title":{"type":"string","default":"Item"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{"category":"design","fields":{"props.title":{"label":"Title","control":"text"}}}}`
	sectionSchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{}}`
	childSchema := `{"schemaVersion":1,"placement":{"parents":["test/parent-a","test/parent-b"]},"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`
	parentASchema := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{}}`
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefP("core", "section", 1, sectionSchema),
		customDefP("core", "accordion", 1, accordionSchema),
		customDefP("core", "accordion-item", 1, accordionItemSchema),
		customDefP("test", "parent-a", 1, parentASchema),
		customDefP("test", "parent-b", 1, parentASchema),
		customDefP("test", "parent-c", 1, parentASchema),
		customDefP("test", "child", 1, childSchema),
	}}
	reg, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, json string
		wantFail   bool
	}{
		{"root accordion-item should fail", `{"version":1,"nodes":[{"id":"x","block":"core/accordion-item","version":1,"props":{"title":"t"},"settings":{}}]}`, true},
		{"section->accordion-item should fail", `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"item","block":"core/accordion-item","version":1,"props":{"title":"t"},"settings":{}}]}]}`, true},
		{"accordion->accordion-item should pass", `{"version":1,"nodes":[{"id":"acc","block":"core/accordion","version":1,"props":{},"settings":{},"children":[{"id":"item","block":"core/accordion-item","version":1,"props":{"title":"t"},"settings":{}}]}]}`, false},
		{"test/child in parent-a pass", `{"version":1,"nodes":[{"id":"pa","block":"test/parent-a","version":1,"props":{},"settings":{},"children":[{"id":"c","block":"test/child","version":1,"props":{},"settings":{}}]}]}`, false},
		{"test/child in parent-b pass", `{"version":1,"nodes":[{"id":"pb","block":"test/parent-b","version":1,"props":{},"settings":{},"children":[{"id":"c","block":"test/child","version":1,"props":{},"settings":{}}]}]}`, false},
		{"test/child in parent-c fail", `{"version":1,"nodes":[{"id":"pc","block":"test/parent-c","version":1,"props":{},"settings":{},"children":[{"id":"c","block":"test/child","version":1,"props":{},"settings":{}}]}]}`, true},
		{"test/child at root fail", `{"version":1,"nodes":[{"id":"c","block":"test/child","version":1,"props":{},"settings":{}}]}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc, _ := document.Decode([]byte(tc.json))
			err := reg.ValidateDocument(doc)
			if tc.wantFail && err == nil {
				t.Fatalf("expected fail got ok")
			}
			if !tc.wantFail && err != nil {
				t.Fatalf("expected ok got %v", err)
			}
		})
	}
}

func TestPlacementSchemaValidation(t *testing.T) {
	invalid := []string{
		`{"schemaVersion":1,"placement":{"parents":["Accordion"]},"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`,
		`{"schemaVersion":1,"placement":{"parents":["foo"]},"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`,
		`{"schemaVersion":1,"placement":{"parents":["/bad"]},"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`,
		`{"schemaVersion":1,"placement":{"parents":["core/accordion","core/accordion"]},"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`,
	}
	for i, s := range invalid {
		if _, err := ParseSchema(s); err == nil {
			t.Fatalf("invalid %d should reject", i)
		}
	}
	if _, err := ParseSchema(`{"schemaVersion":1,"placement":{"parents":["core/accordion"]},"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`); err != nil {
		t.Fatalf("valid schema: %v", err)
	}
	if _, err := ParseSchema(`{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`); err != nil {
		t.Fatalf("missing placement should be ok: %v", err)
	}
}
