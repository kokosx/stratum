package blocks

import (
	"context"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/rendering"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const dynamicLeafSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"level":{"type":"integer","default":1},"align":{"type":"string","default":"left"},"location":{"type":"string","default":"primary"},"style":{"type":"string","default":"horizontal"}}},"children":{"mode":"none"},"editor":{"contexts":["archive-template","site-part"]}}`

func epic2RuntimeRegistry(t *testing.T) *Registry {
	t.Helper()
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "archive-title", 1, true, dynamicLeafSchema, `legacy archive title template`),
		customDefinition("core", "archive-description", 1, true, dynamicLeafSchema, `legacy archive description template`),
		customDefinition("core", "navigation", 1, true, dynamicLeafSchema, `{{ template "menu-items" . }}`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestArchivePresentationBlocksNeverLeakPublicPlaceholders(t *testing.T) {
	registry := epic2RuntimeRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "title", Block: "core/archive-title", Version: 1},
		{ID: "description", Block: "core/archive-description", Version: 1},
	}}
	publicHTML, err := registry.RenderDocumentContext(doc, rendering.RenderContext{Mode: rendering.ModePublic})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicHTML), "Archive title") || strings.Contains(string(publicHTML), "Archive description") {
		t.Fatalf("public placeholders leaked: %s", publicHTML)
	}
	previewHTML, err := registry.RenderDocumentContext(doc, rendering.RenderContext{Mode: rendering.ModePreview})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(previewHTML), "Archive title") || !strings.Contains(string(previewHTML), "Archive description") {
		t.Fatalf("preview placeholders missing: %s", previewHTML)
	}
}

func TestNavigationRuntimeIsNestedAccessibleAndEscaped(t *testing.T) {
	registry := epic2RuntimeRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "nav", Block: "core/navigation", Version: 1}}}
	rc := rendering.RenderContext{Mode: rendering.ModePublic, Navigation: map[string]navigation.Menu{
		"primary": {Items: []navigation.MenuItem{{Label: "Home", URL: "/", Current: true}, {Label: `<About>`, URL: `/?q=\"x\"`, Children: []navigation.MenuItem{{Label: "Team", URL: "/team"}}}}},
	}}
	html, err := registry.RenderDocumentContext(doc, rc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(html)
	for _, want := range []string{`<nav`, `aria-label="Primary navigation"`, `aria-current="page"`, `Team`, `&lt;About&gt;`, `<ul>`} {
		if !strings.Contains(got, want) {
			t.Fatalf("navigation output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, `<About>`) || strings.Contains(got, `menu-items not defined`) {
		t.Fatalf("unsafe or theme-dependent navigation output: %s", got)
	}
}
