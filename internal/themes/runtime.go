package themes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"sync"
	"sync/atomic"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const MaxCustomCSSBytes = 200 * 1024

type Runtime struct {
	queries  *db.Queries
	registry *Registry
	reloadMu sync.Mutex
	snapshot atomic.Pointer[runtimeSnapshot]
}

type runtimeSnapshot struct {
	definition *Definition
	settings   map[string]any
	customCSS  string
	styles     string
}

type Customization struct {
	ThemeID     string         `json:"themeID"`
	Version     int            `json:"themeVersion"`
	Settings    map[string]any `json:"settings"`
	CustomCSS   string         `json:"customCSS"`
	Schema      ThemeSchema    `json:"schema"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
}

func NewRuntime(ctx context.Context, queries *db.Queries) (*Runtime, error) {
	registry, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{queries: queries, registry: registry}
	if err := runtime.Reload(ctx); err != nil {
		return nil, err
	}
	return runtime, nil
}

func (r *Runtime) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	site, err := r.queries.GetSiteSettings(ctx)
	if err != nil {
		return fmt.Errorf("load active theme: %w", err)
	}
	definition, err := r.registry.Current(site.ActiveTheme)
	if err != nil {
		return err
	}
	stored, err := r.queries.GetThemeCustomization(ctx, definition.ID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load theme customization: %w", err)
	}
	values := map[string]any{}
	customCSS := ""
	if err == nil {
		if int(stored.ThemeVersion) != definition.Version {
			return fmt.Errorf("theme customization %s@%d requires migration to version %d", stored.ThemeID, stored.ThemeVersion, definition.Version)
		}
		values, err = decodeSettingsJSON([]byte(stored.SettingsJson))
		if err != nil {
			return fmt.Errorf("decode stored theme settings: %w", err)
		}
		customCSS = stored.CustomCss
	}
	validated, err := definition.Schema.ValidateSettings(values)
	if err != nil {
		return fmt.Errorf("validate stored theme settings: %w", err)
	}
	styles, err := definition.Styles(validated, customCSS)
	if err != nil {
		return err
	}
	r.snapshot.Store(&runtimeSnapshot{definition: definition, settings: validated, customCSS: customCSS, styles: styles})
	return nil
}

func (r *Runtime) Current() Customization {
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return Customization{}
	}
	return Customization{ThemeID: snapshot.definition.ID, Version: snapshot.definition.Version, Settings: cloneSettings(snapshot.settings), CustomCSS: snapshot.customCSS, Schema: snapshot.definition.Schema, Name: snapshot.definition.Name, Description: snapshot.definition.Description}
}

func (r *Runtime) Render(view PageView, temporary map[string]any) ([]byte, error) {
	return r.render(view, temporary, nil)
}

// Preview renders with ephemeral settings and custom CSS without publishing
// either value to the atomic runtime snapshot.
func (r *Runtime) Preview(view PageView, temporary map[string]any, customCSS string) ([]byte, error) {
	if len(customCSS) > MaxCustomCSSBytes {
		return nil, fmt.Errorf("custom CSS exceeds the %d byte limit", MaxCustomCSSBytes)
	}
	return r.render(view, temporary, &customCSS)
}

func (r *Runtime) render(view PageView, temporary map[string]any, temporaryCSS *string) ([]byte, error) {
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return nil, fmt.Errorf("theme runtime is not initialized")
	}
	settings := snapshot.settings
	if temporary != nil {
		validated, err := snapshot.definition.Schema.ValidateSettings(temporary)
		if err != nil {
			return nil, err
		}
		settings = validated
		customCSS := snapshot.customCSS
		if temporaryCSS != nil {
			customCSS = *temporaryCSS
		}
		styles, err := snapshot.definition.Styles(settings, customCSS)
		if err != nil {
			return nil, err
		}
		view.PreviewCSS = template.CSS(styles)
	}
	view.Theme = ThemeView{ID: snapshot.definition.ID, Version: snapshot.definition.Version, Settings: settings}
	return snapshot.definition.Render(view)
}

func (r *Runtime) Styles() string {
	if snapshot := r.snapshot.Load(); snapshot != nil {
		return snapshot.styles
	}
	return ""
}

func (r *Runtime) JavaScript() string {
	if snapshot := r.snapshot.Load(); snapshot != nil {
		return snapshot.definition.javascript
	}
	return ""
}

func (r *Runtime) ValidateSettings(values map[string]any) (map[string]any, error) {
	if snapshot := r.snapshot.Load(); snapshot != nil {
		return snapshot.definition.Schema.ValidateSettings(values)
	}
	return nil, fmt.Errorf("theme runtime is not initialized")
}

func (r *Runtime) Save(ctx context.Context, values map[string]any, customCSS string) error {
	if len(customCSS) > MaxCustomCSSBytes {
		return fmt.Errorf("custom CSS exceeds the %d byte limit", MaxCustomCSSBytes)
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return fmt.Errorf("theme runtime is not initialized")
	}
	validated, err := snapshot.definition.Schema.ValidateSettings(values)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(validated)
	if err != nil {
		return fmt.Errorf("encode theme settings: %w", err)
	}
	if err := r.queries.UpsertThemeCustomization(ctx, db.UpsertThemeCustomizationParams{ThemeID: snapshot.definition.ID, ThemeVersion: int64(snapshot.definition.Version), SettingsJson: string(encoded), CustomCss: customCSS}); err != nil {
		return fmt.Errorf("save theme customization: %w", err)
	}
	return r.Reload(ctx)
}

func decodeSettingsJSON(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	if values == nil {
		return nil, fmt.Errorf("settings must be an object")
	}
	return values, nil
}

func cloneSettings(values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
