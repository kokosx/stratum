package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kokosx/stratum/internal/document"
)

func (r *Registry) ValidateDocument(doc *document.Document) error {
	current := r.snapshot.Load()
	if current == nil {
		return fmt.Errorf("block registry is not initialized")
	}
	return current.validateDocument(doc)
}

func (s *snapshot) validateDocument(doc *document.Document) error {
	if err := document.Validate(doc); err != nil {
		return err
	}
	for i, node := range doc.Nodes {
		if err := s.validateNode(node, fmt.Sprintf("nodes[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) validateNode(node document.Node, path string) error {
	definition, ok := s.definitions[BlockKey{Name: node.Block, Version: int64(node.Version)}]
	if !ok {
		return fmt.Errorf("%s: unknown block %s@%d", path, node.Block, node.Version)
	}
	props, err := decodeJSONValue(node.Props, map[string]any{})
	if err != nil {
		return fmt.Errorf("%s.props: %w", path, err)
	}
	if err := validateValue(definition.Schema.Props, props, path+".props", true); err != nil {
		return err
	}
	settings, err := decodeJSONValue(node.Settings, map[string]any{})
	if err != nil {
		return fmt.Errorf("%s.settings: %w", path, err)
	}
	if err := validateValue(definition.Schema.Settings, settings, path+".settings", true); err != nil {
		return err
	}
	children := definition.Schema.Children
	if children.Mode == "none" && len(node.Children) > 0 {
		return fmt.Errorf("%s.children: block does not allow children", path)
	}
	if children.Min != nil && len(node.Children) < *children.Min {
		return fmt.Errorf("%s.children: requires at least %d children", path, *children.Min)
	}
	if children.Max != nil && len(node.Children) > *children.Max {
		return fmt.Errorf("%s.children: allows at most %d children", path, *children.Max)
	}
	allowed := make(map[string]bool, len(children.Blocks))
	for _, block := range children.Blocks {
		allowed[block] = true
	}
	for i, child := range node.Children {
		childPath := fmt.Sprintf("%s.children[%d]", path, i)
		if children.Mode == "allowed" && !allowed[child.Block] {
			return fmt.Errorf("%s: %s is not allowed inside %s", childPath, child.Block, node.Block)
		}
		if err := s.validateNode(child, childPath); err != nil {
			return err
		}
	}
	return nil
}

func decodeJSONValue(data json.RawMessage, empty any) (any, error) {
	if len(data) == 0 || string(data) == "null" {
		return empty, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}
