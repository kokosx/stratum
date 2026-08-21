package themes

import (
	"encoding/json"
	"html/template"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/navigation"
)

func defaultDefinition(t *testing.T) *Definition {
	t.Helper()
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition, err := registry.Exact("stratum/default", 1)
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func TestDefaultThemeSchemaV1Parses(t *testing.T) {
	definition := defaultDefinition(t)
	if definition.Schema.SchemaVersion != 1 || len(definition.Schema.Settings) < 70 {
		t.Fatalf("schema = version %d with %d settings", definition.Schema.SchemaVersion, len(definition.Schema.Settings))
	}
	if len(definition.Schema.MenuLocations) != 4 {
		t.Fatalf("menu locations = %v", definition.Schema.MenuLocations)
	}
}

func TestUnknownSchemaVersionRejected(t *testing.T) {
	_, err := ParseSchema([]byte(`{"schemaVersion":2,"id":"test/theme","version":1,"name":"Test","groups":[],"settings":{}}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported theme schema version") {
		t.Fatalf("error = %v", err)
	}
}

func TestSettingsValidationAndDefaults(t *testing.T) {
	schema := defaultDefinition(t).Schema
	valid, err := schema.ValidateSettings(map[string]any{"colors.primary": "#123456", "layout.contentWidth": json.Number("1200"), "header.layout": "split"})
	if err != nil {
		t.Fatal(err)
	}
	if valid["colors.primary"] != "#123456" || valid["header.layout"] != "split" || valid["colors.background"] != "#ffffff" {
		t.Fatalf("validation/defaults = %#v", valid)
	}
	for name, values := range map[string]map[string]any{
		"type":    {"header.sticky": "yes"},
		"enum":    {"header.layout": "injected class"},
		"range":   {"layout.contentWidth": 99999},
		"color":   {"colors.primary": "red;body{display:none}"},
		"unknown": {"header.injectCSS": "body{}"},
	} {
		if _, err := schema.ValidateSettings(values); err == nil {
			t.Errorf("%s value was accepted", name)
		}
	}
}

func TestCSSVariablesAreSafeAndStructuralSettingsAreNotEmitted(t *testing.T) {
	definition := defaultDefinition(t)
	settings, err := definition.Schema.ValidateSettings(map[string]any{"colors.primary": "#123456", "layout.contentWidth": 1234, "header.layout": "stacked"})
	if err != nil {
		t.Fatal(err)
	}
	css, err := definition.Styles(settings, ".custom{display:grid}")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--st-color-primary: #123456;", "--st-content-width: 1234px;", ".custom{display:grid}"} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS missing %q", want)
		}
	}
	if strings.Contains(css, "--st-header-layout") || strings.Contains(css, "stacked;") {
		t.Fatalf("structural setting emitted as CSS: %s", css)
	}
}

func TestThemeTemplateUsesStructuralSettingsMenusAndEscaping(t *testing.T) {
	definition := defaultDefinition(t)
	settings, err := definition.Schema.ValidateSettings(map[string]any{"header.layout": "centered", "footer.layout": "columns"})
	if err != nil {
		t.Fatal(err)
	}
	view := PageView{
		Site:  SiteView{Title: `<script>alert(1)</script>`, Language: "en"},
		Entry: EntryView{Title: "Page"}, Theme: ThemeView{ID: definition.ID, Version: definition.Version, Settings: settings},
		Navigation: map[string]navigation.Menu{
			"primary": {Items: []navigation.MenuItem{{Label: "Current", URL: "/", Current: true, Children: []navigation.MenuItem{{Label: "Child", URL: "/child"}}}}},
			"footer":  {Items: []navigation.MenuItem{{Label: "Footer", URL: "/footer"}}},
		},
		Content: template.HTML(`<p>same rendered document</p>`),
	}
	output, err := definition.Render(view)
	if err != nil {
		t.Fatal(err)
	}
	html := string(output)
	for _, want := range []string{"site-header--centered", "site-footer--columns", `aria-current="page"`, "Child", "Footer", "same rendered document", "&lt;script&gt;alert(1)&lt;/script&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("render missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, `<script>alert(1)</script>`) {
		t.Fatal("site title was not HTML escaped")
	}
}

func TestPresetValuesValidateAndResetUsesDefaults(t *testing.T) {
	schema := defaultDefinition(t).Schema
	draft := schema.Defaults()
	var bold Preset
	for _, preset := range schema.Presets {
		if preset.ID == "bold" {
			bold = preset
		}
	}
	for key, value := range bold.Values {
		draft[key] = value
	}
	validated, err := schema.ValidateSettings(draft)
	if err != nil {
		t.Fatal(err)
	}
	if validated["header.layout"] != "split" || validated["colors.primary"] == schema.Defaults()["colors.primary"] {
		t.Fatalf("bold preset not applied: %#v", validated)
	}
	reset, err := schema.ValidateSettings(map[string]any{})
	if err != nil || reset["header.layout"] != schema.Defaults()["header.layout"] {
		t.Fatalf("reset defaults = %#v, %v", reset, err)
	}
}
