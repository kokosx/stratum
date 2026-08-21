package themes

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
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
	var output bytes.Buffer
	if err := d.template.ExecuteTemplate(&output, "layout.html", view); err != nil {
		return nil, fmt.Errorf("render theme %s@%d: %w", d.ID, d.Version, err)
	}
	return output.Bytes(), nil
}
