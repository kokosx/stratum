// Package blocks provides the runtime registry of versioned block definitions.
package blocks

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Registry holds a fully compiled immutable snapshot. Readers use atomic loads;
// Reload serializes writers and publishes only after every definition compiles.
type Registry struct {
	store    DefinitionStore
	reloadMu sync.Mutex
	snapshot atomic.Pointer[snapshot]
}

type snapshot struct {
	renderer    *rendering.Renderer
	definitions map[BlockKey]*Definition
	catalog     []EditorDefinition
	styles      string
}

// NewRegistry loads the initial renderer snapshot.
func NewRegistry(ctx context.Context, store DefinitionStore) (*Registry, error) {
	registry := &Registry{store: store}
	if err := registry.Reload(ctx); err != nil {
		return nil, err
	}
	return registry, nil
}

// Reload creates and atomically publishes a renderer from all installed block
// definitions. A failed reload leaves the previous snapshot unchanged.
func (r *Registry) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	definitions, err := r.store.ListBlockDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("list block definitions: %w", err)
	}

	rendererDefinitions := make([]rendering.Definition, 0, len(definitions))
	compiled := make(map[BlockKey]*Definition, len(definitions))
	catalog := make([]EditorDefinition, 0, len(definitions))
	styles := ""
	for _, stored := range definitions {
		blockName := stored.Namespace + "/" + stored.Name
		if !blockNamePattern.MatchString(blockName) {
			return fmt.Errorf("invalid block name %q", blockName)
		}
		if !stored.Template.Valid {
			return fmt.Errorf("block %s@%d: template is required", blockName, stored.Version)
		}
		schema, err := ParseSchema(stored.SchemaJson)
		if err != nil {
			return fmt.Errorf("block %s@%d: %w", blockName, stored.Version, err)
		}
		key := BlockKey{Name: blockName, Version: stored.Version}
		if _, exists := compiled[key]; exists {
			return fmt.Errorf("duplicate block definition: %s@%d", blockName, stored.Version)
		}
		definition := &Definition{
			Namespace: stored.Namespace, Name: stored.Name, Version: stored.Version,
			DisplayName: stored.DisplayName, Description: nullString(stored.Description),
			Schema: schema, Template: stored.Template.String, Styles: nullString(stored.Styles),
			Source: stored.Source, Enabled: stored.Enabled == 1,
		}
		compiled[key] = definition
		rendererDefinitions = append(rendererDefinitions, rendering.Definition{
			Namespace: stored.Namespace, Name: stored.Name, Version: stored.Version,
			RendererType: stored.RendererType, Template: stored.Template.String,
		})
		if definition.Enabled {
			catalog = append(catalog, EditorDefinition{Block: blockName, Version: stored.Version, DisplayName: stored.DisplayName, Description: definition.Description, Schema: schema})
		}
		if definition.Styles != "" {
			styles += fmt.Sprintf("/* %s@%d */\n%s\n", blockName, stored.Version, definition.Styles)
		}
	}

	renderer, err := rendering.NewRenderer(rendererDefinitions)
	if err != nil {
		return fmt.Errorf("build block renderer: %w", err)
	}

	r.snapshot.Store(&snapshot{renderer: renderer, definitions: compiled, catalog: catalog, styles: styles})
	return nil
}

// RenderDocument renders using one consistent registry snapshot.
func (r *Registry) RenderDocument(doc *document.Document) (template.HTML, error) {
	current := r.snapshot.Load()
	if current == nil {
		return "", fmt.Errorf("block registry is not initialized")
	}
	if err := current.validateDocument(doc); err != nil {
		return "", err
	}
	renderDocument, err := current.documentWithDefaults(doc)
	if err != nil {
		return "", fmt.Errorf("apply block defaults: %w", err)
	}
	return current.renderer.RenderDocument(renderDocument)
}

// EditorCatalog returns a detached copy of the enabled definitions. Disabled
// definitions remain in the snapshot for historical documents, but cannot be inserted.
func (r *Registry) EditorCatalog() []EditorDefinition {
	current := r.snapshot.Load()
	if current == nil {
		return nil
	}
	data, _ := json.Marshal(current.catalog)
	var catalog []EditorDefinition
	_ = json.Unmarshal(data, &catalog)
	return catalog
}

// EditorDefinitions returns exact definitions referenced by a document,
// including disabled historical versions. They are inspector data, not inserter data.
func (r *Registry) EditorDefinitions(doc *document.Document) []EditorDefinition {
	current := r.snapshot.Load()
	if current == nil || doc == nil {
		return nil
	}
	seen := make(map[BlockKey]bool)
	result := make([]EditorDefinition, 0)
	var visit func([]document.Node)
	visit = func(nodes []document.Node) {
		for _, node := range nodes {
			key := BlockKey{Name: node.Block, Version: int64(node.Version)}
			if !seen[key] {
				seen[key] = true
				if definition := current.definitions[key]; definition != nil {
					result = append(result, EditorDefinition{Block: key.Name, Version: key.Version, DisplayName: definition.DisplayName, Description: definition.Description, Schema: definition.Schema})
				}
			}
			visit(node.Children)
		}
	}
	visit(doc.Nodes)
	data, _ := json.Marshal(result)
	result = nil
	_ = json.Unmarshal(data, &result)
	return result
}

func (r *Registry) Styles() string {
	current := r.snapshot.Load()
	if current == nil {
		return ""
	}
	return current.styles
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
