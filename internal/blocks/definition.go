package blocks

type BlockKey struct {
	Name    string
	Version int64
}

type Definition struct {
	Namespace   string
	Name        string
	Version     int64
	DisplayName string
	Description string
	Schema      Schema
	Template    string
	Styles      string
	Source      string
	Enabled     bool
	// EditorContexts indicates where the block may be inserted (e.g. entry, layout-template).
	// Parsed from schema_json editor.contexts; defaults to ["entry"].
	EditorContexts []string
	// LCPCandidate indicates this block may be chosen as the LCP image.
	LCPCandidate bool
	// RequiresFeatured indicates the LCP candidate requires an entry featured image to be eligible.
	RequiresFeatured bool
	// SummaryFields indicates which props/settings are used for the editor's nodeSummary. Empty means use title fallback.
	SummaryFields []string
	// Hidden indicates the block should not appear in the inserter for new documents,
	// but historical documents containing it remain renderable.
	Hidden bool
}

type EditorDefinition struct {
	Block       string `json:"block"`
	Version     int64  `json:"version"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	Schema      Schema `json:"schema"`
}
