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
		return fmt.Errorf("unknown block %s@%d", node.Block, node.Version)
	}
	name := definition.DisplayName
	label := fmt.Sprintf("%q block", name)
	props, err := decodeJSONValue(node.Props, map[string]any{})
	if err != nil {
		return fmt.Errorf("%s.props: %w", path, err)
	}
	if err := validateValue(definition.Schema.Props, props, path+".props", true, false); err != nil {
		return err
	}
	settings, err := decodeJSONValue(node.Settings, map[string]any{})
	if err != nil {
		return fmt.Errorf("%s.settings: %w", path, err)
	}
	if err := validateValue(definition.Schema.Settings, settings, path+".settings", true, true); err != nil {
		return err
	}
	children := definition.Schema.Children
	if children.Mode == "none" && len(node.Children) > 0 {
		return fmt.Errorf("the %s does not allow child blocks", label)
	}
	if children.Min != nil && len(node.Children) < *children.Min {
		unit := "child block"
		if *children.Min > 1 {
			unit = "child blocks"
		}
		return fmt.Errorf("the %s requires at least %d %s", label, *children.Min, unit)
	}
	if children.Max != nil && len(node.Children) > *children.Max {
		unit := "child block"
		if *children.Max > 1 {
			unit = "child blocks"
		}
		return fmt.Errorf("the %s allows at most %d %s", label, *children.Max, unit)
	}
	allowed := make(map[string]bool, len(children.Blocks))
	for _, block := range children.Blocks {
		allowed[block] = true
	}
	for i, child := range node.Children {
		childPath := fmt.Sprintf("%s.children[%d]", path, i)
		if children.Mode == "allowed" && !allowed[child.Block] {
			childName := child.Block
			if childDef, ok := s.definitions[BlockKey{Name: child.Block, Version: int64(child.Version)}]; ok {
				childName = childDef.DisplayName
			}
			return fmt.Errorf("the %q block is not allowed inside the %s", childName, label)
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
