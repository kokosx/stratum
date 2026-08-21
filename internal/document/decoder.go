package document

import (
	"encoding/json"
	"errors"
	"fmt"
)

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
	if doc.Version <= 0 {
		return errors.New("document version must be greater than 0")
	}

	for i, node := range doc.Nodes {
		if err := validateNode(node); err != nil {
			return fmt.Errorf("node %d: %w", i, err)
		}
	}

	return nil
}

func validateNode(node Node) error {
	if node.ID == "" {
		return errors.New("node id is required")
	}

	if node.Block == "" {
		return errors.New("node block is required")
	}

	if node.Version <= 0 {
		return errors.New("node version must be greater than 0")
	}

	for i, child := range node.Children {
		if err := validateNode(child); err != nil {
			return fmt.Errorf("child %d: %w", i, err)
		}
	}

	return nil
}
