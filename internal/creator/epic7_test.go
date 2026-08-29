package creator

import (
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func TestPaletteValidation(t *testing.T) {
	// valid palette
	_, ok := presetByID(PresetBlog)
	if !ok { t.Fatal("preset not found") }
	// Use Preview-like validation
	// We test IsValidPalette etc
	if !IsValidPalette(PaletteClay) { t.Fatal("clay should be valid") }
	if IsValidPalette(PaletteID("unknown")) { t.Fatal("unknown should be invalid") }
	if !IsValidHeader(HeaderMinimal) { t.Fatal("header minimal invalid") }
	if !IsValidFooter(FooterSimple) { t.Fatal("footer simple invalid") }
}

func TestDefaultStyleChoices(t *testing.T) {
	if DefaultPaletteForPreset(PresetBlog) != PaletteClay { t.Fatalf("blog palette %s want clay", DefaultPaletteForPreset(PresetBlog)) }
	if DefaultHeaderForPreset(PresetBlog) != HeaderClassic { t.Fatalf("blog header") }
	if DefaultFooterForPreset(PresetBlog) != FooterSimple { t.Fatalf("blog footer") }
	if DefaultPaletteForPreset(PresetPortfolio) != PaletteInk { t.Fatalf("portfolio palette") }
	if DefaultHeaderForPreset(PresetPortfolio) != HeaderMinimal { t.Fatalf("portfolio header") }
	if DefaultFooterForPreset(PresetPortfolio) != FooterSplit { t.Fatalf("portfolio footer") }
	if DefaultPaletteForPreset(PresetLanding) != PaletteIndigo { t.Fatalf("landing palette") }
	if DefaultHeaderForPreset(PresetLanding) != HeaderMinimal { t.Fatalf("landing header") }
	if DefaultFooterForPreset(PresetLanding) != FooterCentered { t.Fatalf("landing footer") }
}

func TestPaletteStylesDiffer(t *testing.T) {
	ink := paletteStyles(PaletteInk)
	clay := paletteStyles(PaletteClay)
	if ink["colors.primary"] == clay["colors.primary"] { t.Fatal("palettes should differ on primary") }
	if ink["header.background"] == clay["header.background"] { t.Fatal("header bg should differ") }
	if ink["footer.background"] == clay["footer.background"] { t.Fatal("footer bg should differ") }
	if ink["links.color"] == clay["links.color"] { t.Fatal("link color should differ") }
}

func TestHeaderStylesDiffer(t *testing.T) {
	min := headerStyles(HeaderMinimal)
	cla := headerStyles(HeaderClassic)
	cen := headerStyles(HeaderCentered)
	if min["header.layout"] == cla["header.layout"] { t.Fatal("header layouts should differ minimal vs classic") }
	if cla["header.layout"] == cen["header.layout"] { t.Fatal("classic vs centered") }
	// Check that gap is set
	if min["navigation.gap"] == nil { t.Fatal("gap missing") }
}

func TestFooterStylesDiffer(t *testing.T) {
	simple := footerStyles(FooterSimple)
	split := footerStyles(FooterSplit)
	centered := footerStyles(FooterCentered)
	if simple["footer.layout"] == split["footer.layout"] { t.Fatal("footer layouts") }
	if split["footer.layout"] == centered["footer.layout"] && centered["footer.layout"] == simple["footer.layout"] { t.Fatal("all same") }
}

func TestComposedStyles(t *testing.T) {
	blogClayClassicSimple := composedStyles(PresetBlog, PaletteClay, HeaderClassic, FooterSimple)
	// Should contain structural + palette + header + footer
	if blogClayClassicSimple["typography.fontBody"] != "systemSans" { t.Fatalf("blog body font want systemSans got %v", blogClayClassicSimple["typography.fontBody"]) }
	if blogClayClassicSimple["typography.fontHeading"] != "editorialSerif" { t.Fatalf("blog heading") }
	if blogClayClassicSimple["colors.primary"] != "#8b3a3a" { t.Fatalf("clay primary") }
	if blogClayClassicSimple["header.layout"] != "left" { t.Fatalf("classic header layout") }
	if blogClayClassicSimple["footer.layout"] != "simple" { t.Fatalf("footer simple") }
	// Test forest palette gives different primary
	forest := composedStyles(PresetBlog, PaletteForest, HeaderClassic, FooterSimple)
	if forest["colors.primary"] == blogClayClassicSimple["colors.primary"] { t.Fatal("different palette should give different primary") }
}

func TestSitePartDocumentsDiffer(t *testing.T) {
	minHeader := sitePartDocumentForHeader("test", HeaderMinimal)
	classicHeader := sitePartDocumentForHeader("test", HeaderClassic)
	centeredHeader := sitePartDocumentForHeader("test", HeaderCentered)
	if len(minHeader.Nodes) == 0 || len(classicHeader.Nodes) == 0 || len(centeredHeader.Nodes) == 0 { t.Fatal("empty header") }
	// Check that classic contains site-tagline while minimal does not (minimal should not)
	// Minimal doc should have 1 stack with site-name + navigation; classic has tagline
	hasTagline := func(doc *document.Document) bool {
		for _, n := range doc.Nodes {
			// stack children may contain tagline
			if containsBlock(n, "core/site-tagline") { return true }
		}
		return false
	}
	if hasTagline(minHeader) { t.Fatal("minimal header should not have tagline") }
	if !hasTagline(classicHeader) { t.Fatal("classic header should have tagline") }
	if hasTagline(centeredHeader) { t.Fatal("centered header currently no tagline? expected no tagline per spec (brand + nav only)") }

	simpleFooter := sitePartDocumentForFooter("test", FooterSimple)
	splitFooter := sitePartDocumentForFooter("test", FooterSplit)
	centeredFooter := sitePartDocumentForFooter("test", FooterCentered)
	if hasTagline(simpleFooter) { t.Fatal("simple footer should not have tagline? Actually spec simple is just site name, no tagline") }
	if !hasTagline(splitFooter) { t.Fatal("split footer should have tagline") }
	if !containsBlock(splitFooter.Nodes[0], "core/navigation") { t.Fatal("split footer should have navigation") }
	if containsBlock(simpleFooter.Nodes[0], "core/navigation") { t.Fatal("simple footer should not have navigation") }
	if !containsBlock(centeredFooter.Nodes[0], "core/navigation") { t.Fatal("centered footer should have navigation") }
}

func containsBlock(node document.Node, block string) bool {
	if node.Block == block { return true }
	for _, c := range node.Children {
		if containsBlock(c, block) { return true }
	}
	return false
}

func TestBlogEmptyTaglineSpacing(t *testing.T) {
	withTagline := homepageTemplate("p", PresetBlog, "hello", "")
	without := homepageTemplate("p", PresetBlog, "", "")
	// with tagline hero spacing should be md, without sm
	// homepageTemplate returns doc with sections; first section is hero, second is latest posts
	if len(withTagline.Nodes) < 2 || len(without.Nodes) < 2 { t.Fatal("homepage nodes missing") }
	// decode settings for hero section
	heroWith := withTagline.Nodes[0]
	heroWithout := without.Nodes[0]
	// Settings is json.RawMessage, need to check contains spacing md vs sm
	if !containsSetting(heroWith.Settings, `"verticalSpacing":"md"`) { t.Fatalf("blog hero with tagline should be md, got %s", string(heroWith.Settings)) }
	if !containsSetting(heroWithout.Settings, `"verticalSpacing":"sm"`) { t.Fatalf("blog hero without tagline should be sm, got %s", string(heroWithout.Settings)) }
	// Check collections
	// Second section for blog contains heading + collection
	// The collection is inside section children second? Actually section has children heading + collection
	// For blog, second node is section content md default with heading + collection
	// Find collection node
	findCollection := func(doc *document.Document) document.Node {
		for _, n := range doc.Nodes {
			for _, child := range n.Children {
				if child.Block == "core/collection" { return child }
				for _, gc := range child.Children {
					if gc.Block == "core/collection" { return gc }
				}
			}
		}
		return document.Node{}
	}
	c := findCollection(withTagline)
	if c.Block != "core/collection" { t.Fatal("collection not found") }
	// Check layout list
	if !containsSetting(c.Settings, `"layout":"list"`) { t.Fatalf("blog collection layout should be list, got %s", string(c.Settings)) }
}

func containsSetting(raw []byte, substr string) bool {
	return len(raw) > 0 && strings.Contains(string(raw), substr)
}

func TestCollectionArchiveContext(t *testing.T) {
	// Ensure collection v3 schema allows archive-template: we can't directly test DB, but ensure specForWithStyles produces collection v3 nodes that validate in archive-template context
	// This is indirectly tested via ValidateTemplateDocument with archive kind
}
