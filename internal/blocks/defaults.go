package blocks

import (
	"encoding/json"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/rendering"
)

// documentWithDefaults creates a render-only copy. Defaults make older valid
// revisions deterministic without migrating or rewriting their stored JSON.
func (s *snapshot) documentWithDefaults(doc *document.Document) (*document.Document, error) {
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var result document.Document
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	for i := range result.Nodes {
		if err := s.applyNodeDefaults(&result.Nodes[i]); err != nil {
			return nil, err
		}
	}
	return &result, nil
}

func (s *snapshot) applyNodeDefaults(node *document.Node) error {
	definition := s.definitions[BlockKey{Name: node.Block, Version: int64(node.Version)}]
	props, err := decodeJSONValue(node.Props, map[string]any{})
	if err != nil {
		return err
	}
	settings, err := decodeJSONValue(node.Settings, map[string]any{})
	if err != nil {
		return err
	}
	applyDefaults(definition.Schema.Props, props)
	applyDefaults(definition.Schema.Settings, settings)
	if node.Props, err = json.Marshal(props); err != nil {
		return err
	}
	if node.Settings, err = json.Marshal(settings); err != nil {
		return err
	}
	for i := range node.Children {
		if err := s.applyNodeDefaults(&node.Children[i]); err != nil {
			return err
		}
	}
	return nil
}

// prepareNode builds a PreparedNode: it decodes props/settings, applies schema
// defaults once, and recursively prepares children. The result carries ready
// maps so the renderer never decodes or marshals JSON again.
func (s *snapshot) prepareNode(node document.Node) (rendering.PreparedNode, error) {
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
		prepared, err := s.prepareNode(child)
		if err != nil {
			return rendering.PreparedNode{}, err
		}
		children = append(children, prepared)
	}
	return rendering.PreparedNode{
		ID:       node.ID,
		Block:    node.Block,
		Version:  node.Version,
		Props:    propMap,
		Settings: settingMap,
		Children: children,
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
