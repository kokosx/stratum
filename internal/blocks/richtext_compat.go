package blocks

import (
	"encoding/json"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/richtext"
)

var richTextMigrationRegistry = document.NewMigrationRegistry()

func init() {
	richTextMigrationRegistry.Register("core/text", 1, migrateRichTextV1ToV2)
	richTextMigrationRegistry.Register("core/heading", 1, migrateRichTextV1ToV2)
}

func migrateRichTextV1ToV2(node document.Node) (document.Node, error) {
	// Preserve ID, Children, Settings, but upgrade props.text from string to RichText.
	var props map[string]any
	if err := json.Unmarshal(node.Props, &props); err != nil {
		return node, nil // leave unchanged if props invalid
	}
	if value, ok := props["text"].(string); ok {
		props["text"] = richtext.RichText{Version: richtext.Version, Content: []richtext.Run{{Text: value}}}
		if data, err := json.Marshal(props); err == nil {
			node.Props = data
			node.Version = 2
		}
	}
	return node, nil
}

// migrateLegacyRichTextInPlace upgrades legacy string props only in the
// render/edit copy using the block migration registry. Published revision JSON remains immutable.
// The generic walker looks up migrators via the registry instead of hardcoding block names.
func migrateLegacyRichTextInPlace(doc *document.Document, definitions map[BlockKey]*Definition) *document.Document {
	if doc == nil {
		return nil
	}
	// Only migrate if target v2 definitions are present (preserves test registries that only have v1).
	if definitions[BlockKey{Name: "core/text", Version: 2}] == nil && definitions[BlockKey{Name: "core/heading", Version: 2}] == nil {
		return doc
	}
	migrated, err := richTextMigrationRegistry.MigrateDocument(doc)
	if err != nil {
		return doc
	}
	origJSON, _ := json.Marshal(doc)
	newJSON, _ := json.Marshal(migrated)
	if string(origJSON) == string(newJSON) {
		return doc
	}
	return migrated
}
