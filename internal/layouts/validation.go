package layouts

import (
	"errors"
	"fmt"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
)

// ValidateLayoutTemplateDocument validates that doc is a valid block document
// with exactly one Content Slot. Kept for backward compat – new code should use ValidateTemplateDocument.
func ValidateLayoutTemplateDocument(registry *blocks.Registry, doc *document.Document) error {
	return ValidateTemplateDocument(registry, doc, "single", nil)
}

// ValidateLayoutTemplateDocumentForKind is historic alias.
func ValidateLayoutTemplateDocumentForKind(registry *blocks.Registry, doc *document.Document, kind string, hasContent *bool) error {
	return ValidateTemplateDocument(registry, doc, kind, hasContent)
}

// ValidateTemplateDocument validates a template document by kind.
// hasContent is optional content type HasContent flag; nil means unknown (allow 0 or 1).
func ValidateTemplateDocument(registry *blocks.Registry, doc *document.Document, kind string, hasContent *bool) error {
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
	switch kind {
	case "single":
		if count > 1 {
			return fmt.Errorf("Single template must contain at most one Content block, found %d", count)
		}
		// zero allowed – editor will warn if HasContent true
		_ = hasContent
	case "archive":
		if count != 0 {
			return errors.New("Archive template must not contain a Content Slot")
		}
	default:
		if count != 0 {
			return fmt.Errorf("template kind %q must not contain a Content Slot", kind)
		}
	}
	// Validate slot invariants
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
				if kind == "archive" {
					return errors.New("Content Slot is not allowed in Archive Templates")
				}
			}
			// Archive-only blocks
			if (n.Block == "core/archive-title" || n.Block == "core/archive-description") && kind != "archive" {
				return fmt.Errorf("block %s is only allowed in Archive Templates", n.Block)
			}
			// Content-slot check already
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
	// Entry must not contain archive-only blocks
	var check func([]document.Node) error
	check = func(nodes []document.Node) error {
		for _, n := range nodes {
			if n.Block == "core/archive-title" || n.Block == "core/archive-description" {
				return fmt.Errorf("block %s is only allowed in Archive Templates", n.Block)
			}
			if err := check(n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return check(doc.Nodes)
}
