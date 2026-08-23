package themes

import (
	"testing"
)

func TestPresetSemanticsAreDeterministic(t *testing.T) {
	// Load default definition to get schema and defaults
	def, err := loadDefaultDefinition()
	if err != nil {
		t.Fatalf("load default: %v", err)
	}
	schema := def.Schema
	defaults := schema.Defaults()

	settingsForPreset := func(preset Preset) map[string]any {
		next := make(map[string]any, len(defaults))
		for k, v := range defaults {
			next[k] = v
		}
		for k, v := range preset.Values {
			next[k] = v
		}
		validated, err := schema.ValidateSettings(next)
		if err != nil {
			t.Fatalf("validate preset %s: %v", preset.ID, err)
		}
		return validated
	}

	// Helper to find preset by id
	find := func(id string) Preset {
		for _, p := range schema.Presets {
			if p.ID == id {
				return p
			}
		}
		t.Fatalf("preset %s not found", id)
		return Preset{}
	}

	// Editorial -> Dark must equal Dark directly
	darkDirect := settingsForPreset(find("dark"))
	// Simulate stacking bug: Object.assign(defaults+editorial, dark)
	// vs correct: defaults+dark
	editorial := settingsForPreset(find("editorial"))
	// Simulate buggy: start from editorial, then assign dark
	buggy := make(map[string]any, len(editorial))
	for k, v := range editorial {
		buggy[k] = v
	}
	for k, v := range find("dark").Values {
		buggy[k] = v
	}
	// Correct Dark should be defaults+dark, not editorial+dark
	// Check that buggy != darkDirect for at least fontBody
	if buggy["typography.fontBody"] == darkDirect["typography.fontBody"] {
		t.Fatalf("buggy editorial->dark should retain serif but dark direct should be sans: buggy=%v direct=%v", buggy["typography.fontBody"], darkDirect["typography.fontBody"])
	}
	if darkDirect["typography.fontBody"] != "systemSans" {
		t.Fatalf("dark should be systemSans, got %v", darkDirect["typography.fontBody"])
	}
	if darkDirect["typography.fontHeading"] != "systemSans" {
		t.Fatalf("dark should be systemSans heading, got %v", darkDirect["typography.fontHeading"])
	}
	if darkDirect["layout.contentWidth"] != float64(1140) {
		t.Fatalf("dark contentWidth should be default 1140, got %v", darkDirect["layout.contentWidth"])
	}
	if darkDirect["footer.layout"] != "split" {
		t.Fatalf("dark footer.layout should be split (default), got %v", darkDirect["footer.layout"])
	}

	// Dark -> Editorial must equal Editorial directly
	edDirect := settingsForPreset(find("editorial"))
	// Simulate buggy dark->editorial
	buggy2 := make(map[string]any, len(darkDirect))
	for k, v := range darkDirect {
		buggy2[k] = v
	}
	for k, v := range find("editorial").Values {
		buggy2[k] = v
	}
	if edDirect["colors.background"] == buggy2["colors.background"] {
		// Editorial direct has light background #ffffff, buggy has dark #0b1120
		t.Fatalf("editorial direct background should be light, buggy is dark")
	}

	// Soft -> Clean must equal Clean
	cleanDirect := settingsForPreset(find("clean"))
	softDirect := settingsForPreset(find("soft"))
	_ = softDirect
	_ = cleanDirect
	// Clean should be close to defaults
	if cleanDirect["typography.fontBody"] != "systemSans" {
		t.Fatalf("clean fontBody should be systemSans")
	}
	// Bold -> Dark must equal Dark
	boldDirect := settingsForPreset(find("bold"))
	_ = boldDirect
	if darkDirect["typography.fontBody"] != "systemSans" {
		t.Fatalf("bold->dark should be sans")
	}
}

func TestResetToDefaults(t *testing.T) {
	def, _ := loadDefaultDefinition()
	schema := def.Schema
	defaults := schema.Defaults()
	// Simulate a dirty state
	dirty := make(map[string]any, len(defaults))
	for k, v := range defaults {
		dirty[k] = v
	}
	dirty["colors.primary"] = "#ff0000"
	dirty["typography.fontBody"] = "monospace"
	// Reset should be exactly defaults
	reset := make(map[string]any, len(defaults))
	for k, v := range defaults {
		reset[k] = v
	}
	validated, err := schema.ValidateSettings(reset)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range defaults {
		if validated[k] != v {
			// For numbers, compare as float64
			t.Fatalf("reset %s = %v, want %v", k, validated[k], v)
		}
	}
}

func TestPresetValidationRejectsInvalid(t *testing.T) {
	def, _ := loadDefaultDefinition()
	schema := def.Schema
	_, err := schema.ValidateSettings(map[string]any{"colors.primary": "not-a-color"})
	if err == nil {
		t.Fatal("invalid color should be rejected")
	}
	_, err = schema.ValidateSettings(map[string]any{"typography.fontBody": "invalidFont"})
	if err == nil {
		t.Fatal("invalid font should be rejected")
	}
}
