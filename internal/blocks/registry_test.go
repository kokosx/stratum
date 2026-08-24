package blocks

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type definitionStore struct {
	definitions []db.BlockDefinition
	err         error
}

func (s *definitionStore) ListBlockDefinitions(context.Context) ([]db.BlockDefinition, error) {
	return s.definitions, s.err
}

func TestRegistryReloadPublishesNewRenderer(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{blockDefinition(`<p>first</p>`)}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	assertRendered(t, registry, "<p>first</p>")

	store.definitions = []db.BlockDefinition{blockDefinition(`<p>second</p>`)}
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	assertRendered(t, registry, "<p>second</p>")
}

func TestRegistryFailedReloadKeepsCurrentSnapshot(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{blockDefinition(`<p>working</p>`)}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	store.definitions = []db.BlockDefinition{blockDefinition(`{{ if }}`)}
	err = registry.Reload(context.Background())
	if err == nil || !strings.Contains(err.Error(), "build block renderer") {
		t.Fatalf("Reload() error = %v, want template error", err)
	}
	assertRendered(t, registry, "<p>working</p>")
}

func TestPrepareSelectsOneNonDecorativeImageForLCP(t *testing.T) {
	image := blockDefinition(`<img>`)
	image.Name = "image"
	image.SchemaJson = `{"schemaVersion":1,"props":{"type":"object","properties":{"mediaId":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"decorative":{"type":"boolean","default":false},"priority":{"type":"string","enum":["auto","high","normal"],"default":"auto"}}},"children":{"mode":"none"},"editor":{}}`
	image.Styles = sql.NullString{String: ".image{}", Valid: true}
	registry, err := NewRegistry(context.Background(), &definitionStore{definitions: []db.BlockDefinition{image}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"decorative","block":"core/image","version":1,"props":{"mediaId":"a"},"settings":{"decorative":true}},{"id":"normal","block":"core/image","version":1,"props":{"mediaId":"b"},"settings":{"priority":"normal"}},{"id":"first","block":"core/image","version":1,"props":{"mediaId":"c"},"settings":{}},{"id":"high","block":"core/image","version":1,"props":{"mediaId":"d"},"settings":{"priority":"high"}},{"id":"other-high","block":"core/image","version":1,"props":{"mediaId":"e"},"settings":{"priority":"high"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := registry.Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.HighPriority) == 0 || prepared.HighPriority[0].ID != "high" {
		t.Fatalf("HighPriority = %#v, want first manual high image", prepared.HighPriority)
	}
	if len(prepared.AutoCandidates) == 0 || prepared.AutoCandidates[0].ID != "first" {
		t.Fatalf("AutoCandidates = %#v, want first auto", prepared.AutoCandidates)
	}
	if len(prepared.UsedBlocks) != 1 || prepared.UsedBlocks[0].Name != "core/image" {
		t.Fatalf("UsedBlocks = %#v, want core/image once", prepared.UsedBlocks)
	}
}

func blockDefinition(blockTemplate string) db.BlockDefinition {
	return db.BlockDefinition{
		Namespace: "core", Name: "text", Version: 1, RendererType: "template",
		DisplayName: "Text", SchemaJson: `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{}}`,
		Template: sql.NullString{String: blockTemplate, Valid: true}, Enabled: 1,
	}
}

func assertRendered(t *testing.T, registry *Registry, want string) {
	t.Helper()
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"text","block":"core/text","version":1,"props":{},"settings":{}}]}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	got, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatalf("RenderDocument() error = %v", err)
	}
	if string(got) != want {
		t.Errorf("rendered = %q, want %q", got, want)
	}
}
