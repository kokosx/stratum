package rendering

import (
	"context"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func BenchmarkRenderDocumentContext(b *testing.B) {
	b.ReportAllocs()
	doc := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "s1", Block: "core/section", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"width":"content"}`), Children: []document.Node{
				{ID: "h1", Block: "core/heading", Version: 1, Props: []byte(`{"text":"Hello","level":1}`)},
				{ID: "t1", Block: "core/text", Version: 1, Props: []byte(`{"text":"Body"}`)},
				{ID: "b1", Block: "core/button", Version: 1, Props: []byte(`{"label":"Click","url":"/"}`)},
			}},
		},
	}
	defs := []Definition{
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<div>{{ .Children }}</div>`},
		{Namespace: "core", Name: "heading", Version: 1, RendererType: "template", Template: `<h1>{{ .Props.text }}</h1>`},
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{ .Props.text }}</p>`},
		{Namespace: "core", Name: "button", Version: 1, RendererType: "template", Template: `<a href="{{ .Props.url }}">{{ .Props.label }}</a>`},
	}
	r, _ := NewRenderer(defs, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.RenderDocumentContext(doc, RenderContext{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrepareDocument was misnamed – it measured RenderDocumentContext.
// Kept for backward compat, now delegates to the correctly named benchmark.
func BenchmarkPrepareDocument(b *testing.B) { BenchmarkRenderDocumentContext(b) }

func BenchmarkRenderPrepared(b *testing.B) {
	b.ReportAllocs()
	doc := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "t1", Block: "core/text", Version: 1, Props: []byte(`{"text":"hello"}`)},
		},
	}
	defs := []Definition{
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{ .Props.text }}</p>`},
	}
	r, _ := NewRenderer(defs, nil)
	prepared, _ := r.RenderDocumentContext(doc, RenderContext{})
	_ = prepared
	// Use prepared path
	pd := &PreparedDocument{
		Nodes: []PreparedNode{{ID: "t1", Block: "core/text", Version: 1, Props: map[string]any{"text": "hello"}, Settings: map[string]any{}}},
	}
	rc := RenderContext{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.RenderPreparedDocumentContext(context.Background(), pd, rc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCollection10(b *testing.B) {
	b.ReportAllocs()
	defs := []Definition{
		{Namespace: "core", Name: "collection", Version: 1, RendererType: "template", Template: `<div class="col">{{ .Children }}</div>`},
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{ .Props.text }}</p>`},
	}
	r, _ := NewRenderer(defs, nil)
	// Mock ContentReader returning 10 entries
	cr := &mockContentReader{entries: make([]ArchiveEntry, 10)}
	for i := range cr.entries {
		cr.entries[i] = ArchiveEntry{ID: "id", Title: "T", URL: "/t"}
	}
	rc := RenderContext{ContentReader: cr, QueryCache: make(map[string][]ArchiveEntry)}
	pd := &PreparedDocument{Nodes: []PreparedNode{{ID: "c1", Block: "core/collection", Version: 1, Settings: map[string]any{"source": "query", "limit": float64(10), "contentType": "post"}, Children: []PreparedNode{{ID: "t1", Block: "core/text", Version: 1, Props: map[string]any{"text": "hi"}}}}}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.RenderPreparedDocumentContext(context.Background(), pd, rc); err != nil {
			b.Fatal(err)
		}
	}
}

type mockContentReader struct{ entries []ArchiveEntry }

func (m *mockContentReader) Query(ctx context.Context, contentType string, limit, offset int, order string, excludeIDs []string) ([]ArchiveEntry, error) {
	if limit > len(m.entries) {
		limit = len(m.entries)
	}
	return m.entries[:limit], nil
}
