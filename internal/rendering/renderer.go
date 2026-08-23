package rendering

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/kokosx/stratum/internal/document"
)

// Definition is the rendering information for one versioned block.
// Definitions are loaded by the web layer from the database.
type Definition struct {
	Namespace    string
	Name         string
	Version      int64
	RendererType string
	Template     string
}

type Renderer struct {
	blocks        map[blockKey]*template.Template
	mediaProvider MediaProvider
	runtimes      map[blockKey]RuntimeRenderer
}

// RuntimeRenderer is the extensibility boundary for blocks that need to render
// children multiple times with different contexts (e.g. Collection). Normal
// blocks use the template path. Future WASM blocks will implement the same
// interface without receiving *sql.DB or sqlc handles – they use host capabilities
// like ContentReader.
type RuntimeRenderer interface {
	Render(ctx context.Context, node PreparedNode, rc RenderContext, r *Renderer) (template.HTML, error)
}

type blockKey struct {
	name    string
	version int64
}

type blockData struct {
	ID       string
	Props    map[string]any
	Settings map[string]any
	Children template.HTML
	Context  RenderContext
	Priority bool
}

// LCPState is request-scoped mutable state shared across all scoped
// RenderContext copies. Exactly one runtime instance of an LCP candidate
// gets Priority=true; the first that actually renders with a real image
// claims it and records the preload that the head should emit.
type LCPState struct {
	Consumed      bool
	PreloadHref   string
	PreloadSrcSet string
	PreloadSizes  string
	PreloadView   MediaView
}

// RenderContext carries request-time data that dynamic blocks bind to (the
// current Entry and Site settings). It is the same for every node in a document.
// In the editor preview it is empty, so dynamic blocks fall back to placeholders.
type RenderContext struct {
	Site    SiteContext
	Entry   EntryContext
	Archive *ArchiveContext
	// Collections and ArchiveURL are legacy: core/posts latest mode used them.
	// New Collection blocks use Route.Archive and ContentReader. Deprecated.
	Collections map[string][]ArchiveEntry
	ArchiveURL  string // legacy URL of the post archive
	LCPNodeID   string
	// LCPConsumed is a request-scoped flag shared by all scoped copies so the
	// Priority claim is consumed exactly once even when a Collection renders the
	// same node ID for multiple entries.
	LCPConsumed *bool
	// LCP is the unified request-scoped state for the current render. When
	// non-nil it is shared across all scoped copies (Collection per-entry).
	LCP       *LCPState
	IsPreview bool   // true in editor preview; public renders are false
	EntryID   string // current entry ID for current-post exclusion in latest blocks

	// Route is the generic route scope for the current render. Archive blocks
	// and Collection(source=context) read from Route.Archive when present.
	Route RouteContext
	// Mode indicates the render pipeline: public vs preview.
	Mode RenderMode
	// ContentReader is the request-scoped, memoised reader for Collection queries.
	// Nil in editor preview; public handler injects a ContentReader backed by the
	// stable EntryQuery shapes.
	ContentReader ContentReader
	// QueryCache is the request-local memo for identical EntryQuery shapes. It
	// is populated by the Collection block at render time so duplicate queries
	// in one request hit the cache.
	QueryCache map[string][]ArchiveEntry
}

// RouteContext is the generic route scope for the current request.
type RouteContext struct {
	Path        string
	IsArchive   bool
	ContentType string
	Archive     *ArchiveContext
	Pagination  PaginationContext
}

// RenderMode distinguishes public vs preview rendering.
type RenderMode string

const (
	ModePublic  RenderMode = "public"
	ModePreview RenderMode = "preview"
)

// ContentReader is the host capability for Collection blocks. It is the only
// way a block may read published entries; arbitrary SQL is never exposed.
type ContentReader interface {
	Query(ctx context.Context, contentType string, limit, offset int, order string, excludeIDs []string) ([]ArchiveEntry, error)
}

// WithEntry returns a shallow copy of rc with Entry scoped to the given archive entry.
// This is used by Collection to render its child blocks per-item.
func (rc RenderContext) WithEntry(ae ArchiveEntry) RenderContext {
	// Derive EntryContext from ArchiveEntry so EntryTitle etc work in any scope.
	rc.Entry = EntryContext{
		Title:         ae.Title,
		Excerpt:       ae.Excerpt,
		Permalink:     ae.URL,
		PublishDate:   ae.PublishedAt,
		PublishISO:    ae.PublishedISO,
		FeaturedImage: ae.FeaturedImage.ID,
	}
	rc.EntryID = ae.ID
	return rc
}

// WithRoute returns a copy with Route replaced.
func (rc RenderContext) WithRoute(route RouteContext) RenderContext {
	rc.Route = route
	return rc
}

// BlockRenderInput is the future lazy-children API. Parent blocks receive a
// RenderChildren function that may be called multiple times with different
// RenderContexts. Normal containers call it once; Collection calls it per entry.
type BlockRenderInput struct {
	Context        RenderContext
	Node           PreparedNode
	RenderChildren func(RenderContext) (template.HTML, error)
}

// ArchiveContext is the typed archive listing supplied to core/posts source=archive.
// Nil on single renders; non-nil on archive renders.
type ArchiveContext struct {
	Entries    []ArchiveEntry
	Pagination PaginationContext
	Permalink  string // canonical archive path for this page (e.g. "/blog" or "/blog/page/2")
}

// ArchiveEntry is one post card, already resolved via routes.
type ArchiveEntry struct {
	ID            string
	Title         string
	Excerpt       string
	URL           string
	PublishedAt   string
	PublishedISO  string
	FeaturedImage MediaView
}

// PaginationContext carries pagination state pre-built from site settings.
type PaginationContext struct {
	Current     int
	TotalPages  int
	TotalItems  int64
	PreviousURL string
	NextURL     string
}

// SiteSocialLink is a single configured social profile surfaced by the Social
// Links block. It is populated from Site Settings at render time.
type SiteSocialLink struct {
	Platform string
	URL      string
	Label    string
}

type SiteContext struct {
	Name    string
	Tagline string
	URL     string
	LogoURL string
	// LogoWidth/LogoHeight carry the logo asset's intrinsic dimensions when
	// known, so theme and logo-block <img> tags can reserve layout space
	// (no CLS). Zero means unknown; templates omit the attributes.
	LogoWidth   int
	LogoHeight  int
	SocialLinks []SiteSocialLink
}

type EntryContext struct {
	Title         string
	Excerpt       string
	Permalink     string
	PublishDate   string
	PublishISO    string
	FeaturedImage string
}

// NewRenderer validates and compiles enabled block templates from the database.
// provider may be nil; when nil the media template function returns an empty view
// so documents without media (and tests) keep working.
func NewRenderer(definitions []Definition, provider MediaProvider) (*Renderer, error) {
	renderer := &Renderer{blocks: make(map[blockKey]*template.Template, len(definitions)), mediaProvider: provider}

	mediaFunc := func(id any) MediaView {
		if renderer.mediaProvider == nil {
			return MediaView{}
		}
		str, ok := id.(string)
		if !ok || str == "" {
			return MediaView{}
		}
		view, ok := renderer.mediaProvider.MediaView(context.Background(), str)
		if !ok {
			return MediaView{}
		}
		return view
	}

	for _, definition := range definitions {
		// Template type is the default. Runtime blocks (e.g. core/collection) are
		// template-backed but also have a RuntimeRenderer registered separately.
		// Future WASM blocks will use renderer_type = "wasm" and a WASM runtime.
		if definition.RendererType != "template" && definition.RendererType != "runtime" && definition.RendererType != "wasm" {
			return nil, fmt.Errorf("block %s/%s@%d: unsupported renderer type %q", definition.Namespace, definition.Name, definition.Version, definition.RendererType)
		}
		if definition.Template == "" && definition.RendererType == "template" {
			return nil, fmt.Errorf("block %s/%s@%d: template is required", definition.Namespace, definition.Name, definition.Version)
		}

		key := blockKey{name: definition.Namespace + "/" + definition.Name, version: definition.Version}
		if _, exists := renderer.blocks[key]; exists {
			return nil, fmt.Errorf("duplicate block definition: %s@%d", key.name, key.version)
		}

		if definition.Template != "" {
			tmpl, err := template.New(key.name).Funcs(template.FuncMap{
				"integerEquals": integerEquals,
				"media":         mediaFunc,
				"icon":          iconFunc,
				"lines":         linesFunc,
				"split":         splitFunc,
				"youtubeID":     youtubeIDFunc,
				"vimeoID":       vimeoIDFunc,
				"tagFor":        tagForFunc,
				"tagOpen":       tagOpenFunc,
				"tagClose":      tagCloseFunc,
			}).Parse(definition.Template)
			if err != nil {
				return nil, fmt.Errorf("parse block %s@%d template: %w", key.name, key.version, err)
			}
			renderer.blocks[key] = tmpl
		}
	}

	// Register runtime renderers via a table. Adding a new dynamic block only
	// requires adding an entry here (or via RegisterRuntime), not modifying
	// renderPreparedNode's hot path. This satisfies the Block invariant:
	// generic rendering does not branch on concrete block names per-node.
	renderer.runtimes = make(map[blockKey]RuntimeRenderer)
	for key := range renderer.blocks {
		if key.name == "core/collection" {
			renderer.runtimes[key] = &collectionRenderer{}
		}
		// Future: if key.name == "core/other-dynamic" { renderer.runtimes[key]=... }
	}

	return renderer, nil
}

// RegisterRuntime allows external callers (e.g. tests or future plugin loader)
// to register a runtime renderer without modifying this file. This is the
// extensibility boundary for WASM/plugin renderers.
func (r *Renderer) RegisterRuntime(name string, version int64, rr RuntimeRenderer) {
	if r.runtimes == nil {
		r.runtimes = make(map[blockKey]RuntimeRenderer)
	}
	r.runtimes[blockKey{name: name, version: version}] = rr
}

func integerEquals(value any, expected int) bool {
	switch number := value.(type) {
	case float64:
		return number == float64(expected)
	case json.Number:
		integer, err := number.Int64()
		return err == nil && integer == int64(expected)
	case int:
		return number == expected
	case int64:
		return number == int64(expected)
	default:
		return false
	}
}

func (r *Renderer) RenderDocument(doc *document.Document) (template.HTML, error) {
	return r.RenderDocumentContext(doc, RenderContext{})
}

func (r *Renderer) RenderDocumentContext(doc *document.Document, rc RenderContext) (template.HTML, error) {
	// Ensure raw and prepared have identical Collection semantics. Instead of
	// maintaining two renderers, convert the document to a PreparedDocument
	// and reuse the prepared path which already handles RuntimeRenderer
	// dispatch, LCP and per-entry scoping identically.
	pd, err := r.prepareFromDocument(doc)
	if err != nil {
		return "", err
	}
	return r.RenderPreparedDocumentContext(context.Background(), pd, rc)
}

func (r *Renderer) prepareFromDocument(doc *document.Document) (*PreparedDocument, error) {
	if doc == nil {
		return &PreparedDocument{}, nil
	}
	nodes := make([]PreparedNode, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		pn, err := r.documentNodeToPrepared(n)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, pn)
	}
	pd := &PreparedDocument{Nodes: nodes}
	// Populate LCP candidates for raw path so raw vs prepared have identical
	// semantics. Use the same heuristic as blocks.Registry: image/featured-image
	// candidates gated by decorative/priority.
	var high, auto []LCPCandidate
	var visit func([]PreparedNode)
	visit = func(items []PreparedNode) {
		for _, n := range items {
			isCandidate := false
			requiresFeatured := false
			switch n.Block {
			case "core/image":
				if decorative, _ := n.Settings["decorative"].(bool); decorative {
					isCandidate = false
				} else if mediaID, _ := n.Props["mediaId"].(string); mediaID != "" {
					if prio, _ := n.Settings["priority"].(string); prio == "normal" {
						isCandidate = false
					} else {
						isCandidate = true
					}
				}
			case "core/featured-image":
				if decorative, _ := n.Settings["decorative"].(bool); decorative {
					isCandidate = false
				} else {
					if prio, _ := n.Settings["priority"].(string); prio == "normal" {
						isCandidate = false
					} else {
						isCandidate = true
						requiresFeatured = true
					}
				}
			}
			if isCandidate {
				priority, _ := n.Settings["priority"].(string)
				if priority == "auto" || priority == "" {
					if eager, _ := n.Settings["eager"].(bool); eager {
						priority = "high"
					} else if priority == "" {
						priority = "auto"
					}
				}
				cand := LCPCandidate{ID: n.ID, Block: n.Block, RequiresFeatured: requiresFeatured}
				if priority == "high" {
					high = append(high, cand)
				} else if priority == "auto" {
					auto = append(auto, cand)
				}
			}
			visit(n.Children)
		}
	}
	visit(nodes)
	// Also account for candidates inside Collection children (they are already
	// visited via traversal, so per-entry expansion not needed here; winner
	// selection will handle per-entry existence).
	pd.HighPriority = high
	pd.AutoCandidates = auto
	return pd, nil
}

func (r *Renderer) documentNodeToPrepared(node document.Node) (PreparedNode, error) {
	props, err := decodeObject(node.Props, "props")
	if err != nil {
		return PreparedNode{}, err
	}
	settings, err := decodeObject(node.Settings, "settings")
	if err != nil {
		return PreparedNode{}, err
	}
	children := make([]PreparedNode, 0, len(node.Children))
	for _, ch := range node.Children {
		pch, err := r.documentNodeToPrepared(ch)
		if err != nil {
			return PreparedNode{}, err
		}
		children = append(children, pch)
	}
	// Preserve LegacySource if the raw document already contains a migrated
	// collection? For raw tests that use core/posts@1 we mimic the Prepare
	// migration: if node is core/posts@1, treat as legacy collection.
	legacy := ""
	if node.Block == "core/posts" && node.Version == 1 {
		legacy = "core/posts@1"
		// Normalize to collection for rendering equivalence.
		return PreparedNode{
			ID:           node.ID,
			Block:        "core/collection",
			Version:      1,
			Props:        props,
			Settings:     settings,
			Children:     children,
			LegacySource: legacy,
		}, nil
	}
	return PreparedNode{
		ID:       node.ID,
		Block:    node.Block,
		Version:  node.Version,
		Props:    props,
		Settings: settings,
		Children: children,
	}, nil
}

func (r *Renderer) renderNode(ctx context.Context, node document.Node, rc RenderContext) (template.HTML, error) {
	key := blockKey{name: node.Block, version: int64(node.Version)}
	tmpl, ok := r.blocks[key]
	if !ok {
		return "", fmt.Errorf("block definition not found: %s@%d", node.Block, node.Version)
	}

	props, err := decodeObject(node.Props, "props")
	if err != nil {
		return "", err
	}
	settings, err := decodeObject(node.Settings, "settings")
	if err != nil {
		return "", err
	}

	var children strings.Builder
	for _, child := range node.Children {
		rendered, err := r.renderNode(ctx, child, rc)
		if err != nil {
			return "", err
		}
		children.WriteString(string(rendered))
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, blockData{ID: node.ID, Props: props, Settings: settings, Children: template.HTML(children.String()), Context: rc}); err != nil {
		return "", fmt.Errorf("render block %s@%d: %w", node.Block, node.Version, err)
	}
	return template.HTML(out.String()), nil
}

// RenderPreparedDocumentContext renders a PreparedDocument without any JSON
// decoding or defaults processing. It is the fast path used for published pages
// after the document has been prepared once and cached.
func (r *Renderer) RenderPreparedDocumentContext(ctx context.Context, pd *PreparedDocument, rc RenderContext) (template.HTML, error) {
	// Initialise shared LCP state if caller did not. Registry.RenderPrepared
	// normally does this, but direct callers (tests) may not.
	if rc.LCP == nil {
		rc.LCP = &LCPState{}
	}
	if rc.LCPConsumed == nil {
		rc.LCPConsumed = &rc.LCP.Consumed
	} else {
		// Keep both in sync: LCP.Consumed is the source of truth.
		rc.LCP.Consumed = *rc.LCPConsumed
		rc.LCPConsumed = &rc.LCP.Consumed
	}
	// Resolve LCP winner if not already set. The winner is the first
	// High candidate that has a real image (per-entry for collection),
	// else first Auto. This is the single policy used by both renderer
	// and preload; handler must not duplicate it.
	if rc.LCPNodeID == "" && (len(pd.HighPriority) > 0 || len(pd.AutoCandidates) > 0) {
		if winner := r.findLCPWinner(ctx, pd, rc); winner != "" {
			rc.LCPNodeID = winner
		}
	}
	var out strings.Builder
	for i := range pd.Nodes {
		rendered, err := r.renderPreparedNode(ctx, pd.Nodes[i], rc)
		if err != nil {
			return "", err
		}
		out.WriteString(string(rendered))
	}
	return template.HTML(out.String()), nil
}

func (r *Renderer) renderPreparedNode(ctx context.Context, node PreparedNode, rc RenderContext) (template.HTML, error) {
	key := blockKey{name: node.Block, version: int64(node.Version)}
	tmpl, ok := r.blocks[key]
	if !ok && r.runtimes[key] == nil {
		return "", fmt.Errorf("block definition not found: %s@%d", node.Block, node.Version)
	}
	// Runtime blocks (Collection, future WASM) are dispatched via the registry
	// table, not a per-node if-branch on concrete names. Adding a new runtime
	// block only requires RegisterRuntime, not editing this function.
	if rr, ok := r.runtimes[key]; ok {
		return rr.Render(ctx, node, rc, r)
	}

	var children strings.Builder
	for i := range node.Children {
		rendered, err := r.renderPreparedNode(ctx, node.Children[i], rc)
		if err != nil {
			return "", err
		}
		children.WriteString(string(rendered))
	}

	priority := false
	if node.ID == rc.LCPNodeID && (rc.LCPConsumed == nil || !*rc.LCPConsumed) && (rc.LCP == nil || !rc.LCP.Consumed) {
		// Only claim if this actual instance has a real image. For a
		// Collection-embedded featured-image the first entry may be a
		// placeholder while the second has media; the second should win.
		if view, ok := r.hasActualImage(node, rc); ok {
			priority = true
			// log.Printf("CLAIM node=%s viewSrc=%q LCP=%p consumed=%v", node.ID, view.Src, rc.LCP, rc.LCP.Consumed)
			if rc.LCP != nil {
				rc.LCP.Consumed = true
				rc.LCP.PreloadHref = view.Src
				rc.LCP.PreloadSrcSet = view.SrcSet
				rc.LCP.PreloadView = view
				if s, _ := node.Settings["sizes"].(string); s != "" {
					rc.LCP.PreloadSizes = s
				} else {
					rc.LCP.PreloadSizes = "(min-width: 768px) min(100vw, 1200px), 100vw"
				}
				// log.Printf("SET preload href=%q", view.Src)
			}
			if rc.LCPConsumed != nil {
				*rc.LCPConsumed = true
			}
		} else {
			// log.Printf("NOIMAGE node=%s LCPNodeID=%s", node.ID, rc.LCPNodeID)
		}
	} else if node.ID == rc.LCPNodeID {
		// log.Printf("SKIP consumed node=%s", node.ID)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, blockData{ID: node.ID, Props: node.Props, Settings: node.Settings, Children: template.HTML(children.String()), Context: rc, Priority: priority}); err != nil {
		return "", fmt.Errorf("render block %s@%d: %w", node.Block, node.Version, err)
	}
	return template.HTML(out.String()), nil
}

func (r *Renderer) renderPreparedNodes(ctx context.Context, nodes []PreparedNode, rc RenderContext) (template.HTML, error) {
	var out strings.Builder
	for i := range nodes {
		rendered, err := r.renderPreparedNode(ctx, nodes[i], rc)
		if err != nil {
			return "", err
		}
		out.WriteString(string(rendered))
	}
	return template.HTML(out.String()), nil
}

type collectionRenderer struct{}

func (c *collectionRenderer) Render(ctx context.Context, node PreparedNode, rc RenderContext, r *Renderer) (template.HTML, error) {
	tmpl := r.blocks[blockKey{name: node.Block, version: int64(node.Version)}]
	return r.renderCollectionNode(ctx, node, rc, tmpl)
}

func (r *Renderer) renderCollectionNode(ctx context.Context, node PreparedNode, rc RenderContext, tmpl *template.Template) (template.HTML, error) {
	source, _ := node.Settings["source"].(string)
	if source == "" {
		source = "query"
	}
	var entries []ArchiveEntry
	switch source {
	case "context":
		if rc.Route.Archive != nil {
			entries = rc.Route.Archive.Entries
		} else if rc.Archive != nil {
			entries = rc.Archive.Entries
		}
	case "query":
		fallthrough
	default:
		contentType, _ := node.Settings["contentType"].(string)
		if contentType == "" {
			contentType, _ = node.Settings["content_type"].(string)
		}
		if contentType == "" {
			contentType = "post"
		}
		limit := intFromSettings(node.Settings, "limit", 3)
		if limit < 1 {
			limit = 1
		}
		if limit > 20 {
			limit = 20
		}
		offset := intFromSettings(node.Settings, "offset", 0)
		order, _ := node.Settings["order"].(string)
		if order == "" {
			order = "published_desc"
		}
		excludeCurrent, _ := node.Settings["excludeCurrent"].(bool)
		var excludeIDs []string
		if excludeCurrent && rc.EntryID != "" {
			excludeIDs = append(excludeIDs, rc.EntryID)
		}
		// Request memo: canonical key is contentType|limit|offset|order|exclude
		cacheKey := collectionCacheKey(contentType, limit, offset, order, excludeIDs)
		if rc.QueryCache != nil {
			if cached, ok := rc.QueryCache[cacheKey]; ok {
				entries = cached
			} else if rc.ContentReader != nil {
				fetched, err := rc.ContentReader.Query(ctx, contentType, limit, offset, order, excludeIDs)
				if err != nil {
					return "", fmt.Errorf("collection %s: %w", node.ID, err)
				}
				entries = fetched
				rc.QueryCache[cacheKey] = entries
			} else {
				// Fallback to legacy Collections map (populated by public handler for old docs)
				if col, ok := rc.Collections[node.ID]; ok {
					entries = col
					if len(entries) > limit {
						entries = entries[:limit]
					}
				}
			}
		} else if rc.ContentReader != nil {
			fetched, err := rc.ContentReader.Query(ctx, contentType, limit, offset, order, excludeIDs)
			if err != nil {
				return "", fmt.Errorf("collection %s: %w", node.ID, err)
			}
			entries = fetched
		}
		// Pagination handling for collection when source=query and pagination flag true?
		// Pagination is handled at route level for archive (source=context). For
		// source=query we treat limit/offset as the query; no pagination UI here.
	}

	// Render children per entry with scoped Entry context (lazy children invariant).
	var children strings.Builder
	isLegacy := node.LegacySource != ""
	if isLegacy && len(node.Children) == 0 && len(entries) > 0 {
		children.WriteString(string(renderLegacyCollectionFallback(node, entries, rc)))
	} else {
		for _, entry := range entries {
			scoped := rc.WithEntry(entry)
			scoped.Route = rc.Route
			scoped.QueryCache = rc.QueryCache
			scoped.ContentReader = rc.ContentReader
			rendered, err := r.renderPreparedNodes(ctx, node.Children, scoped)
			if err != nil {
				return "", err
			}
			children.WriteString(string(rendered))
		}
		if len(entries) == 0 && len(node.Children) == 0 {
			if isLegacy {
				// Legacy empty without entries keeps historic empty wording.
				children.WriteString(`<div class="stratum-posts-empty"><p>No posts found.</p></div>`)
			} else {
				children.WriteString(`<div class="stratum-posts-empty"><p>No posts found.</p></div>`)
			}
		}
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, blockData{ID: node.ID, Props: node.Props, Settings: node.Settings, Children: template.HTML(children.String()), Context: rc, Priority: false}); err != nil {
		return "", fmt.Errorf("render block %s@%d: %w", node.Block, node.Version, err)
	}
	return template.HTML(out.String()), nil
}

func intFromSettings(m map[string]any, key string, def int) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case int64:
			return int(val)
		case json.Number:
			if i, err := val.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return def
}

func collectionCacheKey(contentType string, limit, offset int, order string, excludeIDs []string) string {
	key := fmt.Sprintf("%s|%d|%d|%s|", contentType, limit, offset, order)
	for i, id := range excludeIDs {
		if i > 0 {
			key += ","
		}
		key += id
	}
	return key
}

// renderLegacyCollectionFallback renders a backwards-compatible card list for
// migrated core/posts@1 documents that have no children. New documents should
// provide explicit children (EntryTitle etc) and will not hit this path.
// It preserves historic layout/columns/viewAll semantics so old published content
// does not change appearance without an explicit migration.
func renderLegacyCollectionFallback(node PreparedNode, entries []ArchiveEntry, rc RenderContext) template.HTML {
	showImage := true
	if v, ok := node.Settings["showImage"]; ok {
		if b, ok := v.(bool); ok {
			showImage = b
		}
	}
	showDate := true
	if v, ok := node.Settings["showDate"]; ok {
		if b, ok := v.(bool); ok {
			showDate = b
		}
	}
	showExcerpt := true
	if v, ok := node.Settings["showExcerpt"]; ok {
		if b, ok := v.(bool); ok {
			showExcerpt = b
		}
	}
	pagination := true
	if v, ok := node.Settings["pagination"]; ok {
		if b, ok := v.(bool); ok {
			pagination = b
		}
	}
	showViewAll := false
	if v, ok := node.Settings["showViewAll"]; ok {
		if b, ok := v.(bool); ok {
			showViewAll = b
		}
	}
	viewAllLabel, _ := node.Settings["viewAllLabel"].(string)
	if viewAllLabel == "" {
		viewAllLabel = "View all posts"
	}
	layout, _ := node.Settings["layout"].(string)
	if layout == "" {
		layout = "list"
	}
	columns := intFromSettings(node.Settings, "columns", 3)
	if columns < 1 {
		columns = 1
	}
	if columns > 4 {
		columns = 4
	}
	source, _ := node.Settings["source"].(string)
	var b strings.Builder
	cls := "stratum-posts stratum-posts--list"
	if layout == "grid" {
		cls = fmt.Sprintf("stratum-posts stratum-posts--grid stratum-posts--cols-%d", columns)
	}
	// Sizes follows the historic core/posts template: list uses a fixed 280px bucket,
	// grid varies by columns. This preserves byte-level visual compatibility.
	sizes := "(min-width: 768px) 280px, 100vw"
	if layout == "grid" {
		switch columns {
		case 2:
			sizes = "(min-width: 768px) 50vw, 100vw"
		case 3:
			sizes = "(min-width: 768px) 33vw, 100vw"
		default:
			sizes = "(min-width: 768px) min(720px, 100vw), 100vw"
		}
	} else {
		sizes = "(min-width: 768px) min(720px, 100vw), 100vw"
	}
	b.WriteString(`<section class="` + cls + `">`)
	for _, e := range entries {
		b.WriteString(`<article class="stratum-post-card">`)
		if showImage && e.FeaturedImage.Src != "" {
			b.WriteString(`<figure class="stratum-post-card__media"><img src="` + template.HTMLEscapeString(e.FeaturedImage.Src) + `"`)
			if e.FeaturedImage.SrcSet != "" {
				b.WriteString(` srcset="` + template.HTMLEscapeString(e.FeaturedImage.SrcSet) + `" sizes="` + template.HTMLEscapeString(sizes) + `"`)
			}
			if e.FeaturedImage.Width != 0 {
				b.WriteString(` width="` + template.HTMLEscapeString(fmt.Sprint(e.FeaturedImage.Width)) + `"`)
			}
			if e.FeaturedImage.Height != 0 {
				b.WriteString(` height="` + template.HTMLEscapeString(fmt.Sprint(e.FeaturedImage.Height)) + `"`)
			}
			b.WriteString(` alt="` + template.HTMLEscapeString(e.FeaturedImage.Alt) + `" loading="lazy" decoding="async"></figure>`)
		}
		b.WriteString(`<header class="stratum-post-card__header"><h2 class="stratum-post-card__title"><a href="` + template.HTMLEscapeString(e.URL) + `">` + template.HTMLEscapeString(e.Title) + `</a></h2>`)
		if showDate && e.PublishedISO != "" {
			b.WriteString(`<time class="stratum-post-card__date" datetime="` + template.HTMLEscapeString(e.PublishedISO) + `">` + template.HTMLEscapeString(e.PublishedAt) + `</time>`)
		}
		b.WriteString(`</header>`)
		if showExcerpt && e.Excerpt != "" {
			b.WriteString(`<p class="stratum-post-card__excerpt">` + template.HTMLEscapeString(e.Excerpt) + `</p>`)
		}
		b.WriteString(`</article>`)
	}
	b.WriteString(`</section>`)
	// Pagination: only for source=context (archive) when pagination enabled and we have archive pagination.
	if pagination && source == "context" {
		var pag PaginationContext
		hasPag := false
		if rc.Route.Archive != nil {
			pag = rc.Route.Archive.Pagination
			hasPag = true
		} else if rc.Archive != nil {
			pag = rc.Archive.Pagination
			hasPag = true
		}
		if hasPag && pag.TotalPages > 1 {
			b.WriteString(`<nav aria-label="Pagination" class="stratum-pagination">`)
			if pag.PreviousURL != "" {
				b.WriteString(`<a href="` + template.HTMLEscapeString(pag.PreviousURL) + `" rel="prev">Previous</a>`)
			}
			b.WriteString(`<span>Page ` + template.HTMLEscapeString(fmt.Sprint(pag.Current)) + ` of ` + template.HTMLEscapeString(fmt.Sprint(pag.TotalPages)) + `</span>`)
			if pag.NextURL != "" {
				b.WriteString(`<a href="` + template.HTMLEscapeString(pag.NextURL) + `" rel="next">Next</a>`)
			}
			b.WriteString(`</nav>`)
		}
	}
	if showViewAll && source == "query" {
		var archiveURL string
		if rc.Route.Archive != nil && rc.Route.Archive.Permalink != "" {
			archiveURL = rc.Route.Archive.Permalink
		} else if rc.Archive != nil && rc.Archive.Permalink != "" {
			archiveURL = rc.Archive.Permalink
		} else if rc.ArchiveURL != "" {
			archiveURL = rc.ArchiveURL
		} else if rc.Route.Path != "" {
			archiveURL = rc.Route.Path
		} else {
			archiveURL = "/blog"
		}
		if archiveURL != "" {
			b.WriteString(`<p class="stratum-posts-view-all"><a href="` + template.HTMLEscapeString(archiveURL) + `">` + template.HTMLEscapeString(viewAllLabel) + `</a></p>`)
		}
	}
	return template.HTML(b.String())
}

func (r *Renderer) hasActualImage(node PreparedNode, rc RenderContext) (MediaView, bool) {
	switch node.Block {
	case "core/image":
		id, _ := node.Props["mediaId"].(string)
		if id == "" {
			return MediaView{}, false
		}
		if decorative, _ := node.Settings["decorative"].(bool); decorative {
			return MediaView{}, false
		}
		if prio, _ := node.Settings["priority"].(string); prio == "normal" {
			return MediaView{}, false
		}
		if r.mediaProvider == nil {
			// No provider (e.g. in tests where registry was built without media).
			// Assume image exists so LCP selection still works; fabricate a view
			// for preload consistency.
			return MediaView{ID: id, Src: "/media/" + id + "/original", Alt: ""}, true
		}
		view, ok := r.mediaProvider.MediaView(context.Background(), id)
		if !ok || view.Src == "" {
			return MediaView{}, false
		}
		return view, true
	case "core/featured-image":
		if rc.Entry.FeaturedImage == "" {
			return MediaView{}, false
		}
		if decorative, _ := node.Settings["decorative"].(bool); decorative {
			return MediaView{}, false
		}
		if prio, _ := node.Settings["priority"].(string); prio == "normal" {
			return MediaView{}, false
		}
		if r.mediaProvider == nil {
			return MediaView{ID: rc.Entry.FeaturedImage, Src: "/media/" + rc.Entry.FeaturedImage + "/original", Alt: ""}, true
		}
		view, ok := r.mediaProvider.MediaView(context.Background(), rc.Entry.FeaturedImage)
		if !ok || view.Src == "" {
			return MediaView{}, false
		}
		return view, true
	default:
		return MediaView{}, false
	}
}

func (r *Renderer) findLCPWinner(ctx context.Context, pd *PreparedDocument, rc RenderContext) string {
	for _, c := range pd.HighPriority {
		if r.candidateHasImage(ctx, c, pd, rc) {
			return c.ID
		}
	}
	for _, c := range pd.AutoCandidates {
		if r.candidateHasImage(ctx, c, pd, rc) {
			return c.ID
		}
	}
	return ""
}

func (r *Renderer) candidateHasImage(ctx context.Context, cand LCPCandidate, pd *PreparedDocument, rc RenderContext) bool {
	node, parentCollection := r.findCandidateLocation(pd.Nodes, cand.ID, nil)
	if node == nil {
		return false
	}
	if parentCollection == nil {
		_, ok := r.hasActualImage(*node, rc)
		return ok
	}
	entries, err := r.collectionEntriesForWinner(ctx, *parentCollection, rc)
	if err != nil || len(entries) == 0 {
		// No entries to check; treat as no image.
		// For image blocks with static mediaId we still check without entry.
		if node.Block == "core/image" {
			_, ok := r.hasActualImage(*node, rc)
			return ok
		}
		return false
	}
	for _, e := range entries {
		scoped := rc.WithEntry(e)
		scoped.Route = rc.Route
		scoped.QueryCache = rc.QueryCache
		scoped.ContentReader = rc.ContentReader
		if _, ok := r.hasActualImage(*node, scoped); ok {
			return true
		}
	}
	return false
}

func (r *Renderer) findCandidateLocation(nodes []PreparedNode, targetID string, parentCollection *PreparedNode) (*PreparedNode, *PreparedNode) {
	for i := range nodes {
		n := &nodes[i]
		isCollection := n.Block == "core/collection"
		nextParent := parentCollection
		if isCollection {
			nextParent = n
		}
		if n.ID == targetID {
			return n, parentCollection
		}
		if len(n.Children) > 0 {
			if found, parent := r.findCandidateLocation(n.Children, targetID, nextParent); found != nil {
				return found, parent
			}
		}
	}
	return nil, nil
}

func (r *Renderer) collectionEntriesForWinner(ctx context.Context, colNode PreparedNode, rc RenderContext) ([]ArchiveEntry, error) {
	source, _ := colNode.Settings["source"].(string)
	if source == "" {
		source = "query"
	}
	var entries []ArchiveEntry
	switch source {
	case "context":
		if rc.Route.Archive != nil {
			entries = rc.Route.Archive.Entries
		} else if rc.Archive != nil {
			entries = rc.Archive.Entries
		}
	case "query":
		fallthrough
	default:
		contentType, _ := colNode.Settings["contentType"].(string)
		if contentType == "" {
			contentType, _ = colNode.Settings["content_type"].(string)
		}
		if contentType == "" {
			contentType = "post"
		}
		limit := intFromSettings(colNode.Settings, "limit", 3)
		if limit < 1 {
			limit = 1
		}
		if limit > 20 {
			limit = 20
		}
		offset := intFromSettings(colNode.Settings, "offset", 0)
		order, _ := colNode.Settings["order"].(string)
		if order == "" {
			order = "published_desc"
		}
		excludeCurrent, _ := colNode.Settings["excludeCurrent"].(bool)
		var excludeIDs []string
		if excludeCurrent && rc.EntryID != "" {
			excludeIDs = append(excludeIDs, rc.EntryID)
		}
		cacheKey := collectionCacheKey(contentType, limit, offset, order, excludeIDs)
		if rc.QueryCache != nil {
			if cached, ok := rc.QueryCache[cacheKey]; ok {
				entries = cached
			} else if rc.ContentReader != nil {
				fetched, err := rc.ContentReader.Query(ctx, contentType, limit, offset, order, excludeIDs)
				if err != nil {
					return nil, err
				}
				entries = fetched
				rc.QueryCache[cacheKey] = entries
			} else if col, ok := rc.Collections[colNode.ID]; ok {
				entries = col
				if len(entries) > limit {
					entries = entries[:limit]
				}
			}
		} else if rc.ContentReader != nil {
			fetched, err := rc.ContentReader.Query(ctx, contentType, limit, offset, order, excludeIDs)
			if err != nil {
				return nil, err
			}
			entries = fetched
		}
	}
	return entries, nil
}

func decodeObject(value json.RawMessage, name string) (map[string]any, error) {
	if len(value) == 0 {
		return map[string]any{}, nil
	}

	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil {
		return nil, fmt.Errorf("decode block %s: %w", name, err)
	}
	if object == nil {
		return nil, fmt.Errorf("block %s must be an object", name)
	}
	return object, nil
}
