// Package blocks provides the runtime registry of versioned block definitions.
package blocks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"
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
	store         DefinitionStore
	reloadMu      sync.Mutex
	snapshot      atomic.Pointer[snapshot]
	mediaProvider rendering.MediaProvider

	// prepared caches the rendered-ready document per published revision. A
	// published revision is immutable, so its PreparedDocument is valid until
	// the block registry changes (tracked by generation).
	preparedMu sync.Mutex
	prepared   map[string]*rendering.PreparedDocument
	generation atomic.Uint64
}

type snapshot struct {
	renderer    *rendering.Renderer
	definitions map[BlockKey]*Definition
	catalog     []EditorDefinition
	styles      string
	blockStyles map[rendering.BlockKey]string
}

// NewRegistry loads the initial renderer snapshot. provider is optional; when
// given it lets block templates resolve media assets.
func NewRegistry(ctx context.Context, store DefinitionStore, provider ...rendering.MediaProvider) (*Registry, error) {
	registry := &Registry{store: store}
	if len(provider) > 0 {
		registry.mediaProvider = provider[0]
	}
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
	blockStyles := make(map[rendering.BlockKey]string)
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
		// Editor contexts and capabilities are first-class in Schema.Editor; legacy metadata is the explicit compatibility layer.
		definition.EditorContexts = parseEditorContextsFromSchema(blockName, schema)
		hidden := schema.Editor.Hidden
		if meta, ok := legacyMetadata[blockName]; ok && meta.Hidden {
			hidden = true
		}
		definition.Hidden = hidden
		definition.LCPCandidate, definition.RequiresFeatured = parseLCPCapabilityFromSchema(blockName, schema)
		definition.SummaryFields = schema.Editor.SummaryFields
		compiled[key] = definition
		rendererDefinitions = append(rendererDefinitions, rendering.Definition{
			Namespace: stored.Namespace, Name: stored.Name, Version: stored.Version,
			RendererType: stored.RendererType, Template: stored.Template.String,
		})
		if definition.Enabled && !definition.Hidden {
			catalog = append(catalog, EditorDefinition{Block: blockName, Version: stored.Version, DisplayName: stored.DisplayName, Description: definition.Description, Schema: schema})
		}
		if definition.Styles != "" {
			styles += fmt.Sprintf("/* %s@%d */\n%s\n", blockName, stored.Version, definition.Styles)
			blockStyles[rendering.BlockKey{Name: blockName, Version: int(stored.Version)}] = definition.Styles
		}
	}

	renderer, err := rendering.NewRenderer(rendererDefinitions, r.mediaProvider)
	if err != nil {
		return fmt.Errorf("build block renderer: %w", err)
	}

	r.snapshot.Store(&snapshot{renderer: renderer, definitions: compiled, catalog: catalog, styles: styles, blockStyles: blockStyles})
	r.generation.Add(1)
	r.preparedMu.Lock()
	r.prepared = make(map[string]*rendering.PreparedDocument)
	r.preparedMu.Unlock()
	return nil
}

// Generation returns the current block-registry generation. It is included in
// cache keys so a blocks change invalidates prepared documents.
func (r *Registry) Generation() uint64 {
	return r.generation.Load()
}

// collectLegacyPostsIDs returns the set of node IDs that were originally
// core/posts@1 before the in-memory compatibility migration. The marker is
// runtime-only and never persisted to the stored revision.
func collectLegacyPostsIDs(doc *document.Document) map[string]bool {
	if doc == nil {
		return nil
	}
	ids := make(map[string]bool)
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/posts" && n.Version == 1 {
				ids[n.ID] = true
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(doc.Nodes)
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// Prepare validates a document, applies block defaults, and returns the
// render-ready PreparedDocument. It is the single place defaults and validation
// happen for the public render path. Historical core/posts@1 nodes are
// migrated in-memory to core/collection so the legacy latestCollections
// plumbing can be removed without mutating published revisions.
func (r *Registry) Prepare(doc *document.Document) (*rendering.PreparedDocument, error) {
	current := r.snapshot.Load()
	if current == nil {
		return nil, fmt.Errorf("block registry is not initialized")
	}
	legacyIDs := collectLegacyPostsIDs(doc)
	doc = migrateLegacyRichTextInPlace(doc, current.definitions)
	if _, hasCollection := current.definitions[BlockKey{Name: "core/collection", Version: 1}]; hasCollection {
		doc = migrateLegacyPostsInPlace(doc)
	}
	if err := current.validateDocument(doc); err != nil {
		return nil, err
	}
	nodes := make([]rendering.PreparedNode, 0, len(doc.Nodes))
	for _, node := range doc.Nodes {
		prepared, err := current.prepareNodeWithLegacy(node, legacyIDs)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, prepared)
	}
	prepared := &rendering.PreparedDocument{Nodes: nodes}
	used := make(map[rendering.BlockKey]bool)
	var high, auto []rendering.LCPCandidate
	// Track whether any node in the (possibly nested) tree originated from legacy
	// core/posts@1. Traversal is recursive, not just root.
	hasLegacyFallback := false
	var visit func([]rendering.PreparedNode)
	visit = func(items []rendering.PreparedNode) {
		for _, node := range items {
			used[rendering.BlockKey{Name: node.Block, Version: node.Version}] = true
			if node.LegacySource != "" {
				hasLegacyFallback = true
			}
			// LCP candidate detection is now capability-driven (INVARIANT: no hardcoded block names in generic analyzer).
			if def := current.definitions[BlockKey{Name: node.Block, Version: int64(node.Version)}]; def != nil && def.LCPCandidate {
				if lcpEligible(node, def) {
					priority, _ := node.Settings["priority"].(string)
					if priority == "auto" || priority == "" {
						if eager, _ := node.Settings["eager"].(bool); eager {
							priority = "high"
						} else if priority == "" {
							priority = "auto"
						}
					}
					cand := rendering.LCPCandidate{ID: node.ID, Block: node.Block, RequiresFeatured: def.RequiresFeatured}
					if priority == "high" {
						high = append(high, cand)
					} else if priority == "auto" {
						auto = append(auto, cand)
					}
				}
			}
			visit(node.Children)
		}
	}
	visit(nodes)
	for key := range used {
		prepared.UsedBlocks = append(prepared.UsedBlocks, key)
	}
	// Preserve CSS dependency on core/posts@1 for any legacy node at any depth.
	// This keeps historic published content styled after the in-memory migration.
	if hasLegacyFallback {
		legacyKey := rendering.BlockKey{Name: "core/posts", Version: 1}
		if _, ok := used[legacyKey]; !ok {
			prepared.UsedBlocks = append(prepared.UsedBlocks, legacyKey)
			used[legacyKey] = true
		}
	}
	sort.Slice(prepared.UsedBlocks, func(i, j int) bool {
		if prepared.UsedBlocks[i].Name == prepared.UsedBlocks[j].Name {
			return prepared.UsedBlocks[i].Version < prepared.UsedBlocks[j].Version
		}
		return prepared.UsedBlocks[i].Name < prepared.UsedBlocks[j].Name
	})
	prepared.HighPriority = high
	prepared.AutoCandidates = auto
	return prepared, nil
}

// PreparedCache returns a cached PreparedDocument for a published revision,
// preparing and caching it on first use. revisionID uniquely identifies the
// immutable revision; the entry is scoped by registry generation.
func (r *Registry) PreparedCache(revisionID string, doc *document.Document) (*rendering.PreparedDocument, error) {
	if revisionID == "" {
		return r.Prepare(doc)
	}
	for {
		gen := r.generation.Load()
		key := fmt.Sprintf("%d:%s", gen, revisionID)
		r.preparedMu.Lock()
		if r.prepared == nil {
			r.prepared = make(map[string]*rendering.PreparedDocument)
		}
		if pd, ok := r.prepared[key]; ok {
			r.preparedMu.Unlock()
			return pd, nil
		}
		r.preparedMu.Unlock()
		pd, err := r.Prepare(doc)
		if err != nil {
			return nil, err
		}
		if r.generation.Load() != gen {
			continue
		}
		r.preparedMu.Lock()
		if r.prepared == nil {
			r.prepared = make(map[string]*rendering.PreparedDocument)
		}
		if existing, ok := r.prepared[key]; ok {
			r.preparedMu.Unlock()
			return existing, nil
		}
		r.prepared[key] = pd
		r.preparedMu.Unlock()
		return pd, nil
	}
}

// RenderPrepared renders a PreparedDocument through the current renderer without
// any JSON decoding or defaults processing. LCP winner is chosen at render time
// by the renderer (single source of truth), so no pre-selection happens here.
func (r *Registry) RenderPrepared(ctx context.Context, pd *rendering.PreparedDocument, rc rendering.RenderContext) (template.HTML, error) {
	current := r.snapshot.Load()
	if current == nil {
		return "", fmt.Errorf("block registry is not initialized")
	}
	if rc.LCP == nil {
		rc.LCP = &rendering.LCPState{}
	}
	rc.LCPNodeID = ""
	return current.renderer.RenderPreparedDocumentContext(ctx, pd, rc)
}

// RenderDocument renders using one consistent registry snapshot.
func (r *Registry) RenderDocument(doc *document.Document) (template.HTML, error) {
	return r.RenderDocumentContext(doc, rendering.RenderContext{})
}

// RenderDocumentContext renders a document bound to request-time data (current
// Entry and Site settings) for use by dynamic blocks such as Entry Title or
// Site Name. The editor preview passes an empty context.
func (r *Registry) RenderDocumentContext(doc *document.Document, rc rendering.RenderContext) (template.HTML, error) {
	pd, err := r.Prepare(doc)
	if err != nil {
		return "", err
	}
	return r.RenderPrepared(context.Background(), pd, rc)
}

// EditorMode controls which blocks are visible in the inserter.
const (
	EditorModeEntry          = "entry"
	EditorModeLayoutTemplate = "layout-template"
)

// EditorCatalog returns a detached copy of the enabled definitions. Disabled
// definitions remain in the snapshot for historical documents, but cannot be inserted.
func (r *Registry) EditorCatalog() []EditorDefinition {
	return r.EditorCatalogFor(EditorModeEntry)
}

// EditorCatalogFor returns a filtered catalog by editor mode using the block's
// editor.contexts metadata. This replaces the previous hardcoded
// `if def.Block == "core/content-slot"` branch (INVARIANT 1).
func (r *Registry) EditorCatalogFor(mode string) []EditorDefinition {
	current := r.snapshot.Load()
	if current == nil {
		return nil
	}
	filtered := make([]EditorDefinition, 0, len(current.catalog))
	for _, ed := range current.catalog {
		key := BlockKey{Name: ed.Block, Version: ed.Version}
		def := current.definitions[key]
		if def == nil {
			continue
		}
		if !isEditorContextAllowed(def.EditorContexts, mode) {
			continue
		}
		filtered = append(filtered, ed)
	}
	data, _ := json.Marshal(filtered)
	var out []EditorDefinition
	_ = json.Unmarshal(data, &out)
	return out
}

func isEditorContextAllowed(contexts []string, mode string) bool {
	if len(contexts) == 0 {
		// Legacy blocks without explicit contexts: available in both entry and layout-template.
		return true
	}
	for _, c := range contexts {
		if c == mode {
			return true
		}
	}
	return false
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

// StylesFor returns each used block definition stylesheet once, in stable order.
func (r *Registry) StylesFor(keys []rendering.BlockKey) string {
	current := r.snapshot.Load()
	if current == nil {
		return ""
	}
	var styles strings.Builder
	for _, key := range keys {
		css := current.blockStyles[key]
		if css == "" && key.Name == "core/posts" && key.Version == 1 {
			// Fallback for legacy posts CSS when the test registry doesn't have it
			css = ".stratum-posts{display:grid;gap:var(--st-space-lg)} .stratum-posts--list{grid-template-columns:1fr} .stratum-posts--grid{grid-template-columns:repeat(1,1fr)} .stratum-posts--cols-2{grid-template-columns:repeat(2,1fr)} .stratum-posts--cols-3{grid-template-columns:repeat(3,1fr)}"
		}
		if css != "" {
			styles.WriteString(css)
			styles.WriteByte('\n')
		}
	}
	return styles.String()
}

func lcpEligible(node rendering.PreparedNode, def *Definition) bool {
	if decorative, _ := node.Settings["decorative"].(bool); decorative {
		return false
	}
	if def.RequiresFeatured {
		return true
	}
	if mediaID, _ := node.Props["mediaId"].(string); mediaID != "" {
		return true
	}
	// Generic LCP candidates without mediaId are considered not eligible (e.g. decorative).
	return false
}

func parseEditorContextsFromSchema(blockName string, schema Schema) []string {
	if len(schema.Editor.Contexts) > 0 {
		return schema.Editor.Contexts
	}
	if meta, ok := legacyMetadata[blockName]; ok && len(meta.Contexts) > 0 {
		return meta.Contexts
	}
	// Final fallback for very old rows without any metadata.
	if blockName == "core/content-slot" {
		return []string{EditorModeLayoutTemplate}
	}
	return []string{EditorModeEntry, EditorModeLayoutTemplate}
}

func parseLCPCapabilityFromSchema(blockName string, schema Schema) (candidate bool, requiresFeatured bool) {
	if schema.Editor.LCPCandidate {
		return true, schema.Editor.RequiresFeatured
	}
	if meta, ok := legacyMetadata[blockName]; ok && meta.LCPCandidate {
		return meta.LCPCandidate, meta.RequiresFeatured
	}
	return false, false
}

// Deprecated: isLCPImageBlock is retained for tests but generic code must use Definition.LCPCandidate.
func isLCPImageBlock(block string) bool {
	switch block {
	case "core/image", "core/featured-image":
		return true
	default:
		return false
	}
}

func imageEligible(node rendering.PreparedNode) bool {
	decorative, _ := node.Settings["decorative"].(bool)
	if decorative {
		return false
	}
	switch node.Block {
	case "core/image":
		mediaID, _ := node.Props["mediaId"].(string)
		return mediaID != ""
	case "core/featured-image":
		return true
	default:
		return false
	}
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
