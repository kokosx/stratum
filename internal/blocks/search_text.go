package blocks

import (
	"encoding/json"
	"strings"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/richtext"
)

// SearchText extracts semantic text from blocks. Concrete block knowledge stays
// with block implementations; consumers only ask the registry for text.
func (r *Registry) SearchText(doc *document.Document) []string {
	var result []string
	_ = document.Walk(doc, func(node document.Node) error {
		var props map[string]any
		if json.Unmarshal(node.Props, &props) != nil {
			return nil
		}
		for _, text := range blockSearchText(node.Block, props) {
			if text = strings.TrimSpace(text); text != "" {
				result = append(result, text)
			}
		}
		return nil
	})
	return result
}

func blockSearchText(block string, props map[string]any) []string {
	switch block {
	case "core/text", "core/heading":
		if text, ok := props["text"].(string); ok {
			return []string{text}
		}
		if text, err := richtext.Parse(props["text"]); err == nil {
			return []string{text.PlainText()}
		}
	case "core/button":
		return stringProp(props, "label", "text")
	case "core/image":
		return stringProp(props, "alt", "caption")
	case "core/quote":
		return stringProp(props, "text", "citation")
	case "core/accordion-item":
		return stringProp(props, "title")
	case "core/card", "core/section", "core/stack", "core/grid", "core/columns", "core/divider":
		return nil
	case "core/collection", "core/posts", "core/entry-field", "core/entry-media", "core/entry-title", "core/entry-content", "core/site-part", "core/navigation", "core/site-logo", "core/archive-title", "core/archive-description", "core/embed", "core/custom-code":
		return nil
	}
	// Generic fallback for textual blocks that expose a "text" string: avoid indexing CSS/variant/IDs.
	// Only index explicit textual props known to be safe.
	if text, ok := props["text"].(string); ok && text != "" {
		// For unknown blocks, conservatively treat "text" as searchable if not obviously an ID/class.
		// Still allow generic "text" without indexing variant/settings which are separate.
		return []string{text}
	}
	return nil
}

func stringProp(props map[string]any, keys ...string) []string {
	var result []string
	for _, key := range keys {
		if value, ok := props[key].(string); ok {
			result = append(result, value)
		}
	}
	return result
}
