package patterns

import (
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func TestPatternCloneRegeneratesIDs(t *testing.T) {
	cat := NewCatalog()
	p, ok := cat.Get("hero-centered")
	if !ok {
		t.Fatalf("hero-centered not found")
	}
	clone1, err := p.CloneWithNewIDs()
	if err != nil {
		t.Fatalf("clone1: %v", err)
	}
	clone2, err := p.CloneWithNewIDs()
	if err != nil {
		t.Fatalf("clone2: %v", err)
	}
	// IDs between clones must be distinct
	ids1 := collectIDs(clone1)
	ids2 := collectIDs(clone2)
	for id := range ids1 {
		if ids2[id] {
			t.Fatalf("duplicate ID %q between two clones", id)
		}
	}
	// Parent-child structure must be preserved
	if len(clone1.Nodes) != len(p.Document.Nodes) {
		t.Fatalf("clone node count mismatch")
	}
	// Ensure no duplicate inside single clone
	if len(ids1) != document.Count(clone1) {
		t.Fatalf("duplicate inside clone")
	}
	// Ensure original not mutated
	if p.Document.Nodes[0].ID == clone1.Nodes[0].ID {
		t.Fatalf("original mutated")
	}
}

func TestPatternCatalogValidation(t *testing.T) {
	cat := NewCatalog()
	// Validate without registry (just document structure)
	if err := cat.ValidateAll(nil); err != nil {
		t.Fatalf("validate without registry: %v", err)
	}
	// Every pattern must have unique IDs internally and non-empty fields
	for _, p := range cat.List("") {
		if p.ID == "" || p.Name == "" || p.Category == "" {
			t.Fatalf("pattern %s missing metadata", p.ID)
		}
		if err := document.Validate(&p.Document); err != nil {
			t.Fatalf("pattern %s document invalid: %v", p.ID, err)
		}
	}
}

func TestPatternValidationCatchesDuplicateIDs(t *testing.T) {
	doc := document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "dup", Block: "core/text", Version: 2, Props: document.MustMarshal(map[string]any{"text": map[string]any{"version": 1, "content": []any{}}}), Settings: document.MustMarshal(map[string]any{})},
			{ID: "dup", Block: "core/text", Version: 2, Props: document.MustMarshal(map[string]any{"text": map[string]any{"version": 1, "content": []any{}}}), Settings: document.MustMarshal(map[string]any{})},
		},
	}
	if err := document.Validate(&doc); err == nil {
		t.Fatalf("expected duplicate ID validation error")
	}
}

func TestCloneDocumentWithNewIDsPreservesStructure(t *testing.T) {
	doc := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "a", Block: "core/section", Version: 1, Props: document.MustMarshal(map[string]any{}), Settings: document.MustMarshal(map[string]any{}), Children: []document.Node{
				{ID: "b", Block: "core/heading", Version: 2, Props: document.MustMarshal(map[string]any{"text": map[string]any{"version": 1, "content": []any{}}, "level": 2}), Settings: document.MustMarshal(map[string]any{})},
			}},
		},
	}
	clone, err := CloneDocumentWithNewIDs(doc)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if clone.Nodes[0].ID == "a" {
		t.Fatalf("ID not regenerated")
	}
	if len(clone.Nodes[0].Children) != 1 || clone.Nodes[0].Children[0].ID == "b" {
		t.Fatalf("child ID not regenerated")
	}
}

func collectIDs(doc *document.Document) map[string]bool {
	m := make(map[string]bool)
	_ = document.Walk(doc, func(n document.Node) error {
		m[n.ID] = true
		return nil
	})
	return m
}
