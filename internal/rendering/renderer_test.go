package rendering

import (
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func TestRendererUsesVersionedDefinitions(t *testing.T) {
	doc, err := document.Decode([]byte(`{
        "version": 1,
        "nodes": [{
            "id": "card-1", "block": "example/card", "version": 1,
            "props": {"title": "<unsafe>"},
            "children": [{
                "id": "text-1", "block": "example/text", "version": 1,
                "props": {"text": "Body"}
            }]
        }]
    }`))
	if err != nil {
		t.Fatal(err)
	}

	renderer, err := NewRenderer([]Definition{
		{Namespace: "example", Name: "card", Version: 1, RendererType: "template", Template: `<article><h2>{{ .Props.title }}</h2>{{ .Children }}</article>`},
		{Namespace: "example", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{ .Props.text }}</p>`},
	})
	if err != nil {
		t.Fatal(err)
	}

	content, err := renderer.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), `<article><h2>&lt;unsafe&gt;</h2><p>Body</p></article>`; got != want {
		t.Errorf("rendered content = %q, want %q", got, want)
	}
}

func TestRendererRejectsMissingBlockDefinition(t *testing.T) {
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"missing","block":"example/missing","version":1,"props":{}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = renderer.RenderDocument(doc)
	if err == nil || !strings.Contains(err.Error(), "block definition not found") {
		t.Errorf("RenderDocument error = %v, want missing definition error", err)
	}
}
