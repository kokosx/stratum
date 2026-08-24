package themes

import (
	"html/template"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/navigation"
)

func renderDefaultTheme(t *testing.T, nav map[string]navigation.Menu) string {
	t.Helper()
	def, err := loadDefaultDefinition()
	if err != nil {
		t.Fatalf("load default definition: %v", err)
	}
	settings, err := def.Schema.ValidateSettings(def.Schema.Defaults())
	if err != nil {
		t.Fatalf("validate defaults: %v", err)
	}
	view := PageView{
		Site:       SiteView{Title: "Acme", Language: "en"},
		Entry:      EntryView{Title: "Home", SEOTitle: "Home"},
		Theme:      ThemeView{ID: def.ID, Version: def.Version, Settings: settings},
		Navigation: nav,
		Content:    template.HTML("<h1>Hello</h1>"),
	}
	out, err := def.Render(view)
	if err != nil {
		t.Fatalf("render theme: %v", err)
	}
	return string(out)
}

func TestFooterMenuRendersNestedItemsAsListWithoutDropdown(t *testing.T) {
	nav := map[string]navigation.Menu{
		"footer": {Items: []navigation.MenuItem{
			{Label: "About", URL: "/about", Children: []navigation.MenuItem{
				{Label: "Team", URL: "/about/team"},
				{Label: "History", URL: "/about/history"},
			}},
			{Label: "Contact", URL: "/contact"},
		}},
		"primary": {Items: []navigation.MenuItem{
			{Label: "Products", URL: "/products", Children: []navigation.MenuItem{
				{Label: "Widgets", URL: "/products/widgets"},
			}},
		}},
	}

	html := renderDefaultTheme(t, nav)

	footerSection := html
	if idx := strings.Index(html, `class="site-footer`); idx >= 0 {
		// Isolate the footer markup for targeted checks.
		end := strings.Index(html[idx:], `</footer>`)
		if end >= 0 {
			footerSection = html[idx : idx+end+len(`</footer>`)]
		}
	}

	if !strings.Contains(footerSection, "footer-subnav") {
		t.Fatalf("footer with nested items should render a footer-subnav list; got:\n%s", footerSection)
	}
	if strings.Contains(footerSection, "submenu-toggle") {
		t.Fatalf("footer must not render header-style dropdown chevrons; got:\n%s", footerSection)
	}
	if strings.Contains(footerSection, `class="menu-item `) || strings.Contains(footerSection, `class="menu-item"`) {
		t.Fatalf("footer must not reuse primary-navigation item classes; got:\n%s", footerSection)
	}

	// The primary navigation keeps the dropdown chevron where children exist.
	if !strings.Contains(html, "submenu-toggle") {
		t.Fatalf("primary navigation with children should render a dropdown chevron")
	}
}

func TestPlainFooterLinkHasNoChevron(t *testing.T) {
	nav := map[string]navigation.Menu{
		"footer": {Items: []navigation.MenuItem{
			{Label: "Privacy", URL: "/privacy"},
		}},
	}
	html := renderDefaultTheme(t, nav)
	if strings.Contains(html, "submenu-toggle") {
		t.Fatalf("a plain footer link must not show a dropdown indicator")
	}
}
