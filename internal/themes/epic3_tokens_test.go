package themes

import (
	"strings"
	"testing"
)

func TestSiteStylesCSSVariablesSafe(t *testing.T) {
	def, err := loadDefaultDefinition()
	if err != nil {
		t.Fatalf("load default: %v", err)
	}
	// Malicious color value must be rejected, not emitted
	malicious := map[string]any{
		"colors.primary": "red;}body{display:none",
	}
	if _, err := def.Schema.ValidateSettings(malicious); err == nil {
		t.Fatalf("expected validation to reject CSS injection")
	}
	// Valid color must emit correct variable
	valid := map[string]any{
		"colors.primary": "#123456",
	}
	validated, err := def.Schema.ValidateSettings(valid)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	css, err := def.Styles(validated, "")
	if err != nil {
		t.Fatalf("styles: %v", err)
	}
	if !strings.Contains(css, "--st-color-primary: #123456") {
		t.Fatalf("css missing expected variable: %s", css)
	}
	if strings.Contains(css, "display:none") {
		t.Fatalf("injection leaked into css")
	}
}

func TestSiteStylesTokensExist(t *testing.T) {
	def, err := loadDefaultDefinition()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	required := []string{
		"colors.background",
		"colors.surface",
		"colors.text",
		"colors.primary",
		"colors.secondaryContrast",
		"typography.fontBody",
		"typography.scale",
		"layout.contentWidth",
		"spacing.md",
		"radius.md",
	}
	for _, key := range required {
		if _, ok := def.Schema.Settings[key]; !ok {
			t.Fatalf("required token %q missing", key)
		}
	}
}

func TestThemeStylesEmitContentWidthAndSpacing(t *testing.T) {
	def, err := loadDefaultDefinition()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	validated, _ := def.Schema.ValidateSettings(map[string]any{})
	css, err := def.Styles(validated, "")
	if err != nil {
		t.Fatalf("styles: %v", err)
	}
	for _, v := range []string{"--st-content-width", "--st-wide-width", "--st-space-md", "--st-radius-md"} {
		if !strings.Contains(css, v) {
			t.Fatalf("missing css variable %s", v)
		}
	}
}
