package blocks

import (
	"encoding/json"

	"github.com/kokosx/stratum/internal/document"
)

// migrateLegacyPostsInPlace performs an in-memory compatibility migration for
// historical documents containing core/posts@1. It rewrites them to
// core/collection@1 without mutating the stored revision. This allows the
// public handler and block registry to delete the legacy latestCollections
// / Collections / ArchiveURL plumbing while keeping old published content
// renderable.
//
// Mapping:
//
//	source "archive" / "automatic" → "context" (uses Route.Archive)
//	source "latest"               → "query"   (uses ContentReader)
//	limit                         → limit
//	other posts settings (layout etc) are dropped – the new Collection relies
//	on its children for presentation, but an empty-children fallback renders the
//	classic card so old docs without children still look correct.
func migrateLegacyPostsInPlace(doc *document.Document) *document.Document {
	if doc == nil {
		return nil
	}
	changed := false
	var walk func([]document.Node) []document.Node
	walk = func(nodes []document.Node) []document.Node {
		out := make([]document.Node, len(nodes))
		for i, n := range nodes {
			if n.Block == "core/posts" && n.Version == 1 {
				changed = true
				// Decode settings to extract source/limit.
				var settings map[string]any
				if len(n.Settings) > 0 {
					_ = json.Unmarshal(n.Settings, &settings)
				}
				if settings == nil {
					settings = map[string]any{}
				}
				source, _ := settings["source"].(string)
				if source == "" {
					source = "automatic"
				}
				if source == "archive" {
					source = "automatic"
				}
				newSource := "query"
				if source == "automatic" {
					newSource = "context"
				} else if source == "latest" {
					newSource = "query"
				}
				limit := 3
				if v, ok := settings["limit"]; ok {
					switch val := v.(type) {
					case float64:
						limit = int(val)
					case int:
						limit = val
					case int64:
						limit = int(val)
					}
				}
				newSettings := map[string]any{
					"source":      newSource,
					"contentType": "post",
					"limit":       limit,
				}
				// Preserve pagination-related and display settings for fallback rendering.
				for _, k := range []string{"offset", "order", "excludeCurrent", "layout", "columns", "showImage", "showDate", "showExcerpt", "pagination", "showViewAll", "viewAllLabel"} {
					if v, ok := settings[k]; ok {
						newSettings[k] = v
					}
				}
				encoded, _ := json.Marshal(newSettings)
				n.Block = "core/collection"
				n.Version = 1
				n.Settings = json.RawMessage(encoded)
				// Keep existing children (if any) – new Collection will render them per-entry.
				// If none, the renderer will use a legacy card fallback.
			}
			if len(n.Children) > 0 {
				n.Children = walk(n.Children)
			}
			out[i] = n
		}
		return out
	}
	newNodes := walk(doc.Nodes)
	if !changed {
		return doc
	}
	clone := *doc
	clone.Nodes = newNodes
	return &clone
}
