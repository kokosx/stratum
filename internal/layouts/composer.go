package layouts

import (
	"errors"
	"fmt"

	"github.com/kokosx/stratum/internal/document"
)

const contentSlotBlock = "core/content-slot"

// Compose splices entry root nodes into the layout template's single Content Slot.
// It never mutates the input documents. It validates slot count and duplicate IDs.
func Compose(layoutDoc *document.Document, entryDoc *document.Document) (*document.Document, error) {
	if layoutDoc == nil {
		return nil, errors.New("layout document is nil")
	}
	if entryDoc == nil {
		return nil, errors.New("entry document is nil")
	}
	if layoutDoc.Version != entryDoc.Version {
		return nil, fmt.Errorf("document version mismatch: layout %d vs entry %d", layoutDoc.Version, entryDoc.Version)
	}
	if layoutDoc.Version != 1 {
		return nil, fmt.Errorf("unsupported document version: %d", layoutDoc.Version)
	}

	// Count slots in entry (must be 0)
	if c := countSlot(entryDoc.Nodes); c != 0 {
		return nil, errors.New("entry document must not contain a Content Slot")
	}

	// Deep copy via document.Clone (correct, no mutation, preserves raw JSON).
	layoutCopy := document.Clone(layoutDoc)
	entryCopy := document.Clone(entryDoc)
	if layoutCopy == nil || entryCopy == nil {
		return nil, errors.New("failed to clone documents")
	}

	// Duplicate ID check: entry IDs vs template non-slot IDs.
	// Slot ID itself disappears, so entry may reuse it.
	entryIDs := make(map[string]struct{})
	for _, n := range entryCopy.Nodes {
		if err := collectIDs(n, entryIDs); err != nil {
			return nil, err
		}
	}
	nonSlotIDs := make(map[string]struct{})
	var collectNonSlot func([]document.Node) error
	collectNonSlot = func(nodes []document.Node) error {
		for _, n := range nodes {
			if n.Block == contentSlotBlock {
				continue
			}
			if _, exists := nonSlotIDs[n.ID]; exists {
				return fmt.Errorf("duplicate node id %q", n.ID)
			}
			nonSlotIDs[n.ID] = struct{}{}
			if err := collectNonSlot(n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collectNonSlot(layoutCopy.Nodes); err != nil {
		return nil, err
	}
	for id := range entryIDs {
		if _, exists := nonSlotIDs[id]; exists {
			return nil, fmt.Errorf("duplicate node id %q across layout and entry", id)
		}
	}

	composedNodes, slots, err := replace(layoutCopy.Nodes, entryCopy.Nodes)
	if err != nil {
		return nil, err
	}
	if slots != 1 {
		return nil, fmt.Errorf("layout template must contain exactly one Content block, found %d", slots)
	}

	result := &document.Document{
		Version: layoutCopy.Version,
		Nodes:   composedNodes,
	}
	if err := document.Validate(result); err != nil {
		return nil, fmt.Errorf("composed document invalid: %w", err)
	}
	return result, nil
}

func replace(nodes []document.Node, entryNodes []document.Node) ([]document.Node, int, error) {
	out := make([]document.Node, 0, len(nodes)+len(entryNodes))
	slots := 0
	for _, node := range nodes {
		if node.Block == contentSlotBlock {
			slots++
			// Validate slot has no props/settings/children semantics (if any extra data, ignore but ensure empty?)
			// Clone entry nodes deeply
			cloned := cloneNodes(entryNodes)
			out = append(out, cloned...)
			continue
		}
		children, nested, err := replace(node.Children, entryNodes)
		if err != nil {
			return nil, 0, err
		}
		slots += nested
		node.Children = children
		out = append(out, node)
	}
	return out, slots, nil
}

func cloneNodes(nodes []document.Node) []document.Node { return document.CloneNodes(nodes) }

func countSlot(nodes []document.Node) int {
	c := 0
	for _, n := range nodes {
		if n.Block == contentSlotBlock {
			c++
		}
		c += countSlot(n.Children)
	}
	return c
}

func collectSlotIDs(nodes []document.Node) map[string]struct{} {
	m := make(map[string]struct{})
	var walk func([]document.Node)
	walk = func(ns []document.Node) {
		for _, n := range ns {
			if n.Block == contentSlotBlock {
				m[n.ID] = struct{}{}
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	return m
}

func collectIDs(node document.Node, ids map[string]struct{}) error {
	if _, exists := ids[node.ID]; exists {
		return fmt.Errorf("duplicate node id %q", node.ID)
	}
	ids[node.ID] = struct{}{}
	for _, child := range node.Children {
		if err := collectIDs(child, ids); err != nil {
			return err
		}
	}
	return nil
}
