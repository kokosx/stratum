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
	if _, hasCollection := current.definitions[BlockKey{Name: "core/collection", Version: 1}]; hasCollection {
		doc = migrateLegacyPostsInPlace(doc)
	}
	if err := current.validateDocument(doc); err != nil {
		return nil, err
	}
	nodes := make([]rendering.PreparedNode, 0, len(doc.Nodes))
	for _, node := range doc.Nodes {
		prepared, err := current.prepareNode(node)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, prepared)
	}
	prepared := &rendering.PreparedDocument{Nodes: nodes}
	used := make(map[rendering.BlockKey]bool)
	var high, auto []rendering.LCPCandidate
	var visit func([]rendering.PreparedNode)
	visit = func(items []rendering.PreparedNode) {
		for _, node := range items {
			used[rendering.BlockKey{Name: node.Block, Version: node.Version}] = true
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
	sort.Slice(prepared.UsedBlocks, func(i, j int) bool {
		if prepared.UsedBlocks[i].Name == prepared.UsedBlocks[j].Name {
			return prepared.UsedBlocks[i].Version < prepared.UsedBlocks[j].Version
		}
		return prepared.UsedBlocks[i].Name < prepared.UsedBlocks[j].Name
	})
	prepared.HighPriority = high
	prepared.AutoCandidates = auto
	prepared.LCPCandidate = prepared.ResolveLCP(true) // best-effort for legacy direct access
	return prepared, nil
}

// PreparedCache returns a cached PreparedDocument for a published revision,
// preparing and caching it on first use. revisionID uniquely identifies the
// immutable revision; the entry is dropped automatically when the registry
// generation changes.
func (r *Registry) PreparedCache(revisionID string, doc *document.Document) (*rendering.PreparedDocument, error) {
	if revisionID != "" {
		gen := r.generation.Load()
		key := revisionID
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
		r.preparedMu.Lock()
		r.prepared[key] = pd
		_ = gen
		r.preparedMu.Unlock()
		return pd, nil
	}
	return r.Prepare(doc)
}

// RenderPrepared renders a PreparedDocument through the current renderer without
// any JSON decoding or defaults processing. The single LCP candidate chosen at
// prepare time is bound here, so exactly one image node renders eager with
// fetchpriority=high regardless of which pipeline supplies the context.
func (r *Registry) RenderPrepared(ctx context.Context, pd *rendering.PreparedDocument, rc rendering.RenderContext) (template.HTML, error) {
	current := r.snapshot.Load()
	if current == nil {
		return "", fmt.Errorf("block registry is not initialized")
	}
	hasFeatured := rc.Entry.FeaturedImage != ""
	rc.LCPNodeID = pd.ResolveLCP(hasFeatured)
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
	current := r.snapshot.Load()
	if current == nil {
		return "", fmt.Errorf("block registry is not initialized")
	}
	if _, hasCollection := current.definitions[BlockKey{Name: "core/collection", Version: 1}]; hasCollection {
		doc = migrateLegacyPostsInPlace(doc)
	}
	if err := current.validateDocument(doc); err != nil {
		return "", err
	}
	renderDocument, err := current.documentWithDefaults(doc)
	if err != nil {
		return "", fmt.Errorf("apply block defaults: %w", err)
	}
	return current.renderer.RenderDocumentContext(renderDocument, rc)
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
		if css := current.blockStyles[key]; css != "" {
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

func parseEditorContexts(blockName, schemaJSON string) []string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &raw); err != nil {
		return nil
	}
	editor, ok := raw["editor"].(map[string]any)
	if !ok {
		return []string{EditorModeEntry, EditorModeLayoutTemplate}
	}
	val, ok := editor["contexts"]
	if !ok {
		if blockName == "core/content-slot" {
			return []string{EditorModeLayoutTemplate}
		}
		return []string{EditorModeEntry, EditorModeLayoutTemplate}
	}
	switch v := val.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return []string{EditorModeEntry, EditorModeLayoutTemplate}
		}
		return out
	case []string:
		return v
	default:
		return []string{EditorModeEntry, EditorModeLayoutTemplate}
	}
}

func parseHidden(schemaJSON string) bool {
	var raw map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &raw); err != nil {
		return false
	}
	editor, ok := raw["editor"].(map[string]any)
	if !ok {
		return false
	}
	if hidden, ok := editor["hidden"].(bool); ok {
		return hidden
	}
	return false
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

func parseLCPCapability(blockName, schemaJSON string) (candidate bool, requiresFeatured bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &raw); err != nil {
		raw = nil
	}
	if raw != nil {
		if caps, ok := raw["capabilities"].(map[string]any); ok {
			if v, ok := caps["lcpCandidate"].(bool); ok && v {
				req, _ := caps["requiresFeatured"].(bool)
				return true, req
			}
		}
		if editor, ok := raw["editor"].(map[string]any); ok {
			if v, ok := editor["lcpCandidate"].(bool); ok && v {
				req, _ := editor["requiresFeatured"].(bool)
				return true, req
			}
		}
	}
	switch blockName {
	case "core/image":
		return true, false
	case "core/featured-image":
		return true, true
	default:
		return false, false
	}
}

func parseSummaryFields(schemaJSON string) []string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &raw); err != nil {
		return nil
	}
	editor, ok := raw["editor"].(map[string]any)
	if !ok {
		return nil
	}
	val, ok := editor["summaryFields"]
	if !ok {
		return nil
	}
	switch v := val.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
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
