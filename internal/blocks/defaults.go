package blocks

import (
	"encoding/json"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/rendering"
)

// prepareNode builds a PreparedNode: it decodes props/settings, applies schema
// defaults once, and recursively prepares children. The result carries ready
// maps so the renderer never decodes or marshals JSON again.
func (s *snapshot) prepareNode(node document.Node) (rendering.PreparedNode, error) {
	return s.prepareNodeWithLegacy(node, nil)
}

func (s *snapshot) prepareNodeWithLegacy(node document.Node, legacyIDs map[string]bool) (rendering.PreparedNode, error) {
	definition := s.definitions[BlockKey{Name: node.Block, Version: int64(node.Version)}]
	props, err := decodeJSONValue(node.Props, map[string]any{})
	if err != nil {
		return rendering.PreparedNode{}, err
	}
	settings, err := decodeJSONValue(node.Settings, map[string]any{})
	if err != nil {
		return rendering.PreparedNode{}, err
	}
	if definition != nil {
		applyDefaults(definition.Schema.Props, props)
		applyDefaults(definition.Schema.Settings, settings)
	}
	// Normalize json.Number produced by UseNumber to float64 so templates
	// like `ne .Settings.start 1.0` work (json.Number != float64 for template funcs).
	if b, err := json.Marshal(props); err == nil {
		var norm any
		if err := json.Unmarshal(b, &norm); err == nil {
			props = norm
		}
	}
	if b, err := json.Marshal(settings); err == nil {
		var norm any
		if err := json.Unmarshal(b, &norm); err == nil {
			settings = norm
		}
	}
	propMap, _ := props.(map[string]any)
	if propMap == nil {
		propMap = map[string]any{}
	}
	settingMap, _ := settings.(map[string]any)
	if settingMap == nil {
		settingMap = map[string]any{}
	}
	children := make([]rendering.PreparedNode, 0, len(node.Children))
	for _, child := range node.Children {
		prepared, err := s.prepareNodeWithLegacy(child, legacyIDs)
		if err != nil {
			return rendering.PreparedNode{}, err
		}
		children = append(children, prepared)
	}
	legacySource := ""
	if legacyIDs != nil && legacyIDs[node.ID] {
		legacySource = "core/posts@1"
	}
	return rendering.PreparedNode{
		ID:           node.ID,
		Block:        node.Block,
		Version:      node.Version,
		Props:        propMap,
		Settings:     settingMap,
		Children:     children,
		LegacySource: legacySource,
	}, nil
}

func applyDefaults(schema ValueSchema, value any) {
	switch schema.Type {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return
		}
		for name, property := range schema.Properties {
			item, exists := object[name]
			if !exists && property.hasDefault {
				object[name] = cloneJSONValue(property.Default)
				item = object[name]
				exists = true
			}
			if exists {
				applyDefaults(property, item)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok || schema.Items == nil {
			return
		}
		for _, item := range array {
			applyDefaults(*schema.Items, item)
		}
	}
}

func cloneJSONValue(value any) any {
	encoded, _ := json.Marshal(value)
	var cloned any
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}
