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
