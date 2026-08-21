package themes

import (
	"html/template"

	"github.com/kokosx/stratum/internal/navigation"
)

type SiteView struct {
	Title    string
	Tagline  string
	Language string
}

type EntryView struct {
	Title          string
	SEOTitle       string
	SEODescription string
}

type ThemeView struct {
	ID       string
	Version  int
	Settings map[string]any
}

type PageView struct {
	Site       SiteView
	Entry      EntryView
	Theme      ThemeView
	Navigation map[string]navigation.Menu
	Content    template.HTML
	// PreviewCSS is generated exclusively by Theme Runtime after server-side
	// validation. Public renders leave it empty and use /stratum/theme.css.
	PreviewCSS template.CSS
}
