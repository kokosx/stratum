package rendering

import "github.com/kokosx/stratum/internal/document"

// RenderInput describes an arbitrary document to render through the public
// pipeline outside of a published entry (the block editor preview). It is the
// shared contract between admin preview and public rendering so admin does not
// depend on the public HTTP package (dependency direction: both -> rendering).
type RenderInput struct {
	Document         *document.Document
	Title            string
	Excerpt          string
	SEOTitle         string
	SEODescription   string
	Path             string
	EntryID          string // optional: entry being edited, for Posts Page preview
	Temporary        map[string]any
	CustomCSS        string
	LayoutTemplateID string // optional: selected layout template for preview
	ContentTypeID    string
}
