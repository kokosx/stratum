package creator

import "testing"

func TestDefaultRepresentsIsOrganizationForAllPresets(t *testing.T) {
	for _, p := range Presets() {
		got := DefaultRepresentsForPreset(p.ID)
		if got != "organization" {
			t.Fatalf("DefaultRepresentsForPreset(%q) = %q, want organization", p.ID, got)
		}
	}
	// Explicitly check blog which historically returned person
	if got := DefaultRepresentsForPreset(PresetBlog); got != "organization" {
		t.Fatalf("blog default should be organization, got %q", got)
	}
}

func TestCreatorInvalidSiteURLPreserved(t *testing.T) {
	// Authoritative validation must reject garbage URL
	_, err := NewService(nil, nil, nil, nil, nil, nil, nil).Preview(Input{
		PresetID:       PresetBlog,
		SiteTitle:      "Test Site",
		PaletteID:      PaletteClay,
		HeaderStyleID:  HeaderClassic,
		FooterStyleID:  FooterSimple,
		Language:       "en",
		Timezone:       "UTC",
		SiteRepresents: "organization",
		SiteURL:        "garbage",
	})
	if err == nil {
		t.Fatal("Preview should reject garbage site_url")
	}
	if err.Error() == "" {
		t.Fatal("error should not be empty")
	}
	// Valid URL should pass
	_, err = NewService(nil, nil, nil, nil, nil, nil, nil).Preview(Input{
		PresetID:       PresetBlog,
		SiteTitle:      "Test Site",
		PaletteID:      PaletteClay,
		HeaderStyleID:  HeaderClassic,
		FooterStyleID:  FooterSimple,
		Language:       "en",
		Timezone:       "UTC",
		SiteRepresents: "organization",
		SiteURL:        "https://example.com",
	})
	if err != nil {
		t.Fatalf("valid site_url should pass, got %v", err)
	}
	// Empty URL allowed
	_, err = NewService(nil, nil, nil, nil, nil, nil, nil).Preview(Input{
		PresetID:       PresetBlog,
		SiteTitle:      "Test Site",
		PaletteID:      PaletteClay,
		HeaderStyleID:  HeaderClassic,
		FooterStyleID:  FooterSimple,
		Language:       "en",
		Timezone:       "UTC",
		SiteRepresents: "organization",
		SiteURL:        "",
	})
	if err != nil {
		t.Fatalf("empty site_url should be allowed, got %v", err)
	}
}
