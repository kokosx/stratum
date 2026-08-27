package siteparts

import (
	"errors"
	"fmt"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
)

func ValidateSitePartDocument(registry *blocks.Registry, doc *document.Document) error {
	if doc == nil {
		return errors.New("document is nil")
	}
	if registry == nil {
		return errors.New("block registry is not configured")
	}
	if err := registry.ValidateDocumentForContext(doc, blocks.EditorModeSitePart); err != nil {
		return err
	}
	var check func([]document.Node) error
	check = func(nodes []document.Node) error {
		for _, n := range nodes {
			switch n.Block {
			case "core/content-slot":
				return errors.New("Content Slot is not allowed in Site Parts")
			case "core/archive-title", "core/archive-description":
				return fmt.Errorf("block %s is only allowed in Archive Templates", n.Block)
			case "core/entry-field", "core/entry-media", "core/entry-title", "core/entry-excerpt", "core/entry-publish-date", "core/featured-image":
				return fmt.Errorf("block %s is not allowed in Site Parts (no Entry context)", n.Block)
			}
			if err := check(n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return check(doc.Nodes)
}
