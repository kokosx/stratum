package blocks

import (
	"testing"
)

func TestStarterChildrenSchema(t *testing.T) {
	valid := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"allowed","blocks":["core/accordion-item"],"min":1},"editor":{"starterChildren":[{"block":"core/accordion-item","version":1}]}}`
	if _, err := ParseSchema(valid); err != nil {
		t.Fatalf("valid starterChildren rejected: %v", err)
	}
	invalidBlock := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"any"},"editor":{"starterChildren":[{"block":"bad","version":1}]}}`
	if _, err := ParseSchema(invalidBlock); err == nil {
		t.Fatal("invalid block name in starterChildren accepted")
	}
	invalidVer := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"any"},"editor":{"starterChildren":[{"block":"core/text","version":0}]}}`
	if _, err := ParseSchema(invalidVer); err == nil {
		t.Fatal("zero version in starterChildren accepted")
	}
	empty := `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"any"},"editor":{}}`
	schema, err := ParseSchema(empty)
	if err != nil {
		t.Fatalf("empty starterChildren parse failed: %v", err)
	}
	if len(schema.Editor.StarterChildren) != 0 {
		t.Fatalf("expected no starterChildren, got %v", schema.Editor.StarterChildren)
	}
}
