package blocks

import (
	"encoding/json"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/richtext"
)

// migrateLegacyRichTextInPlace upgrades legacy string props only in the
// render/edit copy. Published revision JSON remains immutable.
func migrateLegacyRichTextInPlace(doc *document.Document, definitions map[BlockKey]*Definition) *document.Document {
	if doc == nil {
		return nil
	}
	changed := false
	var walk func([]document.Node) []document.Node
	walk = func(nodes []document.Node) []document.Node {
		out := make([]document.Node, len(nodes))
		for i, node := range nodes {
			if (node.Block == "core/text" || node.Block == "core/heading") && node.Version == 1 && definitions[BlockKey{Name: node.Block, Version: 2}] != nil {
				var props map[string]any
				if json.Unmarshal(node.Props, &props) == nil {
					if value, ok := props["text"].(string); ok {
						props["text"] = richtext.RichText{Version: richtext.Version, Content: []richtext.Run{{Text: value}}}
						if data, err := json.Marshal(props); err == nil {
							node.Props = data
							node.Version = 2
							changed = true
						}
					}
				}
			}
			node.Children = walk(node.Children)
			out[i] = node
		}
		return out
	}
	clone := *doc
	clone.Nodes = walk(doc.Nodes)
	if !changed {
		return doc
	}
	return &clone
}
