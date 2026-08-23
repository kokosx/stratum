package themes

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"github.com/kokosx/stratum/internal/content"
)

//go:embed default/theme.json default/templates/*.html default/templates/partials/*.html default/assets/*
var embeddedThemes embed.FS

type Definition struct {
	ID          string
	Version     int
	Name        string
	Description string
	Schema      ThemeSchema
	template    *template.Template
	css         string
	javascript  string
}

func loadDefaultDefinition() (*Definition, error) {
	manifest, err := embeddedThemes.ReadFile("default/theme.json")
	if err != nil {
		return nil, fmt.Errorf("read default theme manifest: %w", err)
	}
	schema, err := ParseSchema(manifest)
	if err != nil {
		return nil, err
	}
	functions := template.FuncMap{
		"setting": func(settings map[string]any, key string) any { return settings[key] },
	}
	tmpl, err := template.New("layout.html").Funcs(functions).ParseFS(embeddedThemes,
		"default/templates/*.html", "default/templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("compile default theme templates: %w", err)
	}
	css, err := embeddedThemes.ReadFile("default/assets/theme.css")
	if err != nil {
		return nil, fmt.Errorf("read default theme CSS: %w", err)
	}
	javascript, err := embeddedThemes.ReadFile("default/assets/theme.js")
	if err != nil {
		return nil, fmt.Errorf("read default theme JavaScript: %w", err)
	}
	return &Definition{ID: schema.ID, Version: schema.Version, Name: schema.Name, Description: schema.Description, Schema: schema, template: tmpl, css: string(css), javascript: string(javascript)}, nil
}

func (d *Definition) Render(view PageView) ([]byte, error) {
	name := d.resolveTarget(view)
	var output bytes.Buffer
	if err := d.template.ExecuteTemplate(&output, name, view); err != nil {
		if name != "layout.html" {
			// fallback to layout
			output.Reset()
			if ferr := d.template.ExecuteTemplate(&output, "layout.html", view); ferr == nil {
				return output.Bytes(), nil
			}
		}
		return nil, fmt.Errorf("render theme %s@%d: %w", d.ID, d.Version, err)
	}
	return output.Bytes(), nil
}

func (d *Definition) resolveTarget(view PageView) string {
	def := content.DefinitionFor(view.ContentType)
	switch view.Kind {
	case PageKindArchive:
		// Generic fallback: archive-{contentType} → archive → layout
		patterns := def.Templates.ArchivePatterns
		if len(patterns) == 0 && def.IsArchived() {
			patterns = []string{"archive-" + string(def.ID), "archive"}
		} else if len(patterns) == 0 {
			patterns = []string{"archive"}
		}
		// Normalize to .html suffix and add layout fallback
		candidates := make([]string, 0, len(patterns)+1)
		for _, p := range patterns {
			if p == "archive" || p == "single" {
				candidates = append(candidates, p+".html")
			} else if len(p) > 5 && p[:8] == "archive-" {
				candidates = append(candidates, p+".html")
			} else if len(p) > 7 && p[:7] == "single-" {
				candidates = append(candidates, p+".html")
			} else {
				candidates = append(candidates, p+".html")
			}
		}
		candidates = append(candidates, "layout.html")
		return firstPresent(d.template, candidates...)
	default:
		patterns := def.Templates.SinglePatterns
		if len(patterns) == 0 {
			patterns = []string{"single-" + string(def.ID), "single"}
		}
		candidates := make([]string, 0, len(patterns)+1)
		for _, p := range patterns {
			candidates = append(candidates, p+".html")
		}
		candidates = append(candidates, "layout.html")
		return firstPresent(d.template, candidates...)
	}
}

func firstPresent(t *template.Template, names ...string) string {
	for _, n := range names {
		if t.Lookup(n) != nil {
			return n
		}
	}
	return "layout.html"
}
