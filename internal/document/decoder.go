package document

import (
	"encoding/json"
	"errors"
	"fmt"
)

const maxDocumentDepth = 64

func Decode(data []byte) (*Document, error) {
	if len(data) == 0 {
		return nil, errors.New("document is empty")
	}

	var doc Document

	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode document JSON: %w", err)
	}

	if err := validateDocument(&doc); err != nil {
		return nil, err
	}

	return &doc, nil
}

func validateDocument(doc *Document) error {
	if doc.Version != 1 {
		return fmt.Errorf("unsupported document version: %d", doc.Version)
	}

	ids := make(map[string]struct{})
	for i, node := range doc.Nodes {
		if err := validateNode(node, ids, 1); err != nil {
			return fmt.Errorf("node %d: %w", i, err)
		}
	}

	return nil
}

func validateNode(node Node, ids map[string]struct{}, depth int) error {
	if depth > maxDocumentDepth {
		return fmt.Errorf("maximum document depth of %d exceeded", maxDocumentDepth)
	}
	if node.ID == "" {
		return errors.New("node id is required")
	}
	if _, exists := ids[node.ID]; exists {
		return fmt.Errorf("duplicate node id %q", node.ID)
	}
	ids[node.ID] = struct{}{}

	if node.Block == "" {
		return errors.New("node block is required")
	}

	if node.Version <= 0 {
		return errors.New("node version must be greater than 0")
	}

	for i, child := range node.Children {
		if err := validateNode(child, ids, depth+1); err != nil {
			return fmt.Errorf("child %d: %w", i, err)
		}
	}

	return nil
}
