package blocks

// legacyMetadata holds explicit compatibility for blocks created before
// capabilities/editor contexts were first-class metadata. New blocks must
// declare their metadata in schema_json; old rows without it are backfilled
// here so generic layers never branch on block names.
var legacyMetadata = map[string]struct {
	Contexts         []string
	LCPCandidate     bool
	RequiresFeatured bool
	Hidden           bool
}{
	"core/content-slot": {
		Contexts: []string{"single-template"},
	},
	"core/image": {
		Contexts:     []string{"entry", "layout-template"},
		LCPCandidate: true, RequiresFeatured: false,
	},
	"core/featured-image": {
		Contexts:     []string{"entry", "layout-template"},
		LCPCandidate: true, RequiresFeatured: true,
	},
	"core/posts": {
		Contexts: []string{"entry", "layout-template"},
		Hidden:   true, // hidden from new inserts, historical renderer remains
	},
}

// isLegacyBlock reports whether block has legacy metadata.
func isLegacyBlock(block string) bool {
	_, ok := legacyMetadata[block]
	return ok
}
