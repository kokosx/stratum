package document

import "encoding/json"

// Migration is a deterministic function that upgrades a single node from one
// version to the next. It receives the node's block name, its current version,
// and the raw props/settings/children and returns the upgraded values.
type Migration func(node Node) (Node, error)

// MigrationRegistry holds versioned migration chains per block.
// It is used to upgrade historical documents (e.g. core/posts@1 → core/collection)
// without ever deleting support for old published content.
type MigrationRegistry struct {
	chains map[string]map[int]Migration // block -> fromVersion -> migration
}

// NewMigrationRegistry creates an empty registry.
func NewMigrationRegistry() *MigrationRegistry {
	return &MigrationRegistry{chains: make(map[string]map[int]Migration)}
}

// Register adds a migration for block@version → version+1.
func (r *MigrationRegistry) Register(block string, fromVersion int, fn Migration) {
	if r.chains[block] == nil {
		r.chains[block] = make(map[int]Migration)
	}
	r.chains[block][fromVersion] = fn
}

// CanMigrate reports whether a migrator is registered for block@version.
func (r *MigrationRegistry) CanMigrate(block string, version int) bool {
	if r.chains[block] == nil {
		return false
	}
	_, ok := r.chains[block][version]
	return ok
}

// MigrateNode upgrades a single node deterministically through its migration
// chain until no further migration is registered. The returned node has the
// latest known version for its block.
func (r *MigrationRegistry) MigrateNode(node Node) (Node, error) {
	for {
		chain, ok := r.chains[node.Block]
		if !ok {
			return node, nil
		}
		mig, ok := chain[node.Version]
		if !ok {
			return node, nil
		}
		upgraded, err := mig(node)
		if err != nil {
			return Node{}, err
		}
		node = upgraded
	}
}

// MigrateDocument upgrades every node in doc deterministically.
func (r *MigrationRegistry) MigrateDocument(doc *Document) (*Document, error) {
	if doc == nil {
		return nil, nil
	}
	cp := Clone(doc)
	var walk func([]Node) ([]Node, error)
	walk = func(nodes []Node) ([]Node, error) {
		out := make([]Node, len(nodes))
		for i, n := range nodes {
			migrated, err := r.MigrateNode(n)
			if err != nil {
				return nil, err
			}
			children, err := walk(migrated.Children)
			if err != nil {
				return nil, err
			}
			migrated.Children = children
			out[i] = migrated
		}
		return out, nil
	}
	migratedNodes, err := walk(cp.Nodes)
	if err != nil {
		return nil, err
	}
	cp.Nodes = migratedNodes
	return cp, nil
}

// MustMarshal is a helper for tests: it marshals v to json.RawMessage or panics.
func MustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}
