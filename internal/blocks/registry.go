// Package blocks provides the runtime registry of enabled block renderers.
package blocks

import (
	"context"
	"fmt"
	"html/template"
	"sync"
	"sync/atomic"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/rendering"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// DefinitionStore is the source of truth for block definitions.
type DefinitionStore interface {
	ListBlockDefinitions(context.Context) ([]db.BlockDefinition, error)
}

// Registry holds the current, fully compiled block renderer. Its snapshot is
// replaced only after every enabled definition has been loaded and compiled.
type Registry struct {
	store    DefinitionStore
	reloadMu sync.Mutex
	snapshot atomic.Pointer[snapshot]
}

type snapshot struct {
	renderer *rendering.Renderer
}

// NewRegistry loads the initial renderer snapshot.
func NewRegistry(ctx context.Context, store DefinitionStore) (*Registry, error) {
	registry := &Registry{store: store}
	if err := registry.Reload(ctx); err != nil {
		return nil, err
	}
	return registry, nil
}

// Reload creates and atomically publishes a renderer from the enabled block
// definitions. A failed reload leaves the previous snapshot unchanged.
func (r *Registry) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	definitions, err := r.store.ListBlockDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("list enabled block definitions: %w", err)
	}

	rendererDefinitions := make([]rendering.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if !definition.Template.Valid {
			return fmt.Errorf("block %s/%s@%d: template is required", definition.Namespace, definition.Name, definition.Version)
		}
		rendererDefinitions = append(rendererDefinitions, rendering.Definition{
			Namespace: definition.Namespace, Name: definition.Name, Version: definition.Version,
			RendererType: definition.RendererType, Template: definition.Template.String,
		})
	}

	renderer, err := rendering.NewRenderer(rendererDefinitions)
	if err != nil {
		return fmt.Errorf("build block renderer: %w", err)
	}

	r.snapshot.Store(&snapshot{renderer: renderer})
	return nil
}

// RenderDocument renders using one consistent registry snapshot.
func (r *Registry) RenderDocument(doc *document.Document) (template.HTML, error) {
	current := r.snapshot.Load()
	if current == nil {
		return "", fmt.Errorf("block registry is not initialized")
	}
	return current.renderer.RenderDocument(doc)
}
