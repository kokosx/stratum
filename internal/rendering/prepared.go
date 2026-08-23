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
	// LegacySource is non-empty when this node originated as a legacy block
	// (e.g. core/posts@1) and was migrated in-memory to a runtime block.
	// It is runtime-only metadata, never persisted to the revision.
	LegacySource string
}

// LCPCandidate describes one image block discovered during prepare that may
// become the LCP image. Final selection (filtering featured-image blocks that
// have no actual image on the Entry) is performed at render time.
type LCPCandidate struct {
	ID               string
	Block            string
	RequiresFeatured bool
}

// PreparedDocument is the render-only representation of a document. It is built
// once per published revision (and cached) instead of on every request.
type PreparedDocument struct {
	Nodes          []PreparedNode
	UsedBlocks     []BlockKey
	HighPriority   []LCPCandidate
	AutoCandidates []LCPCandidate
	// LCPCandidate kept for backward compat inside tests that directly inspect it.
	// New code must call ResolveLCP.
	LCPCandidate string
}

// ResolveLCP returns the final LCP node ID according to the policy:
//
//	explicit high that actually exists (featured only if Entry has image) first,
//	then first existing auto candidate,
//	else none.
func (pd *PreparedDocument) ResolveLCP(hasFeaturedImage bool) string {
	for _, c := range pd.HighPriority {
		if !c.RequiresFeatured || hasFeaturedImage {
			return c.ID
		}
	}
	for _, c := range pd.AutoCandidates {
		if !c.RequiresFeatured || hasFeaturedImage {
			return c.ID
		}
	}
	return ""
}

// BlockKey identifies the exact versioned definition whose CSS a document uses.
type BlockKey struct {
	Name    string
	Version int
}
