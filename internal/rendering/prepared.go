package rendering

// PreparedNode is a block ready for template rendering. Props and Settings are
// already decoded into maps and defaults applied, so the renderer never touches
// JSON again. The Block and Version fields let the renderer look up the compiled
// template.
type PreparedNode struct {
	ID       string
	Block    string
	Version  int
	Props    map[string]any
	Settings map[string]any
	Children []PreparedNode
}

// PreparedDocument is the render-only representation of a document. It is built
// once per published revision (and cached) instead of on every request.
type PreparedDocument struct {
	Nodes        []PreparedNode
	UsedBlocks   []BlockKey
	LCPCandidate string
}

// BlockKey identifies the exact versioned definition whose CSS a document uses.
type BlockKey struct {
	Name    string
	Version int
}
