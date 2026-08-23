package rendering

import (
	"context"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func BenchmarkPrepareDocument(b *testing.B) {
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
	// Build a registry with the core blocks used above.
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

func BenchmarkRenderPrepared(b *testing.B) {
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
