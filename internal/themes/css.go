package themes

import (
	"fmt"
	"strconv"
	"strings"
)

func (d *Definition) Styles(settings map[string]any, customCSS string) (string, error) {
	validated, err := d.Schema.ValidateSettings(settings)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	output.WriteString(":root {\n")
	for _, key := range sortedSettingKeys(d.Schema.Settings) {
		setting := d.Schema.Settings[key]
		if setting.Output == nil {
			continue
		}
		cssValue, err := cssValue(setting, validated[key])
		if err != nil {
			return "", fmt.Errorf("generate CSS for %s: %w", key, err)
		}
		fmt.Fprintf(&output, "  %s: %s;\n", setting.Output.CSSVariable, cssValue)
	}
	output.WriteString("}\n\n")
	output.WriteString(d.css)
	if customCSS != "" {
		output.WriteString("\n\n/* Custom CSS */\n")
		output.WriteString(customCSS)
		output.WriteByte('\n')
	}
	return output.String(), nil
}

func cssValue(setting SettingSchema, value any) (string, error) {
	if setting.Output == nil {
		return "", fmt.Errorf("setting has no CSS output")
	}
	switch setting.Type {
	case "number":
		number, ok := asFloat(value)
		if !ok {
			return "", fmt.Errorf("not a number")
		}
		return strconv.FormatFloat(number, 'f', -1, 64) + setting.Output.Unit, nil
	case "color":
		text, ok := value.(string)
		if !ok || !colorPattern.MatchString(text) {
			return "", fmt.Errorf("unsafe color")
		}
		return text, nil
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("not a string")
		}
		mapped, ok := setting.Output.Values[text]
		if !ok {
			return "", fmt.Errorf("no safe CSS mapping")
		}
		return mapped, nil
	default:
		return "", fmt.Errorf("unsupported CSS output type %s", setting.Type)
	}
}
