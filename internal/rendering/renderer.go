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

// RenderContext carries request-time data that dynamic blocks bind to (the
// current Entry and Site settings). It is the same for every node in a document.
// In the editor preview it is empty, so dynamic blocks fall back to placeholders.
type RenderContext struct {
	Site       SiteContext
	Entry      EntryContext
	Archive    *ArchiveContext
	Collections map[string][]ArchiveEntry
	ArchiveURL string // URL of the post archive (for view-all links in latest mode)
	LCPNodeID  string
	IsPreview  bool   // true in editor preview; public renders are false
	EntryID    string // current entry ID for current-post exclusion in latest blocks

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
		FeaturedImage: ae.FeaturedImage.Src,
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
		if definition.RendererType != "template" {
			return nil, fmt.Errorf("block %s/%s@%d: unsupported renderer type %q", definition.Namespace, definition.Name, definition.Version, definition.RendererType)
		}
		if definition.Template == "" {
			return nil, fmt.Errorf("block %s/%s@%d: template is required", definition.Namespace, definition.Name, definition.Version)
		}

		key := blockKey{name: definition.Namespace + "/" + definition.Name, version: definition.Version}
		if _, exists := renderer.blocks[key]; exists {
			return nil, fmt.Errorf("duplicate block definition: %s@%d", key.name, key.version)
		}

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

	return renderer, nil
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
	var out strings.Builder
	for _, node := range doc.Nodes {
		rendered, err := r.renderNode(context.Background(), node, rc)
		if err != nil {
			return "", err
		}
		out.WriteString(string(rendered))
	}
	return template.HTML(out.String()), nil
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
	if !ok {
		return "", fmt.Errorf("block definition not found: %s@%d", node.Block, node.Version)
	}

	// Collection is the generic replacement for core/posts. It must be able to
	// render its children multiple times with different scoped Entry contexts.
	// This is the only block that requires lazy children; other blocks keep the
	// simple eager path. The branch is intentionally limited to this block's
	// runtime behaviour, not a generic handler concern.
	if node.Block == "core/collection" {
		return r.renderCollectionNode(ctx, node, rc, tmpl)
	}

	var children strings.Builder
	for i := range node.Children {
		rendered, err := r.renderPreparedNode(ctx, node.Children[i], rc)
		if err != nil {
			return "", err
		}
		children.WriteString(string(rendered))
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, blockData{ID: node.ID, Props: node.Props, Settings: node.Settings, Children: template.HTML(children.String()), Context: rc, Priority: node.ID == rc.LCPNodeID}); err != nil {
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
				if err == nil {
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
			fetched, _ := rc.ContentReader.Query(ctx, contentType, limit, offset, order, excludeIDs)
			entries = fetched
		}
		// Pagination handling for collection when source=query and pagination flag true?
		// Pagination is handled at route level for archive (source=context). For
		// source=query we treat limit/offset as the query; no pagination UI here.
	}

	// Render children per entry with scoped Entry context (lazy children invariant).
	var children strings.Builder
	if len(node.Children) == 0 && len(entries) > 0 {
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
			children.WriteString(`<div class="stratum-posts-empty"><p>No posts found.</p></div>`)
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
	source, _ := node.Settings["source"].(string)
	var b strings.Builder
	b.WriteString(`<section class="stratum-posts stratum-posts--list">`)
	for _, e := range entries {
		b.WriteString(`<article class="stratum-post-card">`)
		if showImage && e.FeaturedImage.Src != "" {
			b.WriteString(`<figure class="stratum-post-card__media"><img src="` + template.HTMLEscapeString(e.FeaturedImage.Src) + `" alt="` + template.HTMLEscapeString(e.FeaturedImage.Alt) + `" loading="lazy" decoding="async"></figure>`)
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
	return template.HTML(b.String())
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
