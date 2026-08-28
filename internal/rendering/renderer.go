package rendering

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/richtext"
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

// CommentView is the public projection of a comment for templates. Email is never included.
type CommentView struct {
	ID         string
	ParentID   string
	AuthorName string
	Body       string
	BodyHTML   template.HTML
	CreatedAt  string
	CreatedISO string
	Depth      int
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

	// Definition is the effective ContentTypeDefinition for the current Entry scope.
	// It is used for typed field resolution (e.g. suppressing media IDs from entry-field).
	// Zero value means unknown (e.g. preview with no content type) – fallback to untyped.
	Definition content.ContentTypeDefinition

	// Comments is the approved comment tree for the current entry (public only).
	Comments        []CommentView
	CommentsCount   int
	CommentsEnabled bool
	CommentsEntryID string

	// SitePartReader is the host capability for core/site-part blocks.
	SitePartReader SitePartReader
	// SitePartStack tracks visited site part IDs to prevent cycles.
	SitePartStack map[string]struct{}
	// SitePartDepth tracks nesting depth.
	SitePartDepth int
	// CollectedSiteParts accumulates site part dependencies for cache tags and used blocks.
	// Map from site part ID to published revision ID.
	CollectedSiteParts map[string]string
	// Dependencies is request-scoped mutable dependency state shared by every
	// shallow RenderContext copy. CollectedSiteParts remains as a compatibility
	// view for older callers.
	Dependencies *DependencyState

	// Navigation carries the site navigation menus for blocks like core/navigation.
	Navigation map[string]navigation.Menu

	// FormReader is the host capability used by core/form. It exposes only the
	// active public projection; notification settings and submissions stay in the domain.
	FormReader FormReader
	FormCache  map[string]forms.FormView
	FormResult FormResultContext
}

type FormReader interface {
	GetActiveForm(context.Context, string) (forms.FormView, bool)
}

type FormResultContext struct {
	SuccessFormID string
}

// DependencyState is the small request-local state shared while rendering a
// document and any nested Site Parts.
type DependencyState struct {
	SiteParts map[string]ResolvedSitePart
}

// ResolvedSitePart carries cache and asset information discovered during the
// render, avoiding a second read of every nested Site Part afterwards.
type ResolvedSitePart struct {
	RevisionID string
	UsedBlocks []BlockKey
}

// RouteContext is the generic route scope for the current request.
type RouteContext struct {
	Path               string
	IsArchive          bool
	ContentType        string
	Archive            *ArchiveContext
	Pagination         PaginationContext
	TaxonomyID         string
	TermID             string
	ArchiveTitle       string
	ArchiveDescription string
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
	Query(ctx context.Context, query content.EntryQuery) ([]ArchiveEntry, error)
}

// DefinitionProvider is an optional host capability for typed field resolution.
type DefinitionProvider interface {
	Definition(ctx context.Context, contentType string) (content.ContentTypeDefinition, error)
}

// SitePartReader is the host capability for site-part blocks.
type SitePartReader interface {
	GetSitePart(ctx context.Context, id string) (*PreparedDocument, string, error)
}

// WithEntry returns a shallow copy of rc with Entry scoped to the given archive entry.
// This is used by Collection to render its child blocks per-item.
func (rc RenderContext) WithEntry(ae ArchiveEntry) RenderContext {
	// Derive EntryContext from ArchiveEntry so EntryTitle etc work in any scope.
	rc.Entry = EntryContext{
		ID:            ae.ID,
		Slug:          ae.Slug,
		ContentTypeID: ae.ContentTypeID,
		Title:         ae.Title,
		Excerpt:       ae.Excerpt,
		Permalink:     ae.URL,
		PublishDate:   ae.PublishedAt,
		PublishISO:    ae.PublishedISO,
		FeaturedImage: ae.FeaturedImage.ID,
		Fields:        ae.Fields,
	}
	rc.EntryID = ae.ID
	// Try to resolve typed definition for the scoped entry via host capability.
	if rc.ContentReader != nil {
		if provider, ok := rc.ContentReader.(DefinitionProvider); ok {
			if def, err := provider.Definition(context.Background(), ae.ContentTypeID); err == nil {
				rc.Definition = def
			} else {
				rc.Definition = content.DefinitionFor(ae.ContentTypeID)
			}
		} else {
			rc.Definition = content.DefinitionFor(ae.ContentTypeID)
		}
	} else {
		rc.Definition = content.DefinitionFor(ae.ContentTypeID)
	}
	return rc
}

// WithRoute returns a copy with Route replaced.
func (rc RenderContext) WithRoute(route RouteContext) RenderContext {
	rc.Route = route
	return rc
}

// ArchiveContext is the typed archive listing supplied to core/posts source=archive.
// Nil on single renders; non-nil on archive renders.
type ArchiveContext struct {
	Entries     []ArchiveEntry
	Pagination  PaginationContext
	Permalink   string // canonical archive path for this page (e.g. "/blog" or "/blog/page/2")
	TaxonomyID  string
	TermID      string
	Title       string
	Description string
}

// ArchiveEntry is one post card, already resolved via routes.
type ArchiveEntry struct {
	ID            string
	Slug          string
	ContentTypeID string
	Title         string
	Excerpt       string
	URL           string
	PublishedAt   string
	PublishedISO  string
	FeaturedImage MediaView
	Fields        map[string]any
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
	ID            string
	Slug          string
	ContentTypeID string
	Title         string
	Excerpt       string
	Permalink     string
	PublishDate   string
	PublishISO    string
	FeaturedImage string
	// Fields is the immutable normalized snapshot from the rendered revision.
	Fields map[string]any
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
				"richText":      richtext.Render,
				"tagClose":      tagCloseFunc,
				"safeURL":       safeURLFunc,
				"anchorID":      anchorIDFunc,
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
		switch key.name {
		case "core/collection":
			renderer.runtimes[key] = &collectionRenderer{}
		case "core/entry-field":
			renderer.runtimes[key] = &entryFieldRenderer{}
		case "core/entry-media":
			renderer.runtimes[key] = &entryMediaRenderer{}
		case "core/site-part":
			renderer.runtimes[key] = &sitePartRenderer{}
		case "core/archive-title":
			renderer.runtimes[key] = &archiveTitleRenderer{}
		case "core/archive-description":
			renderer.runtimes[key] = &archiveDescriptionRenderer{}
		case "core/navigation":
			renderer.runtimes[key] = &navigationRenderer{}
		case "core/form":
			renderer.runtimes[key] = &formRenderer{}
		}
	}

	return renderer, nil
}

var publicFormTemplate = template.Must(template.New("public-form").Parse(`<form id="form-{{ .InstanceID }}" method="post" action="/_stratum/forms/{{ .Form.ID }}" class="stratum-form"><input type="hidden" name="return_to" value="{{ .ReturnTo }}"><div class="stratum-form-honeypot" aria-hidden="true"><label for="form-{{ .InstanceID }}-website">Leave this field empty</label><input id="form-{{ .InstanceID }}-website" name="website_confirm" tabindex="-1" autocomplete="off"></div>{{ range .Form.Fields }}<div class="stratum-form-field stratum-form-field-{{ .Type }}">{{ if eq .Type "checkbox" }}<label for="form-{{ $.InstanceID }}-field-{{ .ID }}"><input id="form-{{ $.InstanceID }}-field-{{ .ID }}" name="{{ .Key }}" type="checkbox" value="1"{{ if .Required }} required{{ end }}> {{ .Label }}</label>{{ else }}<label for="form-{{ $.InstanceID }}-field-{{ .ID }}">{{ .Label }}</label>{{ if eq .Type "textarea" }}<textarea id="form-{{ $.InstanceID }}-field-{{ .ID }}" name="{{ .Key }}" placeholder="{{ .Placeholder }}" maxlength="10000"{{ if .Required }} required{{ end }}></textarea>{{ else if eq .Type "select" }}<select id="form-{{ $.InstanceID }}-field-{{ .ID }}" name="{{ .Key }}"{{ if .Required }} required{{ end }}><option value="">Select…</option>{{ range .Options }}<option value="{{ . }}">{{ . }}</option>{{ end }}</select>{{ else }}<input id="form-{{ $.InstanceID }}-field-{{ .ID }}" name="{{ .Key }}" type="{{ .Type }}" placeholder="{{ .Placeholder }}" maxlength="{{ if eq .Type "email" }}320{{ else }}500{{ end }}"{{ if .Required }} required{{ end }}>{{ end }}{{ end }}</div>{{ end }}<button type="submit">{{ .Form.SubmitLabel }}</button></form>`))

type formRenderer struct{}

func (f *formRenderer) Render(ctx context.Context, node PreparedNode, rc RenderContext, _ *Renderer) (template.HTML, error) {
	formID, _ := node.Settings["formId"].(string)
	if formID == "" || rc.FormReader == nil {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<div class="block-placeholder">Form unavailable</div>`), nil
		}
		return "", nil
	}
	view, ok := rc.FormCache[formID]
	if !ok {
		view, ok = rc.FormReader.GetActiveForm(ctx, formID)
		if ok && rc.FormCache != nil {
			rc.FormCache[formID] = view
		}
	}
	if !ok {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<div class="block-placeholder">Form unavailable</div>`), nil
		}
		return "", nil
	}
	if rc.FormResult.SuccessFormID == formID {
		var out bytes.Buffer
		_ = template.Must(template.New("success").Parse(`<div id="form-{{ .ID }}" class="stratum-form-success" role="status">{{ .Message }}</div>`)).Execute(&out, map[string]string{"ID": safeDOMToken(node.ID), "Message": view.SuccessMessage})
		return template.HTML(out.String()), nil
	}
	returnTo := rc.Route.Path
	if returnTo == "" || !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = "/"
	}
	var out bytes.Buffer
	err := publicFormTemplate.Execute(&out, map[string]any{"InstanceID": safeDOMToken(node.ID), "Form": view, "ReturnTo": returnTo})
	return template.HTML(out.String()), err
}

func safeDOMToken(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "form"
	}
	return b.String()
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
	if rc.Dependencies == nil {
		rc.Dependencies = &DependencyState{SiteParts: make(map[string]ResolvedSitePart)}
	} else if rc.Dependencies.SiteParts == nil {
		rc.Dependencies.SiteParts = make(map[string]ResolvedSitePart)
	}
	if rc.CollectedSiteParts == nil {
		rc.CollectedSiteParts = make(map[string]string)
	}
	// Initialise shared LCP state if caller did not.
	if rc.LCP == nil {
		rc.LCP = &LCPState{}
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
	if node.ID == rc.LCPNodeID && rc.LCP != nil && !rc.LCP.Consumed {
		// Only claim if this actual instance has a real image. For a
		// Collection-embedded featured-image the first entry may be a
		// placeholder while the second has media; the second should win.
		if view, ok := r.hasActualImage(node, rc); ok {
			priority = true
			rc.LCP.Consumed = true
			rc.LCP.PreloadHref = view.Src
			rc.LCP.PreloadSrcSet = view.SrcSet
			rc.LCP.PreloadView = view
			if s, _ := node.Settings["sizes"].(string); s != "" {
				rc.LCP.PreloadSizes = s
			} else {
				rc.LCP.PreloadSizes = "(min-width: 768px) min(100vw, 1200px), 100vw"
			}
		}
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

type entryFieldRenderer struct{}

func (e *entryFieldRenderer) Render(_ context.Context, node PreparedNode, rc RenderContext, _ *Renderer) (template.HTML, error) {
	source, _ := node.Props["source"].(string)
	ref, err := content.ParseFieldRef(source)
	if err != nil {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Invalid entry field</span>`), nil
		}
		return "", nil
	}
	resolved := content.ResolveEntryFieldTyped(ref, rc.Definition, rc.Entry.Title, rc.Entry.Excerpt, rc.Entry.Permalink, rc.Entry.PublishISO, rc.Entry.FeaturedImage, rc.Entry.Fields)
	if !resolved.Exists || resolved.Value == nil {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Entry field</span>`), nil
		}
		return "", nil
	}
	// Media IDs are never emitted as text. core/entry-media is the only block
	// allowed to turn a media reference into markup.
	if resolved.Type == content.FieldMedia || ref.System == "entry.featured_media" {
		return "", nil
	}
	// Fallback: if typed resolution was unavailable but value looks like a media ID, suppress heuristically.
	if resolved.Type == "" && ref.Key != "" {
		if s, ok := resolved.Value.(string); ok && isLikelyMediaID(s) {
			// If we cannot prove it's not media, be conservative: do not leak.
			// Check if UI catalog for this content type lists this key as media.
			if rc.Definition.ID != "" {
				// If definition is present but type unknown, it was deleted/historical – suppress anyway.
				return "", nil
			}
		}
	}
	value := resolved.Value
	text := ""
	switch v := value.(type) {
	case bool:
		if v {
			text = "Yes"
		} else {
			text = "No"
		}
	case string:
		text = v
	default:
		text = fmt.Sprint(v)
	}
	tag, _ := node.Settings["tag"].(string)
	allowed := map[string]bool{"span": true, "p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "strong": true}
	if !allowed[tag] {
		tag = "span"
	}
	prefix, _ := node.Settings["prefix"].(string)
	suffix, _ := node.Settings["suffix"].(string)
	escaped := template.HTMLEscapeString(prefix + text + suffix)
	return template.HTML("<" + tag + ` class="stratum-entry-field">` + escaped + "</" + tag + ">"), nil
}

func isLikelyMediaID(s string) bool {
	// Media IDs are base64url 22-char strings from randomID(). Be conservative.
	if len(s) < 16 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

type entryMediaRenderer struct{}

func (e *entryMediaRenderer) Render(ctx context.Context, node PreparedNode, rc RenderContext, r *Renderer) (template.HTML, error) {
	source, _ := node.Props["source"].(string)
	ref, err := content.ParseFieldRef(source)
	if err != nil {
		return "", nil
	}
	value, ok := content.ResolveEntryField(ref, rc.Entry.Title, rc.Entry.Excerpt, rc.Entry.Permalink, rc.Entry.PublishISO, rc.Entry.FeaturedImage, rc.Entry.Fields)
	id, stringOK := value.(string)
	if !ok || !stringOK || id == "" || r.mediaProvider == nil {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Entry media</span>`), nil
		}
		return "", nil
	}
	view, ok := r.mediaProvider.MediaView(ctx, id)
	if !ok || view.Src == "" {
		return "", nil
	}
	alt := view.Alt
	if custom, _ := node.Settings["alt"].(string); custom != "" {
		alt = custom
	}
	sizes, _ := node.Settings["sizes"].(string)
	if sizes == "" {
		sizes = "100vw"
	}
	attrs := ` src="` + template.HTMLEscapeString(view.Src) + `" alt="` + template.HTMLEscapeString(alt) + `" loading="lazy" decoding="async"`
	if view.Width > 0 {
		attrs += fmt.Sprintf(` width="%d"`, view.Width)
	}
	if view.Height > 0 {
		attrs += fmt.Sprintf(` height="%d"`, view.Height)
	}
	if view.SrcSet != "" {
		attrs += ` srcset="` + template.HTMLEscapeString(view.SrcSet) + `" sizes="` + template.HTMLEscapeString(sizes) + `"`
	}
	img := "<img" + attrs + ">"
	if view.WebPSrcSet != "" {
		img = `<picture><source type="image/webp" srcset="` + template.HTMLEscapeString(view.WebPSrcSet) + `" sizes="` + template.HTMLEscapeString(sizes) + `">` + img + `</picture>`
	}
	return template.HTML(`<figure class="stratum-entry-media">` + img + `</figure>`), nil
}

func (c *collectionRenderer) Render(ctx context.Context, node PreparedNode, rc RenderContext, r *Renderer) (template.HTML, error) {
	tmpl := r.blocks[blockKey{name: node.Block, version: int64(node.Version)}]
	return r.renderCollectionNode(ctx, node, rc, tmpl)
}

type sitePartRenderer struct{}

type archiveTitleRenderer struct{}

func (a *archiveTitleRenderer) Render(_ context.Context, node PreparedNode, rc RenderContext, _ *Renderer) (template.HTML, error) {
	title := rc.Route.ArchiveTitle
	if title == "" && rc.Route.Archive != nil {
		title = rc.Route.Archive.Title
	}
	if title == "" && rc.Archive != nil {
		title = rc.Archive.Title
	}
	if title == "" {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<h1 class="stratum-archive-title stratum-align-left"><span class="stratum-placeholder">Archive title</span></h1>`), nil
		}
		return "", nil
	}
	level := 1
	switch value := node.Settings["level"].(type) {
	case int:
		level = value
	case int64:
		level = int(value)
	case float64:
		level = int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			level = int(parsed)
		}
	}
	if level < 1 || level > 6 {
		level = 1
	}
	align, _ := node.Settings["align"].(string)
	if align != "center" && align != "right" {
		align = "left"
	}
	tag := fmt.Sprintf("h%d", level)
	return template.HTML("<" + tag + ` class="stratum-archive-title stratum-align-` + align + `">` + template.HTMLEscapeString(title) + "</" + tag + ">"), nil
}

type archiveDescriptionRenderer struct{}

func (a *archiveDescriptionRenderer) Render(_ context.Context, node PreparedNode, rc RenderContext, _ *Renderer) (template.HTML, error) {
	description := rc.Route.ArchiveDescription
	if description == "" && rc.Route.Archive != nil {
		description = rc.Route.Archive.Description
	}
	if description == "" && rc.Archive != nil {
		description = rc.Archive.Description
	}
	if description == "" {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<p class="stratum-archive-description stratum-align-left stratum-placeholder">Archive description</p>`), nil
		}
		return "", nil
	}
	align, _ := node.Settings["align"].(string)
	if align != "center" && align != "right" {
		align = "left"
	}
	return template.HTML(`<p class="stratum-archive-description stratum-align-` + align + `">` + template.HTMLEscapeString(description) + `</p>`), nil
}

type navigationRenderer struct{}

func (n *navigationRenderer) Render(_ context.Context, node PreparedNode, rc RenderContext, _ *Renderer) (template.HTML, error) {
	location, _ := node.Settings["location"].(string)
	if location == "" {
		location = "primary"
	}
	menu, ok := rc.Navigation[location]
	if !ok || len(menu.Items) == 0 {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Navigation</span>`), nil
		}
		return "", nil
	}
	style, _ := node.Settings["style"].(string)
	if style != "vertical" {
		style = "horizontal"
	}
	label := "Navigation"
	switch location {
	case "primary":
		label = "Primary navigation"
	case "footer":
		label = "Footer navigation"
	}
	var out strings.Builder
	out.WriteString(`<nav class="stratum-navigation stratum-navigation--`)
	out.WriteString(style)
	out.WriteString(`" aria-label="`)
	out.WriteString(template.HTMLEscapeString(label))
	out.WriteString(`"><ul>`)
	renderNavigationItems(&out, menu.Items)
	out.WriteString(`</ul></nav>`)
	return template.HTML(out.String()), nil
}

func renderNavigationItems(out *strings.Builder, items []navigation.MenuItem) {
	for _, item := range items {
		out.WriteString(`<li`)
		if item.Current {
			out.WriteString(` class="is-current"`)
		} else if item.Ancestor {
			out.WriteString(` class="is-ancestor"`)
		}
		out.WriteString(`>`)
		label := template.HTMLEscapeString(item.Label)
		if item.URL != "" {
			out.WriteString(`<a href="`)
			out.WriteString(template.HTMLEscapeString(item.URL))
			out.WriteString(`"`)
			if item.Current {
				out.WriteString(` aria-current="page"`)
			}
			out.WriteString(`>`)
			out.WriteString(label)
			out.WriteString(`</a>`)
		} else {
			out.WriteString(`<span>`)
			out.WriteString(label)
			out.WriteString(`</span>`)
		}
		if len(item.Children) > 0 {
			out.WriteString(`<ul>`)
			renderNavigationItems(out, item.Children)
			out.WriteString(`</ul>`)
		}
		out.WriteString(`</li>`)
	}
}

func (s *sitePartRenderer) Render(ctx context.Context, node PreparedNode, rc RenderContext, r *Renderer) (template.HTML, error) {
	sitePartID, _ := node.Settings["sitePartId"].(string)
	if sitePartID == "" {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Site part</span>`), nil
		}
		return "", nil
	}
	if rc.SitePartStack != nil {
		if _, exists := rc.SitePartStack[sitePartID]; exists {
			return template.HTML(`<!-- site-part cycle detected -->`), nil
		}
	}
	if rc.SitePartDepth > 16 {
		return template.HTML(`<!-- site-part max depth -->`), nil
	}
	if rc.SitePartReader == nil {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Site part</span>`), nil
		}
		return "", nil
	}
	pd, revID, err := rc.SitePartReader.GetSitePart(ctx, sitePartID)
	if err != nil || pd == nil {
		if rc.IsPreview || rc.Mode == ModePreview {
			return template.HTML(`<span class="stratum-placeholder">Site part unavailable</span>`), nil
		}
		return "", nil
	}
	if rc.Dependencies == nil {
		rc.Dependencies = &DependencyState{SiteParts: make(map[string]ResolvedSitePart)}
	} else if rc.Dependencies.SiteParts == nil {
		rc.Dependencies.SiteParts = make(map[string]ResolvedSitePart)
	}
	if rc.CollectedSiteParts == nil {
		rc.CollectedSiteParts = make(map[string]string)
	}
	rc.CollectedSiteParts[sitePartID] = revID
	rc.Dependencies.SiteParts[sitePartID] = ResolvedSitePart{RevisionID: revID, UsedBlocks: pd.UsedBlocks}
	nested := rc
	if nested.SitePartStack == nil {
		nested.SitePartStack = make(map[string]struct{})
	} else {
		copied := make(map[string]struct{}, len(rc.SitePartStack)+1)
		for k, v := range rc.SitePartStack {
			copied[k] = v
		}
		nested.SitePartStack = copied
	}
	nested.SitePartStack[sitePartID] = struct{}{}
	nested.SitePartDepth = rc.SitePartDepth + 1
	nested.Entry = EntryContext{}
	nested.EntryID = ""
	nested.ContentReader = rc.ContentReader
	nested.QueryCache = rc.QueryCache
	nested.Route = rc.Route
	nested.LCP = rc.LCP
	nested.Dependencies = rc.Dependencies
	inner, err := r.renderPreparedNodes(ctx, pd.Nodes, nested)
	if err != nil {
		return "", err
	}
	if nested.CollectedSiteParts != nil && rc.CollectedSiteParts != nil {
		for k, v := range nested.CollectedSiteParts {
			rc.CollectedSiteParts[k] = v
		}
	} else if nested.CollectedSiteParts != nil {
		rc.CollectedSiteParts = nested.CollectedSiteParts
	}
	tmpl := r.blocks[blockKey{name: node.Block, version: int64(node.Version)}]
	if tmpl != nil {
		var out bytes.Buffer
		if err := tmpl.Execute(&out, blockData{ID: node.ID, Props: node.Props, Settings: node.Settings, Children: inner, Context: rc}); err != nil {
			return "", fmt.Errorf("render block %s@%d: %w", node.Block, node.Version, err)
		}
		return template.HTML(out.String()), nil
	}
	return inner, nil
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
		query := content.EntryQuery{ContentType: content.ContentTypeID(contentType), Limit: limit, Offset: offset, Order: order, ExcludeIDs: excludeIDs}
		query.OrderBy, _ = node.Settings["orderBy"].(string)
		query.Direction, _ = node.Settings["direction"].(string)
		query.TermID, _ = node.Settings["termId"].(string)
		if rawFilters, ok := node.Settings["filters"].([]any); ok {
			for _, raw := range rawFilters {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				field, _ := item["field"].(string)
				operator, _ := item["operator"].(string)
				query.Filters = append(query.Filters, content.EntryFilter{Field: field, Operator: content.QueryOperator(operator), Value: item["value"]})
			}
		}
		query = query.Normalized()
		if err := query.Validate(); err != nil {
			return "", fmt.Errorf("collection %s: %w", node.ID, err)
		}
		cacheKey := query.CacheKey()
		if rc.QueryCache != nil {
			if cached, ok := rc.QueryCache[cacheKey]; ok {
				entries = cached
			} else if rc.ContentReader != nil {
				fetched, err := rc.ContentReader.Query(ctx, query)
				if err != nil {
					// Historical/deleted field refs must not panic the public render – treat as empty.
					if isHistoricCollectionError(err) {
						entries = nil
					} else {
						return "", fmt.Errorf("collection %s: %w", node.ID, err)
					}
				} else {
					entries = fetched
					rc.QueryCache[cacheKey] = entries
				}
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
			fetched, err := rc.ContentReader.Query(ctx, query)
			if err != nil {
				if isHistoricCollectionError(err) {
					entries = nil
				} else {
					return "", fmt.Errorf("collection %s: %w", node.ID, err)
				}
			} else {
				entries = fetched
			}
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

func isHistoricCollectionError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "does not exist for content type") || strings.Contains(msg, "not allowed for field") || strings.Contains(msg, "requires a value")
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
		query := content.EntryQuery{ContentType: content.ContentTypeID(contentType), Limit: limit, Offset: offset, Order: order, ExcludeIDs: excludeIDs}
		query.OrderBy, _ = colNode.Settings["orderBy"].(string)
		query.Direction, _ = colNode.Settings["direction"].(string)
		query.TermID, _ = colNode.Settings["termId"].(string)
		if rawFilters, ok := colNode.Settings["filters"].([]any); ok {
			for _, raw := range rawFilters {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				field, _ := item["field"].(string)
				operator, _ := item["operator"].(string)
				query.Filters = append(query.Filters, content.EntryFilter{Field: field, Operator: content.QueryOperator(operator), Value: item["value"]})
			}
		}
		query = query.Normalized()
		if err := query.Validate(); err != nil {
			if isHistoricCollectionError(err) {
				return nil, nil
			}
			return nil, err
		}
		cacheKey := query.CacheKey()
		if rc.QueryCache != nil {
			if cached, ok := rc.QueryCache[cacheKey]; ok {
				entries = cached
			} else if rc.ContentReader != nil {
				fetched, err := rc.ContentReader.Query(ctx, query)
				if err != nil {
					if isHistoricCollectionError(err) {
						return nil, nil
					}
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
			fetched, err := rc.ContentReader.Query(ctx, query)
			if err != nil {
				if isHistoricCollectionError(err) {
					return nil, nil
				}
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
