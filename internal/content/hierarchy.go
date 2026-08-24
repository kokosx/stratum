package content

import (
	"errors"
	"fmt"
	"sort"
)

// HierarchyNode is the revision-selected view of an entry in a hierarchy.
// ParentEntryID is deliberately an Entry ID, not a revision ID.
type HierarchyNode struct {
	EntryID       string
	Slug          string
	ParentEntryID string
	MenuOrder     int64
	Title         string
}

// Hierarchy indexes one content-type forest and validates its tree invariant.
type Hierarchy struct {
	nodes    map[string]HierarchyNode
	children map[string][]HierarchyNode
}

func NewHierarchy(nodes []HierarchyNode) (*Hierarchy, error) {
	h := &Hierarchy{nodes: make(map[string]HierarchyNode, len(nodes)), children: make(map[string][]HierarchyNode)}
	for _, node := range nodes {
		if node.EntryID == "" {
			return nil, errors.New("hierarchy entry ID is required")
		}
		if node.MenuOrder < 0 {
			return nil, fmt.Errorf("hierarchy entry %s has a negative menu order", node.EntryID)
		}
		if _, exists := h.nodes[node.EntryID]; exists {
			return nil, fmt.Errorf("duplicate hierarchy entry %s", node.EntryID)
		}
		h.nodes[node.EntryID] = node
	}
	for _, node := range h.nodes {
		if node.ParentEntryID == "" {
			continue
		}
		if node.ParentEntryID == node.EntryID {
			return nil, fmt.Errorf("entry %s cannot be its own parent", node.EntryID)
		}
		if _, ok := h.nodes[node.ParentEntryID]; !ok {
			return nil, fmt.Errorf("parent %s for entry %s does not exist", node.ParentEntryID, node.EntryID)
		}
		h.children[node.ParentEntryID] = append(h.children[node.ParentEntryID], node)
	}
	for id := range h.nodes {
		seen := map[string]bool{}
		for current := id; current != ""; current = h.nodes[current].ParentEntryID {
			if seen[current] {
				return nil, fmt.Errorf("hierarchy contains a cycle at entry %s", current)
			}
			seen[current] = true
		}
	}
	for parent := range h.children {
		sortNodes(h.children[parent])
	}
	return h, nil
}

func (h *Hierarchy) Node(entryID string) (HierarchyNode, bool) {
	node, ok := h.nodes[entryID]
	return node, ok
}

func (h *Hierarchy) Children(entryID string) []HierarchyNode {
	return append([]HierarchyNode(nil), h.children[entryID]...)
}

func (h *Hierarchy) Ancestors(entryID string) []HierarchyNode {
	var out []HierarchyNode
	for node, ok := h.Node(entryID); ok && node.ParentEntryID != ""; {
		parent, exists := h.Node(node.ParentEntryID)
		if !exists {
			break
		}
		out = append(out, parent)
		node = parent
	}
	return out
}

func (h *Hierarchy) Descendants(entryID string) []HierarchyNode {
	var out []HierarchyNode
	var visit func(string)
	visit = func(parent string) {
		for _, child := range h.children[parent] {
			out = append(out, child)
			visit(child.EntryID)
		}
	}
	visit(entryID)
	return out
}

func (h *Hierarchy) Depth(entryID string) int { return len(h.Ancestors(entryID)) }

func sortNodes(nodes []HierarchyNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].MenuOrder != nodes[j].MenuOrder {
			return nodes[i].MenuOrder < nodes[j].MenuOrder
		}
		if nodes[i].Title != nodes[j].Title {
			return nodes[i].Title < nodes[j].Title
		}
		return nodes[i].EntryID < nodes[j].EntryID
	})
}
