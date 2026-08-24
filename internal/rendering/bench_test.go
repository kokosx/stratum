package rendering

import (
	"context"
	"testing"
)

func BenchmarkRenderDocumentContext(b *testing.B) {
	b.ReportAllocs()
	defs := []Definition{
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<div>{{ .Children }}</div>`},
		{Namespace: "core", Name: "heading", Version: 1, RendererType: "template", Template: `<h1>{{ .Props.text }}</h1>`},
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{ .Props.text }}</p>`},
		{Namespace: "core", Name: "button", Version: 1, RendererType: "template", Template: `<a href="{{ .Props.url }}">{{ .Props.label }}</a>`},
	}
	r, _ := NewRenderer(defs, nil)
	pd := &PreparedDocument{
		Nodes: []PreparedNode{
			{ID: "s1", Block: "core/section", Version: 1, Settings: map[string]any{"width": "content"}, Children: []PreparedNode{
				{ID: "h1", Block: "core/heading", Version: 1, Props: map[string]any{"text": "Hello"}},
				{ID: "t1", Block: "core/text", Version: 1, Props: map[string]any{"text": "Body"}},
				{ID: "b1", Block: "core/button", Version: 1, Props: map[string]any{"label": "Click", "url": "/"}},
			}},
		},
	}
	rc := RenderContext{LCP: &LCPState{}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.RenderPreparedDocumentContext(context.Background(), pd, rc); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrepareDocument was misnamed – it measured RenderDocumentContext.
// Kept for backward compat, now delegates to the correctly named benchmark.
func BenchmarkPrepareDocument(b *testing.B) { BenchmarkRenderDocumentContext(b) }

func BenchmarkRenderPrepared(b *testing.B) {
	b.ReportAllocs()
	defs := []Definition{
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{ .Props.text }}</p>`},
	}
	r, _ := NewRenderer(defs, nil)
	pd := &PreparedDocument{
		Nodes: []PreparedNode{{ID: "t1", Block: "core/text", Version: 1, Props: map[string]any{"text": "hello"}, Settings: map[string]any{}}},
	}
	rc := RenderContext{LCP: &LCPState{}}
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
	cr := &mockContentReader{entries: make([]ArchiveEntry, 10)}
	for i := range cr.entries {
		cr.entries[i] = ArchiveEntry{ID: "id", Title: "T", URL: "/t"}
	}
	rc := RenderContext{ContentReader: cr, QueryCache: make(map[string][]ArchiveEntry), LCP: &LCPState{}}
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
