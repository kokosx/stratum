package revisions

import (
	"testing"

	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func mustDoc(t *testing.T, jsonStr string) *document.Document {
	t.Helper()
	doc, err := document.Decode([]byte(jsonStr))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return doc
}

func TestTitleChange(t *testing.T) {
	a := db.EntryRevision{Title: "Old Title", Slug: "old", DocumentJson: `{"version":1,"nodes":[]}`}
	b := db.EntryRevision{Title: "New Title", Slug: "old", DocumentJson: `{"version":1,"nodes":[]}`}
	diff, err := CompareRevisions(a, b, CompareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Metadata.Title == nil || !diff.Metadata.Title.Changed {
		t.Fatalf("expected title changed")
	}
	if diff.Metadata.Title.Old != "Old Title" || diff.Metadata.Title.New != "New Title" {
		t.Fatalf("title diff wrong: %+v", diff.Metadata.Title)
	}
}

func TestFieldChange(t *testing.T) {
	a := db.EntryRevision{Title: "T", FieldsJson: `{"price": 49, "name":"A"}`}
	b := db.EntryRevision{Title: "T", FieldsJson: `{"price": 59, "name":"A"}`}
	diff, err := CompareRevisions(a, b, CompareOptions{
		FieldSchemas: map[string]FieldSchema{
			"price": {Label: "Price", Type: "number"},
			"name":  {Label: "Name", Type: "text"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range diff.Fields {
		if f.Key == "price" && f.Changed {
			if f.Label != "Price" {
				t.Fatalf("label wrong: %s", f.Label)
			}
			found = true
		}
		if f.Key == "name" && f.Changed {
			t.Fatalf("name should not be changed")
		}
	}
	if !found {
		t.Fatalf("price field diff not found")
	}
}

func TestNodeTextChange(t *testing.T) {
	docA := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"Hello"},"settings":{},"children":[]}]}`
	docB := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"Welcome"},"settings":{},"children":[]}]}`
	a := db.EntryRevision{DocumentJson: docA}
	b := db.EntryRevision{DocumentJson: docB}
	diff, err := CompareRevisions(a, b, CompareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Content.Modified) != 1 {
		t.Fatalf("expected 1 modified, got %d", len(diff.Content.Modified))
	}
	if len(diff.Content.Modified[0].PropDiffs) == 0 {
		t.Fatalf("expected prop diffs")
	}
}

func TestNodeAdd(t *testing.T) {
	docA := `{"version":1,"nodes":[{"id":"a1","block":"core/text","version":1,"props":{"text":"A"},"settings":{},"children":[]}]}`
	docB := `{"version":1,"nodes":[{"id":"a1","block":"core/text","version":1,"props":{"text":"A"},"settings":{},"children":[]},{"id":"a2","block":"core/text","version":1,"props":{"text":"B"},"settings":{},"children":[]}]}`
	a := db.EntryRevision{DocumentJson: docA}
	b := db.EntryRevision{DocumentJson: docB}
	diff, _ := CompareRevisions(a, b, CompareOptions{})
	if len(diff.Content.Added) != 1 || diff.Content.Added[0].ID != "a2" {
		t.Fatalf("expected added a2, got %+v", diff.Content.Added)
	}
}

func TestNodeRemove(t *testing.T) {
	docA := `{"version":1,"nodes":[{"id":"a1","block":"core/text","version":1,"props":{"text":"A"},"settings":{},"children":[]},{"id":"a2","block":"core/text","version":1,"props":{"text":"B"},"settings":{},"children":[]}]}`
	docB := `{"version":1,"nodes":[{"id":"a1","block":"core/text","version":1,"props":{"text":"A"},"settings":{},"children":[]}]}`
	a := db.EntryRevision{DocumentJson: docA}
	b := db.EntryRevision{DocumentJson: docB}
	diff, _ := CompareRevisions(a, b, CompareOptions{})
	if len(diff.Content.Removed) != 1 || diff.Content.Removed[0].ID != "a2" {
		t.Fatalf("expected removed a2, got %+v", diff.Content.Removed)
	}
}

func TestNodeMove(t *testing.T) {
	docA := `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"b1","block":"core/button","version":1,"props":{"label":"Click"},"settings":{},"children":[]}]},{"id":"s2","block":"core/section","version":1,"props":{},"settings":{},"children":[]}]}`
	docB := `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{},"children":[]},{"id":"s2","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"b1","block":"core/button","version":1,"props":{"label":"Click"},"settings":{},"children":[]}]}]}`
	a := db.EntryRevision{DocumentJson: docA}
	b := db.EntryRevision{DocumentJson: docB}
	diff, _ := CompareRevisions(a, b, CompareOptions{})
	if len(diff.Content.Moved) != 1 {
		t.Fatalf("expected 1 moved, got %d: %+v", len(diff.Content.Moved), diff.Content)
	}
	if diff.Content.Moved[0].ID != "b1" {
		t.Fatalf("expected b1 moved, got %s", diff.Content.Moved[0].ID)
	}
	// Ensure not classified as added+removed
	if len(diff.Content.Added) != 0 || len(diff.Content.Removed) != 0 {
		t.Fatalf("move should not be added/removed")
	}
}

func TestNodeSettingChange(t *testing.T) {
	docA := `{"version":1,"nodes":[{"id":"img1","block":"core/image","version":1,"props":{"mediaId":"m1"},"settings":{"size":"medium"},"children":[]}]}`
	docB := `{"version":1,"nodes":[{"id":"img1","block":"core/image","version":1,"props":{"mediaId":"m1"},"settings":{"size":"large"},"children":[]}]}`
	a := db.EntryRevision{DocumentJson: docA}
	b := db.EntryRevision{DocumentJson: docB}
	diff, _ := CompareRevisions(a, b, CompareOptions{})
	if len(diff.Content.Modified) != 1 {
		t.Fatalf("expected 1 modified, got %d", len(diff.Content.Modified))
	}
	if len(diff.Content.Modified[0].SettingDiffs) == 0 {
		t.Fatalf("expected setting diffs")
	}
}

func TestHistoricalUnknownBlock(t *testing.T) {
	docA := `{"version":1,"nodes":[{"id":"u1","block":"core/unknown","version":99,"props":{"foo":"bar"},"settings":{},"children":[]}]}`
	docB := `{"version":1,"nodes":[{"id":"u1","block":"core/unknown","version":99,"props":{"foo":"baz"},"settings":{},"children":[]}]}`
	a := db.EntryRevision{DocumentJson: docA}
	b := db.EntryRevision{DocumentJson: docB}
	diff, err := CompareRevisions(a, b, CompareOptions{})
	if err != nil {
		t.Fatalf("should handle unknown block: %v", err)
	}
	if len(diff.Content.Modified) != 1 {
		t.Fatalf("expected modified for unknown block")
	}
}
