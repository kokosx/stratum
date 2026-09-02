package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/richtext"
)

func (r *Registry) ValidateDocument(doc *document.Document) error {
	current := r.snapshot.Load()
	if current == nil {
		return fmt.Errorf("block registry is not initialized")
	}
	return current.validateDocument(doc)
}

// ValidateDocumentForContext applies the normal schema validation and then
// verifies every block is permitted in the server-owned editor context.
func (r *Registry) ValidateDocumentForContext(doc *document.Document, mode string) error {
	current := r.snapshot.Load()
	if current == nil {
		return fmt.Errorf("block registry is not initialized")
	}
	if err := current.validateDocument(doc); err != nil {
		return err
	}
	var walk func([]document.Node) error
	walk = func(nodes []document.Node) error {
		for _, node := range nodes {
			definition := current.definitions[BlockKey{Name: node.Block, Version: int64(node.Version)}]
			if definition == nil || !isEditorContextAllowed(definition.EditorContexts, mode) {
				return fmt.Errorf("block %s is not allowed in %s context", node.Block, mode)
			}
			if err := walk(node.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(doc.Nodes)
}

func (s *snapshot) validateDocument(doc *document.Document) error {
	if err := document.Validate(doc); err != nil {
		return err
	}
	for i, node := range doc.Nodes {
		if err := s.validateNode(node, fmt.Sprintf("nodes[%d]", i), ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) validateNode(node document.Node, path string, parentBlock string) error {
	definition, ok := s.definitions[BlockKey{Name: node.Block, Version: int64(node.Version)}]
	if !ok {
		return fmt.Errorf("unknown block %s@%d", node.Block, node.Version)
	}
	name := definition.DisplayName
	label := fmt.Sprintf("%q block", name)
	if len(definition.Schema.Placement.Parents) > 0 {
		allowed := false
		for _, p := range definition.Schema.Placement.Parents {
			if p == parentBlock {
				allowed = true
				break
			}
		}
		if !allowed {
			if parentBlock == "" {
				return fmt.Errorf("the %s cannot be placed at the top level", label)
			}
			parentName := s.displayNameForBlock(parentBlock)
			parentLabel := fmt.Sprintf("%q block", parentName)
			return fmt.Errorf("the %s cannot be placed inside the %s", label, parentLabel)
		}
	}
	props, err := decodeJSONValue(node.Props, map[string]any{})
	if err != nil {
		return fmt.Errorf("%s.props: %w", path, err)
	}
	if err := validateValue(definition.Schema.Props, props, path+".props", true, false); err != nil {
		return err
	}
	if err := validateRichTextFields(definition.Schema, props, path+".props"); err != nil {
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
		if err := s.validateNode(child, childPath, node.Block); err != nil {
			return err
		}
	}
	return nil
}

func validateRichTextFields(schema Schema, value any, path string) error {
	props, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for name, field := range schema.Editor.Fields {
		if field.Control != "richtext" || !strings.HasPrefix(name, "props.") {
			continue
		}
		property := strings.TrimPrefix(name, "props.")
		if _, err := richtext.Parse(props[property]); err != nil {
			return fmt.Errorf("%s.%s: %w", path, property, err)
		}
	}
	return nil
}

func (s *snapshot) displayNameForBlock(blockName string) string {
	bestVer := int64(-1)
	var bestName string
	for k, d := range s.definitions {
		if k.Name == blockName && k.Version > bestVer {
			bestVer = k.Version
			bestName = d.DisplayName
		}
	}
	if bestName != "" {
		return bestName
	}
	return blockName
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
