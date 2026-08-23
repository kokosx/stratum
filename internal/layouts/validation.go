package layouts

import (
	"errors"
	"fmt"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
)

// ValidateLayoutTemplateDocument validates that doc is a valid block document
// with exactly one Content Slot.
func ValidateLayoutTemplateDocument(registry *blocks.Registry, doc *document.Document) error {
	if doc == nil {
		return errors.New("document is nil")
	}
	if registry == nil {
		return errors.New("block registry is not configured")
	}
	if err := registry.ValidateDocument(doc); err != nil {
		return err
	}
	count := countSlot(doc.Nodes)
	if count != 1 {
		return fmt.Errorf("Layout template must contain exactly one Content block, found %d", count)
	}
	// Validate slot nodes have no children and no unexpected props/settings? The schema already ensures empty.
	// Additional check: ensure slot block version is 1
	var check func([]document.Node) error
	check = func(nodes []document.Node) error {
		for _, n := range nodes {
			if n.Block == contentSlotBlock {
				if n.Version != 1 {
					return fmt.Errorf("Content Slot must be version 1")
				}
				if len(n.Children) != 0 {
					return errors.New("Content Slot must not have children")
				}
			}
			if err := check(n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return check(doc.Nodes)
}

// ValidateEntryDocument validates that doc is a valid document with zero Content Slots.
func ValidateEntryDocument(registry *blocks.Registry, doc *document.Document) error {
	if doc == nil {
		return errors.New("document is nil")
	}
	if registry == nil {
		return errors.New("block registry is not configured")
	}
	if err := registry.ValidateDocument(doc); err != nil {
		return err
	}
	count := countSlot(doc.Nodes)
	if count != 0 {
		return errors.New("Content Slot is only allowed inside Layout Templates")
	}
	return nil
}
