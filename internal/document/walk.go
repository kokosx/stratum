package document

import "encoding/json"

// Walk traverses every node in a document depth-first, calling fn for each.
// If fn returns an error, Walk stops and returns it. The traversal order is
// pre-order (parent before children) which matches rendering order.
func Walk(doc *Document, fn func(Node) error) error {
	if doc == nil {
		return nil
	}
	var walk func([]Node) error
	walk = func(nodes []Node) error {
		for _, n := range nodes {
			if err := fn(n); err != nil {
				return err
			}
			if len(n.Children) > 0 {
				if err := walk(n.Children); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(doc.Nodes)
}

// Find returns the first node with the given ID, or nil if not found.
func Find(doc *Document, id string) *Node {
	if doc == nil {
		return nil
	}
	var found *Node
	_ = Walk(doc, func(n Node) error {
		if n.ID == id {
			cp := n
			found = &cp
			return errStop
		}
		return nil
	})
	return found
}

var errStop = errWalkStop{}

type errWalkStop struct{}

func (e errWalkStop) Error() string { return "stop" }

// Transform returns a deep copy of doc where each node is transformed by fn.
// fn may return a modified node; returning the same node leaves it unchanged.
func Transform(doc *Document, fn func(Node) Node) *Document {
	if doc == nil {
		return nil
	}
	cp := Clone(doc)
	var apply func([]Node) []Node
	apply = func(nodes []Node) []Node {
		out := make([]Node, len(nodes))
		for i, n := range nodes {
			n = fn(n)
			n.Children = apply(n.Children)
			out[i] = n
		}
		return out
	}
	cp.Nodes = apply(cp.Nodes)
	return cp
}

// Clone performs a deep copy via JSON round-trip. It is correct for all
// document shapes and preserves props/settings as raw JSON. For performance
// critical paths use CloneFast when available.
func Clone(doc *Document) *Document {
	if doc == nil {
		return nil
	}
	data, _ := json.Marshal(doc)
	var out Document
	_ = json.Unmarshal(data, &out)
	return &out
}

// CloneNodes performs a deep copy of a node slice.
func CloneNodes(nodes []Node) []Node {
	data, _ := json.Marshal(nodes)
	var out []Node
	_ = json.Unmarshal(data, &out)
	return out
}

// Count returns the total number of nodes in the document.
func Count(doc *Document) int {
	if doc == nil {
		return 0
	}
	c := 0
	_ = Walk(doc, func(Node) error { c++; return nil })
	return c
}
