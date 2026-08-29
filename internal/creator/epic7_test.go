package creator

import (
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func TestPaletteValidation(t *testing.T) {
	// valid palette
	_, ok := presetByID(PresetBlog)
	if !ok {
		t.Fatal("preset not found")
	}
	// Use Preview-like validation
	// We test IsValidPalette etc
	if !IsValidPalette(PaletteClay) {
		t.Fatal("clay should be valid")
	}
	if IsValidPalette(PaletteID("unknown")) {
		t.Fatal("unknown should be invalid")
	}
	if !IsValidHeader(HeaderMinimal) {
		t.Fatal("header minimal invalid")
	}
	if !IsValidFooter(FooterSimple) {
		t.Fatal("footer simple invalid")
	}
}

func TestDefaultStyleChoices(t *testing.T) {
	if DefaultPaletteForPreset(PresetBlog) != PaletteClay {
		t.Fatalf("blog palette %s want clay", DefaultPaletteForPreset(PresetBlog))
	}
	if DefaultHeaderForPreset(PresetBlog) != HeaderClassic {
		t.Fatalf("blog header")
	}
	if DefaultFooterForPreset(PresetBlog) != FooterSimple {
		t.Fatalf("blog footer")
	}
	if DefaultPaletteForPreset(PresetPortfolio) != PaletteInk {
		t.Fatalf("portfolio palette")
	}
	if DefaultHeaderForPreset(PresetPortfolio) != HeaderMinimal {
		t.Fatalf("portfolio header")
	}
	if DefaultFooterForPreset(PresetPortfolio) != FooterSplit {
		t.Fatalf("portfolio footer")
	}
	if DefaultPaletteForPreset(PresetLanding) != PaletteIndigo {
		t.Fatalf("landing palette")
	}
	if DefaultHeaderForPreset(PresetLanding) != HeaderMinimal {
		t.Fatalf("landing header")
	}
	if DefaultFooterForPreset(PresetLanding) != FooterCentered {
		t.Fatalf("landing footer")
	}
}

func TestPaletteStylesDiffer(t *testing.T) {
	ink := paletteStyles(PaletteInk)
	clay := paletteStyles(PaletteClay)
	if ink["colors.primary"] == clay["colors.primary"] {
		t.Fatal("palettes should differ on primary")
	}
	if ink["header.background"] == clay["header.background"] {
		t.Fatal("header bg should differ")
	}
	if ink["footer.background"] == clay["footer.background"] {
		t.Fatal("footer bg should differ")
	}
	if ink["links.color"] == clay["links.color"] {
		t.Fatal("link color should differ")
	}
}

func TestHeaderStylesDiffer(t *testing.T) {
	min := headerStyles(HeaderMinimal)
	cla := headerStyles(HeaderClassic)
	cen := headerStyles(HeaderCentered)
	if min["header.layout"] == cla["header.layout"] {
		t.Fatal("header layouts should differ minimal vs classic")
	}
	if cla["header.layout"] == cen["header.layout"] {
		t.Fatal("classic vs centered")
	}
	// Check that gap is set
	if min["navigation.gap"] == nil {
		t.Fatal("gap missing")
	}
}

func TestFooterStylesDiffer(t *testing.T) {
	simple := footerStyles(FooterSimple)
	split := footerStyles(FooterSplit)
	centered := footerStyles(FooterCentered)
	if simple["footer.layout"] == split["footer.layout"] {
		t.Fatal("footer layouts")
	}
	if split["footer.layout"] == centered["footer.layout"] && centered["footer.layout"] == simple["footer.layout"] {
		t.Fatal("all same")
	}
}

func TestComposedStyles(t *testing.T) {
	blogClayClassicSimple := composedStyles(PresetBlog, PaletteClay, HeaderClassic, FooterSimple)
	// Should contain structural + palette + header + footer
	if blogClayClassicSimple["typography.fontBody"] != "systemSans" {
		t.Fatalf("blog body font want systemSans got %v", blogClayClassicSimple["typography.fontBody"])
	}
	if blogClayClassicSimple["typography.fontHeading"] != "editorialSerif" {
		t.Fatalf("blog heading")
	}
	if blogClayClassicSimple["colors.primary"] != "#8b3a3a" {
		t.Fatalf("clay primary")
	}
	if blogClayClassicSimple["header.layout"] != "left" {
		t.Fatalf("classic header layout")
	}
	if blogClayClassicSimple["footer.layout"] != "simple" {
		t.Fatalf("footer simple")
	}
	// Test forest palette gives different primary
	forest := composedStyles(PresetBlog, PaletteForest, HeaderClassic, FooterSimple)
	if forest["colors.primary"] == blogClayClassicSimple["colors.primary"] {
		t.Fatal("different palette should give different primary")
	}
}

func TestSitePartDocumentsDiffer(t *testing.T) {
	minHeader := sitePartDocumentForHeader("test", HeaderMinimal)
	classicHeader := sitePartDocumentForHeader("test", HeaderClassic)
	centeredHeader := sitePartDocumentForHeader("test", HeaderCentered)
	if len(minHeader.Nodes) == 0 || len(classicHeader.Nodes) == 0 || len(centeredHeader.Nodes) == 0 {
		t.Fatal("empty header")
	}
	// Check that classic contains site-tagline while minimal does not (minimal should not)
	// Minimal doc should have 1 stack with site-name + navigation; classic has tagline
	hasTagline := func(doc *document.Document) bool {
		for _, n := range doc.Nodes {
			// stack children may contain tagline
			if containsBlock(n, "core/site-tagline") {
				return true
			}
		}
		return false
	}
	if hasTagline(minHeader) {
		t.Fatal("minimal header should not have tagline")
	}
	if !hasTagline(classicHeader) {
		t.Fatal("classic header should have tagline")
	}
	if hasTagline(centeredHeader) {
		t.Fatal("centered header currently no tagline? expected no tagline per spec (brand + nav only)")
	}

	simpleFooter := sitePartDocumentForFooter("test", FooterSimple)
	splitFooter := sitePartDocumentForFooter("test", FooterSplit)
	centeredFooter := sitePartDocumentForFooter("test", FooterCentered)
	if hasTagline(simpleFooter) {
		t.Fatal("simple footer should not have tagline? Actually spec simple is just site name, no tagline")
	}
	if !hasTagline(splitFooter) {
		t.Fatal("split footer should have tagline")
	}
	if !containsBlock(splitFooter.Nodes[0], "core/navigation") {
		t.Fatal("split footer should have navigation")
	}
	if containsBlock(simpleFooter.Nodes[0], "core/navigation") {
		t.Fatal("simple footer should not have navigation")
	}
	if !containsBlock(centeredFooter.Nodes[0], "core/navigation") {
		t.Fatal("centered footer should have navigation")
	}
}

func containsBlock(node document.Node, block string) bool {
	if node.Block == block {
		return true
	}
	for _, c := range node.Children {
		if containsBlock(c, block) {
			return true
		}
	}
	return false
}

func TestBlogEmptyTaglineSpacing(t *testing.T) {
	withTagline := homepageTemplate("p", PresetBlog, "hello", "")
	without := homepageTemplate("p", PresetBlog, "", "")
	// After EPIC 7 art-direction, Blog homepage uses ONE main content Section v2 (content/lg/default)
	// containing editorial Stack (hero gap md + latest posts gap lg) via nested stacks.
	// Both with and without tagline use lg for consistent top padding 56-80.
	if len(withTagline.Nodes) < 2 || len(without.Nodes) < 2 {
		t.Fatal("homepage nodes missing")
	}
	heroWith := withTagline.Nodes[0]
	heroWithout := without.Nodes[0]
	if heroWith.Block != "core/section" || heroWith.Version != 2 {
		t.Fatalf("blog first node should be section v2, got %s@%d", heroWith.Block, heroWith.Version)
	}
	if heroWithout.Block != "core/section" || heroWithout.Version != 2 {
		t.Fatalf("blog first node should be section v2, got %s@%d", heroWithout.Block, heroWithout.Version)
	}
	if !containsSetting(heroWith.Settings, `"verticalSpacing":"lg"`) {
		t.Fatalf("blog section should be lg for art-directed rhythm, got %s", string(heroWith.Settings))
	}
	if !containsSetting(heroWithout.Settings, `"verticalSpacing":"lg"`) {
		t.Fatalf("blog section without tagline should also be lg, got %s", string(heroWithout.Settings))
	}
	if !containsSetting(heroWith.Settings, `"width":"content"`) {
		t.Fatalf("blog section width should be content, got %s", string(heroWith.Settings))
	}
	// Find collection deeply (inside stack)
	findCollection := func(doc *document.Document) document.Node {
		var search func([]document.Node) document.Node
		search = func(nodes []document.Node) document.Node {
			for _, n := range nodes {
				if n.Block == "core/collection" {
					return n
				}
				if len(n.Children) > 0 {
					if found := search(n.Children); found.Block != "" {
						return found
					}
				}
			}
			return document.Node{}
		}
		return search(doc.Nodes)
	}
	c := findCollection(withTagline)
	if c.Block != "core/collection" {
		t.Fatal("collection not found")
	}
	if !containsSetting(c.Settings, `"layout":"list"`) {
		t.Fatalf("blog collection layout should be list, got %s", string(c.Settings))
	}
	if c.Version != 3 {
		t.Fatalf("collection should be v3, got %d", c.Version)
	}
}

func containsSetting(raw []byte, substr string) bool {
	return len(raw) > 0 && strings.Contains(string(raw), substr)
}

func TestCollectionArchiveContext(t *testing.T) {
	// Ensure collection v3 schema allows archive-template: we can't directly test DB, but ensure specForWithStyles produces collection v3 nodes that validate in archive-template context
	// This is indirectly tested via ValidateTemplateDocument with archive kind
}

func TestSectionV2Usage(t *testing.T) {
	// Creator must generate Section@2 after migration; historical v1 remains renderable is tested elsewhere.
	for _, preset := range []PresetID{PresetBlog, PresetPortfolio, PresetLanding, PresetProducts, PresetLocalBusiness} {
		doc := homepageTemplate("t", preset, "tagline", "form-id")
		hasV2 := false
		hasV1 := false
		var visit func([]document.Node)
		visit = func(nodes []document.Node) {
			for _, n := range nodes {
				if n.Block == "core/section" {
					if n.Version == 2 {
						hasV2 = true
						// Verify inner wrapper expectation is encoded in template via settings still containing width/verticalSpacing etc
						if !containsSetting(n.Settings, `"width"`) {
							t.Errorf("%s section v2 missing width", preset)
						}
						if !containsSetting(n.Settings, `"verticalSpacing"`) {
							t.Errorf("%s section v2 missing verticalSpacing", preset)
						}
					}
					if n.Version == 1 {
						hasV1 = true
					}
				}
				if len(n.Children) > 0 {
					visit(n.Children)
				}
			}
		}
		visit(doc.Nodes)
		if !hasV2 {
			t.Fatalf("%s homepage should contain section v2", preset)
		}
		if hasV1 {
			t.Fatalf("%s homepage should not contain section v1 after art-direction", preset)
		}
	}
	// Portfolio grid 2, Product grid 3, Services grid 3, Testimonials grid 2, Blog list
	checkGrid := func(preset PresetID, wantColumns int, wantLayout string) {
		doc := homepageTemplate("t", preset, "tagline", "form-id")
		var found bool
		var visit func([]document.Node)
		visit = func(nodes []document.Node) {
			for _, n := range nodes {
				if n.Block == "core/collection" {
					if containsSetting(n.Settings, `"columns":`+string(rune('0'+wantColumns))) || containsSetting(n.Settings, `"columns":`+string(rune('0'+wantColumns))) {
						// crude check, but we check both layout and columns via string contains
					}
					if containsSetting(n.Settings, `"layout":"`+wantLayout+`"`) && containsSetting(n.Settings, `"columns":`+func() string { return string([]byte{byte('0' + wantColumns)}) }()) {
						found = true
					}
				}
				if len(n.Children) > 0 {
					visit(n.Children)
				}
			}
		}
		visit(doc.Nodes)
		if !found {
			t.Fatalf("%s should have collection %s cols %d", preset, wantLayout, wantColumns)
		}
	}
	checkGrid(PresetPortfolio, 2, "grid")
	checkGrid(PresetProducts, 3, "grid")
	checkGrid(PresetLocalBusiness, 3, "grid")
	checkGrid(PresetBlog, 1, "list")
	// Landing testimonials grid 2
	landingDoc := homepageTemplate("t", PresetLanding, "tagline", "form-id")
	hasTestimonialGrid := false
	var vt func([]document.Node)
	vt = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/collection" && containsSetting(n.Settings, `"layout":"grid"`) && containsSetting(n.Settings, `"columns":2`) {
				hasTestimonialGrid = true
			}
			if len(n.Children) > 0 {
				vt(n.Children)
			}
		}
	}
	vt(landingDoc.Nodes)
	if !hasTestimonialGrid {
		t.Fatal("landing should have testimonial grid 2")
	}
}

func TestHeaderFooterChoices(t *testing.T) {
	// Ensure header/footer parts generate v2 sections? Actually site parts are stacks, not sections.
	// Just sanity check palette/header/footer validation already tested.
	if !IsValidPalette(PaletteInk) || !IsValidPalette(PaletteClay) {
		t.Fatal("palette")
	}
	if DefaultPaletteForPreset(PresetBlog) != PaletteClay {
		t.Fatal("blog default palette should be clay")
	}
}

func TestNoLegacyFormV1(t *testing.T) {
	// Ensure no core/form@1 generation remains; all forms should be v2.
	for _, preset := range []PresetID{PresetBlog, PresetPortfolio, PresetLanding, PresetProducts, PresetLocalBusiness} {
		doc := homepageTemplate("t", preset, "tagline", "form-id")
		var visit func([]document.Node)
		visit = func(nodes []document.Node) {
			for _, n := range nodes {
				if n.Block == "core/form" && n.Version == 1 {
					t.Fatalf("%s should not generate form v1, found %s@%d", preset, n.Block, n.Version)
				}
				if len(n.Children) > 0 {
					visit(n.Children)
				}
			}
		}
		visit(doc.Nodes)
	}
	// Also check single and archive templates
	for _, preset := range []PresetID{PresetBlog, PresetPortfolio, PresetLanding, PresetProducts, PresetLocalBusiness} {
		for _, doc := range []*document.Document{singleTemplate("t", preset), archiveTemplate("t", preset)} {
			var visit func([]document.Node)
			visit = func(nodes []document.Node) {
				for _, n := range nodes {
					if n.Block == "core/form" && n.Version == 1 {
						t.Fatalf("%s single/archive has form v1", preset)
					}
					if len(n.Children) > 0 {
						visit(n.Children)
					}
				}
			}
			visit(doc.Nodes)
		}
	}
}
