package rendering

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func TestPublicRenderHasNoMarkers(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{.Children}}</section>`},
		{Namespace: "core", Name: "heading", Version: 1, RendererType: "template", Template: `<h1>{{.Props.text}}</h1>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "sec1", Block: "core/section", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`), Children: []document.Node{
			{ID: "h1", Block: "core/heading", Version: 1, Props: json.RawMessage(`{"text":"Hello"}`), Settings: json.RawMessage(`{}`)},
		}},
	}}
	rc := RenderContext{Mode: ModePublic}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	if strings.Contains(s, "stratum-node-start") || strings.Contains(s, "stratum-node-end") {
		t.Fatalf("public render should not contain markers: %s", s)
	}
}

func TestEditorCanvasHasMarkers(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{.Children}}</section>`},
		{Namespace: "core", Name: "heading", Version: 1, RendererType: "template", Template: `<h1>{{.Props.text}}</h1>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "sec1", Block: "core/section", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`), Children: []document.Node{
			{ID: "h1", Block: "core/heading", Version: 1, Props: json.RawMessage(`{"text":"Hello"}`), Settings: json.RawMessage(`{}`)},
		}},
	}}
	ids := map[string]struct{}{"sec1": {}, "h1": {}}
	rc := RenderContext{
		Mode:      ModePreview,
		IsPreview: true,
		Editor:    &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"},
	}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "stratum-node-start:sec1:") {
		t.Fatalf("editor canvas should contain start marker for sec1: %s", s)
	}
	if !strings.Contains(s, "stratum-node-start:h1:") {
		t.Fatalf("editor canvas should contain start marker for h1: %s", s)
	}
	if !strings.Contains(s, "stratum-node-end:sec1:") || !strings.Contains(s, "stratum-node-end:h1:") {
		t.Fatalf("missing end markers: %s", s)
	}
	// Editable true for both
	if !strings.Contains(s, ":true") {
		t.Fatalf("editable flag missing: %s", s)
	}
}

func TestEditorCanvasExternalMarkers(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{.Children}}</section>`},
		{Namespace: "core", Name: "heading", Version: 1, RendererType: "template", Template: `<h2>{{.Props.text}}</h2>`},
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{.Props.text}}</p>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "sec1", Block: "core/section", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`), Children: []document.Node{
			{ID: "h1", Block: "core/heading", Version: 1, Props: json.RawMessage(`{"text":"Template"}`), Settings: json.RawMessage(`{}`)},
			{ID: "txt1", Block: "core/text", Version: 1, Props: json.RawMessage(`{"text":"Entry"}`), Settings: json.RawMessage(`{}`)},
		}},
	}}
	// Only sec1 and h1 are editable (template), txt1 is external
	ids := map[string]struct{}{"sec1": {}, "h1": {}}
	rc := RenderContext{
		Mode:   ModePreview,
		Editor: &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"},
	}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "stratum-node-start:h1:") || !strings.Contains(s, ":true") {
		t.Fatalf("h1 should be editable: %s", s)
	}
	if !strings.Contains(s, "stratum-node-start:txt1:") {
		t.Fatalf("txt1 marker missing: %s", s)
	}
	if !strings.Contains(s, "txt1:") || !strings.Contains(s, ":false") {
		// Check that txt1 is marked false
		// Find segment for txt1
		idx := strings.Index(s, "stratum-node-start:txt1:")
		if idx < 0 {
			t.Fatalf("txt1 start not found")
		}
		segment := s[idx : idx+200]
		if !strings.Contains(segment, ":false") {
			t.Fatalf("txt1 should be external false, got %s", segment)
		}
	}
}

func TestNestedMarkersBoundaries(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{.Children}}</section>`},
		{Namespace: "core", Name: "stack", Version: 1, RendererType: "template", Template: `<div class="stack">{{.Children}}</div>`},
		{Namespace: "core", Name: "heading", Version: 1, RendererType: "template", Template: `<h1>{{.Props.text}}</h1>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "sec1", Block: "core/section", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`), Children: []document.Node{
			{ID: "stack1", Block: "core/stack", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`), Children: []document.Node{
				{ID: "h1", Block: "core/heading", Version: 1, Props: json.RawMessage(`{"text":"Hello"}`), Settings: json.RawMessage(`{}`)},
			}},
		}},
	}}
	ids := map[string]struct{}{"sec1": {}, "stack1": {}, "h1": {}}
	rc := RenderContext{Mode: ModePreview, Editor: &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"}}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	// Count starts and ends
	if strings.Count(s, "stratum-node-start:") != 3 {
		t.Fatalf("expected 3 starts, got %d: %s", strings.Count(s, "stratum-node-start:"), s)
	}
	if strings.Count(s, "stratum-node-end:") != 3 {
		t.Fatalf("expected 3 ends, got %d: %s", strings.Count(s, "stratum-node-end:"), s)
	}
	// Ensure nesting order: sec1 start before stack1 start before h1 start, and ends in reverse
	secStart := strings.Index(s, "stratum-node-start:sec1:")
	stackStart := strings.Index(s, "stratum-node-start:stack1:")
	h1Start := strings.Index(s, "stratum-node-start:h1:")
	h1End := strings.Index(s, "stratum-node-end:h1:")
	stackEnd := strings.Index(s, "stratum-node-end:stack1:")
	secEnd := strings.Index(s, "stratum-node-end:sec1:")
	if !(secStart < stackStart && stackStart < h1Start && h1Start < h1End && h1End < stackEnd && stackEnd < secEnd) {
		t.Fatalf("nesting order incorrect: %s", s)
	}
}

func TestNormalPreviewHasNoMarkers(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "text", Version: 1, RendererType: "template", Template: `<p>{{.Props.text}}</p>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "t1", Block: "core/text", Version: 1, Props: json.RawMessage(`{"text":"hi"}`), Settings: json.RawMessage(`{}`)}}}
	rc := RenderContext{Mode: ModePreview, IsPreview: true} // preview but no editor
	html, _ := renderer.RenderDocumentContext(doc, rc)
	if strings.Contains(string(html), "stratum-node") {
		t.Fatalf("normal preview should not have markers")
	}
}

func TestPublicRenderHasNoVisualRoot(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "button", Version: 1, RendererType: "template", Template: `<div class="stratum-btn-wrap"><a class="stratum-button">{{.Props.label}}</a></div>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "b1", Block: "core/button", Version: 1, Props: json.RawMessage(`{"label":"Click"}`), Settings: json.RawMessage(`{}`)}}}
	rc := RenderContext{Mode: ModePublic}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	if strings.Contains(s, "data-stratum-editor-visual-root") {
		t.Fatalf("public render should not contain visual root: %s", s)
	}
}

func TestEditorCanvasButtonHasVisualRoot(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "button", Version: 1, RendererType: "template", Template: `<div class="stratum-btn-wrap"><a class="stratum-button">{{.Props.label}}</a></div>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "b1", Block: "core/button", Version: 1, Props: json.RawMessage(`{"label":"Click"}`), Settings: json.RawMessage(`{}`)}}}
	ids := map[string]struct{}{"b1": {}}
	rc := RenderContext{Mode: ModePreview, IsPreview: true, Editor: &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"}}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "data-stratum-editor-visual-root") {
		t.Fatalf("editor canvas button should contain visual root: %s", s)
	}
	if strings.Count(s, "data-stratum-editor-visual-root") != 1 {
		t.Fatalf("expected exactly one visual root for button, got %d: %s", strings.Count(s, "data-stratum-editor-visual-root"), s)
	}
	// Ensure marker still present
	if !strings.Contains(s, "stratum-node-start:b1:") {
		t.Fatalf("should still have node marker: %s", s)
	}
}
