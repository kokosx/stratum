package rendering

import (
	"context"
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
	}, nil)
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
	renderer, err := NewRenderer(nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = renderer.RenderDocument(doc)
	if err == nil || !strings.Contains(err.Error(), "block definition not found") {
		t.Errorf("RenderDocument error = %v, want missing definition error", err)
	}
}

func TestRendererExposesImmutableEntryFieldsInContext(t *testing.T) {
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"field","block":"example/field","version":1,"props":{}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer([]Definition{{Namespace: "example", Name: "field", Version: 1, RendererType: "template", Template: `{{ index .Context.Entry.Fields "price" }}`}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	output, err := renderer.RenderDocumentContext(doc, RenderContext{Entry: EntryContext{Fields: map[string]any{"price": 129.99}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "129.99" {
		t.Fatalf("rendered fields = %q", output)
	}
}

type fakeMediaProvider struct{ view MediaView }

func (f fakeMediaProvider) MediaView(_ context.Context, id string) (MediaView, bool) {
	if id == "missing" {
		return MediaView{}, false
	}
	return f.view, true
}

func TestRendererMediaFunctionResolvesProvider(t *testing.T) {
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"img","block":"example/image","version":1,"props":{"mediaID":"media_1"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer([]Definition{
		{Namespace: "example", Name: "image", Version: 1, RendererType: "template", Template: `<img src="{{ $m := media .Props.mediaID }}{{ if $m.Src }}{{ $m.Src }}{{ end }}"{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}>`},
	}, fakeMediaProvider{view: MediaView{Src: "/media/media_1/768", SrcSet: "/media/media_1/480 480w", Width: 800, Height: 600}})
	if err != nil {
		t.Fatal(err)
	}

	content, err := renderer.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), `<img src="/media/media_1/768" width="800">`; got != want {
		t.Errorf("rendered content = %q, want %q", got, want)
	}
}

func TestRendererMediaFunctionIsNilSafe(t *testing.T) {
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"img","block":"example/image","version":1,"props":{"mediaID":"media_1"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	// provider nil: the media function returns an empty view, rendering must not panic.
	renderer, err := NewRenderer([]Definition{
		{Namespace: "example", Name: "image", Version: 1, RendererType: "template", Template: `{{ $m := media .Props.mediaID }}{{ if $m.Src }}<img src="{{ $m.Src }}">{{ else }}<span>missing</span>{{ end }}`},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	content, err := renderer.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), `<span>missing</span>`; got != want {
		t.Errorf("rendered content = %q, want %q", got, want)
	}
}

func TestRendererMediaFunctionToleratesNonStringID(t *testing.T) {
	// A real document can carry a non-string mediaID (e.g. null when no asset is
	// selected). The media function must accept any value, not just a string, or
	// html/template fails at execution with "invalid value; expected string".
	for _, idJSON := range []string{`null`, `4`, `true`} {
		doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"img","block":"example/image","version":1,"props":{"mediaID":` + idJSON + `}}]}`))
		if err != nil {
			t.Fatal(err)
		}
		renderer, err := NewRenderer([]Definition{
			{Namespace: "example", Name: "image", Version: 1, RendererType: "template", Template: `{{ $m := media .Props.mediaID }}{{ if $m.Src }}<img src="{{ $m.Src }}">{{ else }}<span>missing</span>{{ end }}`},
		}, fakeMediaProvider{view: MediaView{Src: "/media/x/768"}})
		if err != nil {
			t.Fatal(err)
		}
		content, err := renderer.RenderDocument(doc)
		if err != nil {
			t.Fatalf("mediaID=%s: RenderDocument error = %v", idJSON, err)
		}
		if got, want := string(content), `<span>missing</span>`; got != want {
			t.Errorf("mediaID=%s: rendered content = %q, want %q", idJSON, got, want)
		}
	}
}
