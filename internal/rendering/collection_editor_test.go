package rendering

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
)

type stubContentReader struct {
	entries []ArchiveEntry
}

func (s *stubContentReader) Query(_ context.Context, q content.EntryQuery) ([]ArchiveEntry, error) {
	// return first Limit entries
	limit := q.Limit
	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}
	return s.entries[:limit], nil
}

func TestCollectionRepeatedInstanceMarkers(t *testing.T) {
	// This test verifies Node ID != Render Instance ID for Collection.
	// A Collection with one child Entry Title (nodeId title123) rendered with 3 entries
	// should produce 3 occurrences, same nodeId, 3 different instanceKeys.
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "collection", Version: 1, RendererType: "template", Template: `<div class="collection">{{.Children}}</div>`},
		{Namespace: "core", Name: "stack", Version: 1, RendererType: "template", Template: `<div class="stack">{{.Children}}</div>`},
		{Namespace: "core", Name: "entry-title", Version: 1, RendererType: "template", Template: `<h2>{{.Context.Entry.Title}}</h2>`},
	}, nil)
	// Need to ensure collection is registered as runtime — NewRenderer does that
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "coll1", Block: "core/collection", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{"contentType":"post","limit":3}`), Children: []document.Node{
			{ID: "title123", Block: "core/entry-title", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`)},
		}},
	}}
	entries := []ArchiveEntry{
		{ID: "entryA", Slug: "a", Title: "A", URL: "/a"},
		{ID: "entryB", Slug: "b", Title: "B", URL: "/b"},
		{ID: "entryC", Slug: "c", Title: "C", URL: "/c"},
	}
	reader := &stubContentReader{entries: entries}
	ids := map[string]struct{}{"coll1": {}, "title123": {}}
	rc := RenderContext{
		Mode:          ModePreview,
		Editor:        &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"},
		ContentReader: reader,
		QueryCache:    make(map[string][]ArchiveEntry),
	}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	// Count occurrences of title123 markers
	count := strings.Count(s, "stratum-node-start:title123:")
	if count != 3 {
		t.Fatalf("expected 3 occurrences of title123, got %d: %s", count, s)
	}
	// Ensure 3 different instanceKeys
	// Extract instanceKeys
	keys := []string{}
	// collect instanceKeys by scanning
	parts := strings.Split(s, "stratum-node-start:title123:")
	seen := map[string]bool{}
	for i := 1; i < len(parts); i++ {
		// instanceKey until next ':'
		// marker is title123:instanceKey:editable
		// instanceKey contains / and : but we can extract until :true or :false
		segment := parts[i]
		// Find :true or :false
		idxTrue := strings.Index(segment, ":true")
		idxFalse := strings.Index(segment, ":false")
		idx := -1
		if idxTrue >= 0 && idxFalse >= 0 {
			if idxTrue < idxFalse {
				idx = idxTrue
			} else {
				idx = idxFalse
			}
		} else if idxTrue >= 0 {
			idx = idxTrue
		} else if idxFalse >= 0 {
			idx = idxFalse
		}
		if idx < 0 {
			t.Fatalf("could not parse instanceKey in %q", segment[:100])
		}
		key := segment[:idx]
		seen[key] = true
		keys = append(keys, key)
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct instanceKeys, got %d: %v", len(seen), keys)
	}
	// Verify each instanceKey contains entry ID
	for _, k := range keys {
		hasEntry := strings.Contains(k, "entry:entryA") || strings.Contains(k, "entry:entryB") || strings.Contains(k, "entry:entryC")
		if !hasEntry {
			t.Fatalf("instanceKey should contain entry ID, got %q", k)
		}
	}
}
