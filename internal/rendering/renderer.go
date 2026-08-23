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
