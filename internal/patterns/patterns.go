package patterns

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
)

// Pattern is a reusable starter composition. Inserting a pattern copies its
// document with regenerated stable IDs.
type Pattern struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Contexts    []string          `json:"contexts"`
	Source      string            `json:"source"`
	Document    document.Document `json:"document"`
}

// Catalog holds the bundled patterns.
type Catalog struct {
	byID    map[string]Pattern
	ordered []Pattern
}

// NewCatalog returns the core catalog. It validates every pattern against the
// block registry contract (valid SDT, known block versions, unique internal IDs).
func NewCatalog() *Catalog {
	c := &Catalog{byID: make(map[string]Pattern)}
	for _, p := range corePatterns() {
		cp := p
		c.byID[p.ID] = cp
		c.ordered = append(c.ordered, cp)
	}
	return c
}

// List returns patterns valid for the given editor context. Empty context returns all.
func (c *Catalog) List(context string) []Pattern {
	if context == "" {
		out := make([]Pattern, len(c.ordered))
		copy(out, c.ordered)
		return out
	}
	var out []Pattern
	for _, p := range c.ordered {
		if isContextAllowed(p.Contexts, context) {
			out = append(out, p)
		}
	}
	return out
}

// Get returns a pattern by ID.
func (c *Catalog) Get(id string) (Pattern, bool) {
	p, ok := c.byID[id]
	return p, ok
}

// CloneWithNewIDs deep-copies the pattern document and regenerates every node ID.
func (p Pattern) CloneWithNewIDs() (*document.Document, error) {
	clone := document.Clone(&p.Document)
	if clone == nil {
		return nil, fmt.Errorf("pattern %s has empty document", p.ID)
	}
	if err := regenerateIDs(clone); err != nil {
		return nil, err
	}
	if err := document.Validate(clone); err != nil {
		return nil, fmt.Errorf("cloned pattern %s validation failed: %w", p.ID, err)
	}
	return clone, nil
}

// CloneDocumentWithNewIDs clones any document with fresh IDs.
func CloneDocumentWithNewIDs(doc *document.Document) (*document.Document, error) {
	clone := document.Clone(doc)
	if clone == nil {
		return nil, fmt.Errorf("document is nil")
	}
	if err := regenerateIDs(clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func regenerateIDs(doc *document.Document) error {
	seen := make(map[string]bool)
	var walk func([]document.Node) ([]document.Node, error)
	walk = func(nodes []document.Node) ([]document.Node, error) {
		out := make([]document.Node, len(nodes))
		for i, n := range nodes {
			newID, err := randomID()
			if err != nil {
				return nil, err
			}
			// ensure uniqueness within this clone
			for seen[newID] {
				newID, err = randomID()
				if err != nil {
					return nil, err
				}
			}
			seen[newID] = true
			children, err := walk(n.Children)
			if err != nil {
				return nil, err
			}
			n.ID = newID
			n.Children = children
			out[i] = n
		}
		return out, nil
	}
	nodes, err := walk(doc.Nodes)
	if err != nil {
		return err
	}
	doc.Nodes = nodes
	return nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

var canonicalContexts = map[string]bool{
	"entry":            true,
	"single-template":  true,
	"archive-template": true,
	"site-part":        true,
}

func isContextAllowed(contexts []string, mode string) bool {
	if len(contexts) == 0 {
		return true
	}
	for _, c := range contexts {
		if c == mode {
			return true
		}
	}
	return false
}

// ValidateAll checks every bundled pattern decodes, has unique internal IDs,
// uses known block versions, passes block registry validation when a registry is supplied,
// and is valid in every declared context.
func (c *Catalog) ValidateAll(reg *blocks.Registry) error {
	for _, p := range c.ordered {
		// Validate contexts are canonical
		for _, ctx := range p.Contexts {
			if !canonicalContexts[ctx] {
				return fmt.Errorf("pattern %s has invalid context %q", p.ID, ctx)
			}
		}
		if len(p.Contexts) == 0 {
			return fmt.Errorf("pattern %s has no contexts", p.ID)
		}
		// Check unique IDs inside pattern
		ids := make(map[string]bool)
		if err := document.Walk(&p.Document, func(n document.Node) error {
			if ids[n.ID] {
				return fmt.Errorf("pattern %s has duplicate node id %q", p.ID, n.ID)
			}
			ids[n.ID] = true
			return nil
		}); err != nil {
			return err
		}
		if err := document.Validate(&p.Document); err != nil {
			return fmt.Errorf("pattern %s invalid document: %w", p.ID, err)
		}
		if reg != nil {
			if err := reg.ValidateDocument(&p.Document); err != nil {
				return fmt.Errorf("pattern %s block validation: %w", p.ID, err)
			}
			// Must be valid in every declared context
			for _, ctx := range p.Contexts {
				if err := reg.ValidateDocumentForContext(&p.Document, ctx); err != nil {
					return fmt.Errorf("pattern %s invalid for context %q: %w", p.ID, ctx, err)
				}
			}
		}
	}
	return nil
}

// MustValidate panics if bundled patterns are invalid. Useful for init.
func (c *Catalog) MustValidate(reg *blocks.Registry) {
	if err := c.ValidateAll(reg); err != nil {
		panic(err)
	}
}

// Helpers for building patterns without hand-writing JSON.

func mustNodes(raw string) []document.Node {
	var nodes []document.Node
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		panic(fmt.Sprintf("pattern nodes unmarshal: %v", err))
	}
	return nodes
}

func doc(nodes []document.Node) document.Document {
	return document.Document{Version: 1, Nodes: nodes}
}
