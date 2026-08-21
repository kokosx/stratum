package rendering

import (
	"bytes"
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
	blocks map[blockKey]*template.Template
}

type blockKey struct {
	name    string
	version int64
}

type blockData struct {
	Props    map[string]any
	Settings map[string]any
	Children template.HTML
}

// NewRenderer validates and compiles enabled block templates from the database.
func NewRenderer(definitions []Definition) (*Renderer, error) {
	renderer := &Renderer{blocks: make(map[blockKey]*template.Template, len(definitions))}

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

		tmpl, err := template.New(key.name).Parse(definition.Template)
		if err != nil {
			return nil, fmt.Errorf("parse block %s@%d template: %w", key.name, key.version, err)
		}
		renderer.blocks[key] = tmpl
	}

	return renderer, nil
}

func (r *Renderer) RenderDocument(doc *document.Document) (template.HTML, error) {
	var out strings.Builder
	for _, node := range doc.Nodes {
		rendered, err := r.renderNode(node)
		if err != nil {
			return "", err
		}
		out.WriteString(string(rendered))
	}
	return template.HTML(out.String()), nil
}

func (r *Renderer) renderNode(node document.Node) (template.HTML, error) {
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
		rendered, err := r.renderNode(child)
		if err != nil {
			return "", err
		}
		children.WriteString(string(rendered))
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, blockData{Props: props, Settings: settings, Children: template.HTML(children.String())}); err != nil {
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
