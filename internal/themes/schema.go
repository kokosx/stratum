package themes

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const SchemaVersion = 1

type ThemeSchema struct {
	SchemaVersion int                      `json:"schemaVersion"`
	ID            string                   `json:"id"`
	Version       int                      `json:"version"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	MenuLocations []MenuLocation           `json:"menuLocations,omitempty"`
	Groups        []SettingGroup           `json:"groups"`
	Settings      map[string]SettingSchema `json:"settings"`
	Presets       []Preset                 `json:"presets,omitempty"`
}

type MenuLocation struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type SettingGroup struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type SettingSchema struct {
	Type      string         `json:"type"`
	Default   any            `json:"default"`
	Enum      []string       `json:"enum,omitempty"`
	Minimum   *float64       `json:"minimum,omitempty"`
	Maximum   *float64       `json:"maximum,omitempty"`
	MaxLength int            `json:"maxLength,omitempty"`
	UI        SettingUI      `json:"ui"`
	Output    *SettingOutput `json:"output,omitempty"`
}

type SettingUI struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group"`
	Control     string `json:"control"`
}

type SettingOutput struct {
	CSSVariable string            `json:"cssVariable"`
	Unit        string            `json:"unit,omitempty"`
	Values      map[string]string `json:"values,omitempty"`
}

type Preset struct {
	ID     string         `json:"id"`
	Label  string         `json:"label"`
	Values map[string]any `json:"values"`
}

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9]+(?:[._/-][a-z0-9]+)*$`)
	settingPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(?:\.[a-z0-9][a-zA-Z0-9]*)+$`)
	cssVarPattern  = regexp.MustCompile(`^--st-[a-z0-9-]+$`)
	colorPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	supportedTypes = map[string]bool{"string": true, "text": true, "color": true, "number": true, "boolean": true}
	supportedUI    = map[string]bool{"color": true, "text": true, "number": true, "range": true, "checkbox": true, "select": true, "segmented": true, "radio": true, "font": true}
	supportedUnits = map[string]bool{"": true, "px": true, "rem": true, "%": true}
)

func ParseSchema(data []byte) (ThemeSchema, error) {
	var schema ThemeSchema
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schema); err != nil {
		return ThemeSchema{}, fmt.Errorf("parse theme schema: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ThemeSchema{}, fmt.Errorf("parse theme schema: %w", err)
	}
	if err := schema.validate(); err != nil {
		return ThemeSchema{}, err
	}
	return schema, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func (s ThemeSchema) validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported theme schema version %d", s.SchemaVersion)
	}
	if !idPattern.MatchString(s.ID) || s.Version < 1 || strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("invalid theme identity")
	}
	groups := make(map[string]bool, len(s.Groups))
	for _, group := range s.Groups {
		if !idPattern.MatchString(group.ID) || group.Label == "" || groups[group.ID] {
			return fmt.Errorf("invalid or duplicate theme group %q", group.ID)
		}
		groups[group.ID] = true
	}
	locations := map[string]bool{}
	for _, location := range s.MenuLocations {
		if !idPattern.MatchString(location.ID) || location.Label == "" || locations[location.ID] {
			return fmt.Errorf("invalid or duplicate menu location %q", location.ID)
		}
		locations[location.ID] = true
	}
	outputVariables := map[string]string{}
	for key, setting := range s.Settings {
		if !settingPattern.MatchString(key) {
			return fmt.Errorf("invalid setting key %q", key)
		}
		if !supportedTypes[setting.Type] || !supportedUI[setting.UI.Control] || !groups[setting.UI.Group] || setting.UI.Label == "" {
			return fmt.Errorf("setting %s has invalid type or UI", key)
		}
		if setting.Minimum != nil && setting.Maximum != nil && *setting.Minimum > *setting.Maximum {
			return fmt.Errorf("setting %s has invalid numeric range", key)
		}
		if setting.Output != nil {
			if !cssVarPattern.MatchString(setting.Output.CSSVariable) || !supportedUnits[setting.Output.Unit] {
				return fmt.Errorf("setting %s has unsafe CSS output", key)
			}
			if previous := outputVariables[setting.Output.CSSVariable]; previous != "" {
				return fmt.Errorf("settings %s and %s emit the same CSS variable", previous, key)
			}
			outputVariables[setting.Output.CSSVariable] = key
			if setting.Type != "number" && setting.Output.Unit != "" {
				return fmt.Errorf("setting %s uses a unit for a non-number", key)
			}
			if setting.Type != "number" && setting.Type != "color" && setting.Type != "string" {
				return fmt.Errorf("setting %s type cannot emit CSS", key)
			}
			if setting.Type == "string" && len(setting.Output.Values) == 0 {
				return fmt.Errorf("setting %s string CSS output requires a value map", key)
			}
			for _, enumValue := range setting.Enum {
				if setting.Type == "string" && setting.Output.Values[enumValue] == "" {
					return fmt.Errorf("setting %s has no CSS mapping for %q", key, enumValue)
				}
			}
		}
		if _, err := validateValue(key, setting, setting.Default); err != nil {
			return fmt.Errorf("invalid default: %w", err)
		}
	}
	seenPresets := map[string]bool{}
	for _, preset := range s.Presets {
		if !idPattern.MatchString(preset.ID) || preset.Label == "" || seenPresets[preset.ID] {
			return fmt.Errorf("invalid or duplicate preset %q", preset.ID)
		}
		seenPresets[preset.ID] = true
		for key, value := range preset.Values {
			setting, ok := s.Settings[key]
			if !ok {
				return fmt.Errorf("preset %s references unknown setting %s", preset.ID, key)
			}
			if _, err := validateValue(key, setting, value); err != nil {
				return fmt.Errorf("preset %s: %w", preset.ID, err)
			}
		}
	}
	return nil
}

func (s ThemeSchema) Defaults() map[string]any {
	result := make(map[string]any, len(s.Settings))
	for key, setting := range s.Settings {
		result[key] = normalizeJSONValue(setting.Default)
	}
	return result
}

// ValidateSettings rejects unknown keys and returns a complete, normalized map.
func (s ThemeSchema) ValidateSettings(values map[string]any) (map[string]any, error) {
	result := s.Defaults()
	for key, value := range values {
		setting, ok := s.Settings[key]
		if !ok {
			return nil, fmt.Errorf("unknown theme setting %q", key)
		}
		normalized, err := validateValue(key, setting, value)
		if err != nil {
			return nil, err
		}
		result[key] = normalized
	}
	return result, nil
}

func validateValue(key string, setting SettingSchema, value any) (any, error) {
	switch setting.Type {
	case "number":
		number, ok := asFloat(value)
		if !ok {
			return nil, fmt.Errorf("setting %s must be a number", key)
		}
		if setting.Minimum != nil && number < *setting.Minimum || setting.Maximum != nil && number > *setting.Maximum {
			return nil, fmt.Errorf("setting %s is outside its allowed range", key)
		}
		return number, nil
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("setting %s must be a boolean", key)
		}
		return boolean, nil
	case "string", "text", "color":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("setting %s must be a string", key)
		}
		if setting.Type == "color" && !colorPattern.MatchString(text) {
			return nil, fmt.Errorf("setting %s must be a six-digit hex color", key)
		}
		if setting.MaxLength > 0 && len(text) > setting.MaxLength {
			return nil, fmt.Errorf("setting %s is too long", key)
		}
		if len(setting.Enum) > 0 && !contains(setting.Enum, text) {
			return nil, fmt.Errorf("setting %s is not an allowed value", key)
		}
		return text, nil
	default:
		return nil, fmt.Errorf("setting %s has unsupported type", key)
	}
}

func asFloat(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func normalizeJSONValue(value any) any {
	if number, ok := value.(json.Number); ok {
		parsed, _ := number.Float64()
		return parsed
	}
	return value
}

func sortedSettingKeys(settings map[string]SettingSchema) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
