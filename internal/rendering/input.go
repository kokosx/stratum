package rendering

import "github.com/kokosx/stratum/internal/document"

// RenderInput describes an arbitrary document to render through the public
// pipeline outside of a published entry (the block editor preview). It is the
// shared contract between admin preview and public rendering so admin does not
// depend on the public HTTP package (dependency direction: both -> rendering).
type RenderInput struct {
	Document         *document.Document
	Title            string
	Slug             string
	Excerpt          string
	SEOTitle         string
	SEODescription   string
	Path             string
	EntryID          string // optional: entry being edited, for Posts Page preview
	Temporary        map[string]any
	CustomCSS        string
	LayoutTemplateID string // optional: selected layout template for preview
	ContentTypeID    string
	Fields           map[string]any
	FeaturedMediaID  string
	// Archive supplies the already-resolved page slice for Archive Template
	// previews. When set, Document is rendered directly without composition.
	Archive *ArchiveContext
	// HeaderDocument/FooterDocument optionally replace the corresponding
	// published theme region for a no-store editor preview.
	HeaderDocument *document.Document
	FooterDocument *document.Document
}
