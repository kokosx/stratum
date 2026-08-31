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

func TestPublicRenderClean(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "button", Version: 1, RendererType: "template", Template: `<div class="stratum-btn-wrap"><a class="stratum-button">{{.Props.label}}</a></div>`},
		{Namespace: "core", Name: "image", Version: 1, RendererType: "template", Template: `<figure class="stratum-image"><img src="x.jpg"></figure>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "b1", Block: "core/button", Version: 1, Props: json.RawMessage(`{"label":"Click","url":"/"}`), Settings: json.RawMessage(`{}`)},
		{ID: "i1", Block: "core/image", Version: 1, Props: json.RawMessage(`{"mediaId":"m1"}`), Settings: json.RawMessage(`{}`)},
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
	if strings.Contains(s, "data-stratum-editor-visual-root") {
		t.Fatalf("public render should not contain visual root attribute: %s", s)
	}
}

func TestMarkerCarriesBlockAndVersion(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "core", Name: "button", Version: 1, RendererType: "template", Template: `<div class="stratum-btn-wrap"><a class="stratum-button">{{.Props.label}}</a></div>`},
		{Namespace: "core", Name: "image", Version: 1, RendererType: "template", Template: `<figure class="stratum-image"><img src="x.jpg"></figure>`},
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{.Children}}</section>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "b1", Block: "core/button", Version: 1, Props: json.RawMessage(`{"label":"Click","url":"/"}`), Settings: json.RawMessage(`{}`)},
		{ID: "i1", Block: "core/image", Version: 1, Props: json.RawMessage(`{"mediaId":"m1"}`), Settings: json.RawMessage(`{}`)},
		{ID: "sec1", Block: "core/section", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`)},
	}}
	ids := map[string]struct{}{"b1": {}, "i1": {}, "sec1": {}}
	rc := RenderContext{Mode: ModePreview, IsPreview: true, Editor: &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"}}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	// Marker must carry block+version, not just nodeId
	for _, want := range []string{"core%2Fbutton:1", "core%2Fimage:1", "core%2Fsection:1"} {
		if !strings.Contains(s, want) {
			t.Fatalf("marker should contain block/version %q, got %s", want, s)
		}
	}
	// Public output must stay clean: no strings.Replace visual-root patch
	if strings.Contains(s, "data-stratum-editor-visual-root") {
		t.Fatalf("generic renderer must not inject data-stratum-editor-visual-root, got %s", s)
	}
	// Markers still present
	for _, id := range []string{"b1", "i1", "sec1"} {
		if !strings.Contains(s, "stratum-node-start:"+id+":") {
			t.Fatalf("missing start marker for %s: %s", id, s)
		}
		if !strings.Contains(s, "stratum-node-end:"+id+":") {
			t.Fatalf("missing end marker for %s: %s", id, s)
		}
	}
}

func TestCustomBlockVisualRootIsMetadataDriven(t *testing.T) {
	renderer, _ := NewRenderer([]Definition{
		{Namespace: "custom", Name: "demo-widget", Version: 1, RendererType: "template", Template: `<div class="technical-wrapper"><span class="actual-widget">Demo</span></div>`},
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{.Children}}</section>`},
	}, nil)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "w1", Block: "custom/demo-widget", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`)},
	}}
	ids := map[string]struct{}{"w1": {}}
	rc := RenderContext{Mode: ModePreview, IsPreview: true, Editor: &EditorCanvas{Enabled: true, EditableNodeIDs: ids, InstanceScope: "root"}}
	html, err := renderer.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	// Marker must carry custom block identity without generic renderer branching
	if !strings.Contains(s, "custom%2Fdemo-widget:1") {
		t.Fatalf("custom block marker should contain block/version, got %s", s)
	}
	// Generic renderer must not inject visual-root attribute for custom block
	if strings.Contains(s, "data-stratum-editor-visual-root") {
		t.Fatalf("custom block should not have hardcoded visual root injection, got %s", s)
	}
	// Ensure technical wrapper and actual widget are present but untouched
	if !strings.Contains(s, `class="technical-wrapper"`) || !strings.Contains(s, `class="actual-widget"`) {
		t.Fatalf("custom block markup should be untouched: %s", s)
	}
	// Ensure renderer output for custom block is generic: no block-name branch strings remain
	// (we already checked no visual-root attr; additionally ensure public clean)
	rcPublic := RenderContext{Mode: ModePublic}
	htmlPub, _ := renderer.RenderDocumentContext(doc, rcPublic)
	if strings.Contains(string(htmlPub), "stratum-node") {
		t.Fatalf("public render should not contain markers")
	}
}
