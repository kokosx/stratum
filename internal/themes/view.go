package themes

import (
	"html/template"

	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/rendering"
)

type SiteView struct {
	Title    string
	Tagline  string
	Language string
	SiteURL  string
}

type EntryView struct {
	Title          string
	SEOTitle       string
	SEODescription string
	CanonicalURL   string
}

// HeadView is the stable, semantic contract the theme uses to render the
// document <head>. The CMS supplies the data; the theme controls final markup.
type HeadView struct {
	Title       string
	Description string
	Canonical   string
	// Robots is empty when indexing is allowed; otherwise it holds the
	// comma-separated robots directive (e.g. "noindex,nofollow").
	Robots string
	// Speculation carries the generated Navigation Preloading configuration.
	Speculation SpeculationView
	// SiteIcon carries the generated favicon links, or nil when no Site Icon is set.
	SiteIcon *rendering.FaviconView
}

// SpeculationView exposes the safe, server-generated Speculation Rules payload.
// RulesJSON is produced with encoding/json and must never be built by string
// concatenation.
type SpeculationView struct {
	Enabled   bool
	Mode      string
	Eagerness string
	RulesJSON template.JS
}

type ThemeView struct {
	ID       string
	Version  int
	Settings map[string]any
}

type PageView struct {
	Site       SiteView
	Entry      EntryView
	Head       HeadView
	Theme      ThemeView
	Navigation map[string]navigation.Menu
	Content    template.HTML
	// PreviewCSS is generated exclusively by Theme Runtime after server-side
	// validation. Public renders leave it empty and use /stratum/theme.css.
	PreviewCSS template.CSS
}
