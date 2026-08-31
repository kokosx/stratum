package creator

import "github.com/kokosx/stratum/internal/content"

type PresetID string

const (
	PresetBlog          PresetID = "blog"
	PresetPortfolio     PresetID = "portfolio"
	PresetLanding       PresetID = "landing"
	PresetProducts      PresetID = "products"
	PresetLocalBusiness PresetID = "local-business"
	PresetSimpleSite    PresetID = "simple-site"
	PresetMagazine      PresetID = "magazine"
	PresetAgency        PresetID = "agency"
	PresetKnowledgeBase PresetID = "knowledge-base"
)

type Preset struct {
	ID          PresetID
	Name        string
	Description string
	Pages       []string
	Dynamic     string
	Templates   []string
	Group       string
}

type PaletteID string

const (
	PaletteInk      PaletteID = "ink"
	PaletteClay     PaletteID = "clay"
	PaletteForest   PaletteID = "forest"
	PaletteIndigo   PaletteID = "indigo"
	PaletteOcean    PaletteID = "ocean"
	PaletteSand     PaletteID = "sand"
	PaletteBerry    PaletteID = "berry"
	PaletteMidnight PaletteID = "midnight"
)

type HeaderStyleID string

const (
	HeaderMinimal   HeaderStyleID = "minimal"
	HeaderClassic   HeaderStyleID = "classic"
	HeaderCentered  HeaderStyleID = "centered"
	HeaderStacked   HeaderStyleID = "stacked"
	HeaderBold      HeaderStyleID = "bold"
	HeaderEditorial HeaderStyleID = "editorial"
)

type FooterStyleID string

const (
	FooterSimple    FooterStyleID = "simple"
	FooterSplit     FooterStyleID = "split"
	FooterCentered  FooterStyleID = "centered"
	FooterStacked   FooterStyleID = "stacked"
	FooterEditorial FooterStyleID = "editorial"
)

type Palette struct {
	ID          PaletteID
	Name        string
	Description string
	Swatches    []string
}

type HeaderOption struct {
	ID          HeaderStyleID
	Name        string
	Description string
}

type FooterOption struct {
	ID          FooterStyleID
	Name        string
	Description string
}

type Input struct {
	SiteTitle     string
	Tagline       string
	PresetID      PresetID
	PaletteID     PaletteID
	HeaderStyleID HeaderStyleID
	FooterStyleID FooterStyleID

	// J1 layout options (creation input only, not persisted as runtime coupling)
	BlogLatestCount            int    // 5 or 8
	BlogArchiveCount           int    // 10 or 20
	PortfolioColumns           int    // 2 or 3
	ProductColumns             int    // 3 or 4
	ProductMediaPosition       string // "left" or "right"
	LandingTestimonialsColumns int    // 1 or 2
	ServiceColumns             int    // 2 or 3

	// K site details
	Language        string // "en" or "pl" for Creator v1
	Timezone        string // IANA
	SiteRepresents  string // "organization" or "person"
	IndexingEnabled bool   // true = allow indexing, default false (discourage)
	SiteURL         string
}

type Plan struct {
	Input  Input
	Preset Preset
}

type Result struct {
	HomepageID string
	Warnings   []string
	Pages      int
	Entries    int
	Forms      int
}

var presets = []Preset{
	{ID: PresetBlog, Name: "Blog", Description: "A clean editorial site with real posts and a dynamic latest-posts collection.", Pages: []string{"Home", "About"}, Dynamic: "Posts - 5 examples", Templates: []string{"Page", "Homepage", "Post", "Posts Archive"}, Group: "content"},
	{ID: PresetPortfolio, Name: "Portfolio", Description: "A project-focused site for studios, freelancers and creative teams.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Projects - 6 examples", Templates: []string{"Page", "Homepage", "Project", "Projects Archive"}, Group: "content"},
	{ID: PresetLanding, Name: "Landing Page", Description: "A focused lead-generation site with dynamic, route-less testimonials.", Pages: []string{"Home"}, Dynamic: "Testimonials - 4 examples", Templates: []string{"Page", "Homepage"}, Group: "simple"},
	{ID: PresetProducts, Name: "Product Showcase", Description: "A catalog-style product website without cart, checkout or payments.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Products - 6 examples", Templates: []string{"Page", "Homepage", "Product", "Products Archive"}, Group: "business"},
	{ID: PresetLocalBusiness, Name: "Local Business", Description: "A warm, practical local-service website with services and contact details.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Services - 5 examples", Templates: []string{"Page", "Homepage", "Service", "Services Archive"}, Group: "business"},
	{ID: PresetSimpleSite, Name: "Simple Site", Description: "Just a normal website — Home, About, Contact with a simple contact form.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Pages only", Templates: []string{"Page", "Homepage"}, Group: "simple"},
	{ID: PresetMagazine, Name: "Magazine", Description: "Editorial magazine with featured, latest and category-driven examples.", Pages: []string{"Home", "About"}, Dynamic: "Posts - 8 featured", Templates: []string{"Page", "Homepage", "Post", "Posts Archive"}, Group: "content"},
	{ID: PresetAgency, Name: "Agency", Description: "Agency site showcasing case studies — Home, Services, About, Contact.", Pages: []string{"Home", "Services", "About", "Contact"}, Dynamic: "Case Studies - 6 examples", Templates: []string{"Page", "Homepage", "Case Study", "Case Studies Archive"}, Group: "business"},
	{ID: PresetKnowledgeBase, Name: "Knowledge Base", Description: "Restrained documentation site with Articles and a clean archive.", Pages: []string{"Home", "Knowledge Base"}, Dynamic: "Articles - 6 examples", Templates: []string{"Page", "Homepage", "Article", "Articles Archive"}, Group: "content"},
}

func Presets() []Preset { return append([]Preset(nil), presets...) }

func presetByID(id PresetID) (Preset, bool) {
	for _, preset := range presets {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

var palettes = []Palette{
	{ID: PaletteInk, Name: "Ink", Description: "High-contrast, monochrome with sharp typography.", Swatches: []string{"#111111", "#f7f7f5", "#ececea", "#cececa"}},
	{ID: PaletteClay, Name: "Clay", Description: "Warm, editorial with muted earth tones.", Swatches: []string{"#8b3a3a", "#fbf8f4", "#f1e9e1", "#d8ccc3"}},
	{ID: PaletteForest, Name: "Forest", Description: "Grounded greens with a natural feel.", Swatches: []string{"#356859", "#fbfaf6", "#f1eee4", "#d7d5c9"}},
	{ID: PaletteIndigo, Name: "Indigo", Description: "Deep violet with confident contrast.", Swatches: []string{"#6842a8", "#faf8ff", "#f0ebfa", "#d8cfe5"}},
	{ID: PaletteOcean, Name: "Ocean", Description: "Cool blues with airy contrast.", Swatches: []string{"#0f4c75", "#f0f7fb", "#d7e8f3", "#b8d0e6"}},
	{ID: PaletteSand, Name: "Sand", Description: "Warm sand and terracotta neutrals.", Swatches: []string{"#a05a2c", "#fdf8f3", "#f2e6d9", "#e6d2bf"}},
	{ID: PaletteBerry, Name: "Berry", Description: "Berry reds with cream surfaces.", Swatches: []string{"#8b1e3f", "#fdf2f5", "#f3d5dd", "#e8b8c3"}},
	{ID: PaletteMidnight, Name: "Midnight", Description: "Genuinely dark — deep navy, muted surfaces, sharp contrast.", Swatches: []string{"#0e1a2b", "#0e1a2b", "#1e2e4a", "#2a3d5c"}},
}

var headerOptions = []HeaderOption{
	{ID: HeaderMinimal, Name: "Minimal", Description: "Compact, no border. Good for portfolios."},
	{ID: HeaderClassic, Name: "Classic", Description: "Balanced with subtle border. General-purpose."},
	{ID: HeaderCentered, Name: "Centered", Description: "Brand centered, navigation below."},
	{ID: HeaderStacked, Name: "Stacked", Description: "Left-aligned stack: brand, tagline, then nav."},
	{ID: HeaderBold, Name: "Bold", Description: "Larger brand, strong identity. 80–92px."},
	{ID: HeaderEditorial, Name: "Editorial", Description: "Two rows with subtle separator. For blogs."},
}

var footerOptions = []FooterOption{
	{ID: FooterSimple, Name: "Simple", Description: "Site name + copyright only."},
	{ID: FooterSplit, Name: "Split", Description: "Brand left, navigation right, copyright below."},
	{ID: FooterCentered, Name: "Centered", Description: "Centered stack with navigation."},
	{ID: FooterStacked, Name: "Stacked", Description: "Left-aligned: brand, tagline, nav, copyright."},
	{ID: FooterEditorial, Name: "Editorial", Description: "Larger brand identity for editorial sites."},
}

func Palettes() []Palette           { return append([]Palette(nil), palettes...) }
func HeaderOptions() []HeaderOption { return append([]HeaderOption(nil), headerOptions...) }
func FooterOptions() []FooterOption { return append([]FooterOption(nil), footerOptions...) }

var paletteSet = map[PaletteID]bool{PaletteInk: true, PaletteClay: true, PaletteForest: true, PaletteIndigo: true, PaletteOcean: true, PaletteSand: true, PaletteBerry: true, PaletteMidnight: true}
var headerSet = map[HeaderStyleID]bool{HeaderMinimal: true, HeaderClassic: true, HeaderCentered: true, HeaderStacked: true, HeaderBold: true, HeaderEditorial: true}
var footerSet = map[FooterStyleID]bool{FooterSimple: true, FooterSplit: true, FooterCentered: true, FooterStacked: true, FooterEditorial: true}

func IsValidPalette(id PaletteID) bool    { return paletteSet[id] }
func IsValidHeader(id HeaderStyleID) bool { return headerSet[id] }
func IsValidFooter(id FooterStyleID) bool { return footerSet[id] }

func DefaultPaletteForPreset(p PresetID) PaletteID {
	switch p {
	case PresetBlog:
		return PaletteClay
	case PresetPortfolio:
		return PaletteInk
	case PresetLanding:
		return PaletteIndigo
	case PresetProducts:
		return PaletteInk
	default:
		return PaletteForest
	}
}

func DefaultHeaderForPreset(p PresetID) HeaderStyleID {
	switch p {
	case PresetBlog:
		return HeaderClassic
	case PresetPortfolio:
		return HeaderMinimal
	case PresetLanding:
		return HeaderMinimal
	case PresetProducts:
		return HeaderClassic
	default:
		return HeaderClassic
	}
}

func DefaultFooterForPreset(p PresetID) FooterStyleID {
	switch p {
	case PresetBlog:
		return FooterSimple
	case PresetLanding:
		return FooterCentered
	case PresetPortfolio:
		return FooterSplit
	case PresetProducts:
		return FooterSplit
	default:
		return FooterSplit
	}
}

func DefaultRepresentsForPreset(p PresetID) string {
	return "organization"
}

func DefaultLanguageForPreset(p PresetID) string {
	_ = p
	return "en"
}

func DefaultTimezoneForPreset() string { return "UTC" }

func IsValidRepresents(v string) bool           { return v == "organization" || v == "person" }
func IsValidProductMediaPosition(v string) bool { return v == "left" || v == "right" }
func IsValidCreatorLanguage(lang string) bool   { return lang == "en" || lang == "pl" }

type presetSpec struct {
	preset      Preset
	contentType *content.ContentTypeInput
	archivePath string
	seedEntries []entrySpec
	pages       []pageSpec
	form        *formSpec
	styles      map[string]any
	images      bool
}

type entrySpec struct {
	Title   string
	Slug    string
	Excerpt string
	Body    string
	Fields  map[string]any
}

type pageSpec struct {
	Title   string
	Slug    string
	Body    string
	Excerpt string
	Form    bool
}

type formSpec struct {
	Name  string
	Phone bool
}

// structuralStarterStyles returns layout + typography + shape tokens that belong
// to the starter's editorial intent. Colors and header/footer chrome are handled
// by paletteStyles / headerStyles / footerStyles.
func structuralStarterStyles(preset PresetID) map[string]any {
	base := map[string]any{
		"typography.fontBody": "systemSans", "typography.fontHeading": "systemSans", "typography.bodySize": 17, "typography.bodyLineHeight": 1.65, "typography.headingWeight": 700, "typography.h1Size": 56, "typography.h2Size": 36, "typography.h3Size": 26,
		"layout.contentWidth": 1040, "layout.wideWidth": 1440, "layout.pagePadding": 24,
		"buttons.primaryStyle": "solid", "buttons.secondaryStyle": "outline", "buttons.radius": "sm",
		"radius.sm": 3, "radius.md": 6, "radius.lg": 10, "shadow.sm": "none", "shadow.md": "none", "shadow.lg": "none",
	}
	switch preset {
	case PresetBlog:
		base["typography.fontBody"] = "systemSans"
		base["typography.fontHeading"] = "editorialSerif"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.65
		base["typography.headingWeight"] = 600
		base["typography.h1Size"] = 56
		base["typography.h2Size"] = 32
		base["typography.h3Size"] = 24
		base["layout.contentWidth"] = 760
		base["layout.wideWidth"] = 1280
		base["layout.pagePadding"] = 24
		base["radius.md"] = 2
		base["radius.lg"] = 4
	case PresetPortfolio:
		base["typography.fontBody"] = "modernSans"
		base["typography.fontHeading"] = "modernSans"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.6
		base["typography.headingWeight"] = 600
		base["typography.h1Size"] = 74
		base["typography.h2Size"] = 32
		base["typography.h3Size"] = 24
		base["layout.contentWidth"] = 1120
		base["layout.wideWidth"] = 1440
		base["layout.pagePadding"] = 32
		base["radius.sm"] = 0
		base["radius.md"] = 0
		base["radius.lg"] = 0
		base["buttons.radius"] = "none"
	case PresetLanding:
		base["typography.fontBody"] = "humanistSans"
		base["typography.fontHeading"] = "humanistSans"
		base["typography.bodySize"] = 18
		base["typography.bodyLineHeight"] = 1.6
		base["typography.headingWeight"] = 750
		base["typography.h1Size"] = 66
		base["typography.h2Size"] = 36
		base["typography.h3Size"] = 26
		base["layout.contentWidth"] = 940
		base["layout.wideWidth"] = 1280
		base["layout.pagePadding"] = 24
		base["radius.md"] = 4
		base["radius.lg"] = 6
	case PresetProducts:
		base["typography.fontBody"] = "humanistSans"
		base["typography.fontHeading"] = "humanistSans"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.65
		base["typography.headingWeight"] = 650
		base["typography.h1Size"] = 62
		base["typography.h2Size"] = 32
		base["typography.h3Size"] = 22
		base["layout.contentWidth"] = 1120
		base["layout.wideWidth"] = 1440
		base["layout.pagePadding"] = 28
		base["radius.md"] = 6
		base["radius.lg"] = 8
		base["shadow.sm"] = "soft"
	case PresetLocalBusiness:
		base["typography.fontBody"] = "humanistSans"
		base["typography.fontHeading"] = "humanistSans"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.65
		base["typography.headingWeight"] = 700
		base["typography.h1Size"] = 58
		base["typography.h2Size"] = 30
		base["typography.h3Size"] = 22
		base["layout.contentWidth"] = 980
		base["layout.wideWidth"] = 1320
		base["layout.pagePadding"] = 24
		base["radius.md"] = 5
		base["radius.lg"] = 8
	case PresetSimpleSite:
		base["typography.fontBody"] = "humanistSans"
		base["typography.fontHeading"] = "humanistSans"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.65
		base["typography.headingWeight"] = 650
		base["typography.h1Size"] = 52
		base["typography.h2Size"] = 30
		base["typography.h3Size"] = 22
		base["layout.contentWidth"] = 960
		base["layout.wideWidth"] = 1280
		base["layout.pagePadding"] = 24
		base["radius.md"] = 6
		base["radius.lg"] = 8
	case PresetMagazine:
		base["typography.fontBody"] = "editorialSerif"
		base["typography.fontHeading"] = "editorialSerif"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.65
		base["typography.headingWeight"] = 600
		base["typography.h1Size"] = 54
		base["typography.h2Size"] = 30
		base["typography.h3Size"] = 22
		base["layout.contentWidth"] = 760
		base["layout.wideWidth"] = 1320
		base["layout.pagePadding"] = 24
		base["radius.md"] = 2
		base["radius.lg"] = 4
	case PresetAgency:
		base["typography.fontBody"] = "modernSans"
		base["typography.fontHeading"] = "modernSans"
		base["typography.bodySize"] = 17
		base["typography.bodyLineHeight"] = 1.6
		base["typography.headingWeight"] = 700
		base["typography.h1Size"] = 56
		base["typography.h2Size"] = 30
		base["typography.h3Size"] = 22
		base["layout.contentWidth"] = 1120
		base["layout.wideWidth"] = 1440
		base["layout.pagePadding"] = 28
		base["radius.md"] = 4
		base["radius.lg"] = 6
	case PresetKnowledgeBase:
		base["typography.fontBody"] = "systemSans"
		base["typography.fontHeading"] = "systemSans"
		base["typography.bodySize"] = 16
		base["typography.bodyLineHeight"] = 1.65
		base["typography.headingWeight"] = 700
		base["typography.h1Size"] = 48
		base["typography.h2Size"] = 26
		base["typography.h3Size"] = 20
		base["layout.contentWidth"] = 760
		base["layout.wideWidth"] = 1120
		base["layout.pagePadding"] = 24
		base["radius.md"] = 4
		base["radius.lg"] = 6
	}
	return base
}

func paletteStyles(id PaletteID) map[string]any {
	var palette map[string]string
	var headerBG, footerBG, footerText string
	switch id {
	case PaletteInk:
		palette = map[string]string{"background": "#f7f7f5", "surface": "#ffffff", "surfaceMuted": "#ececea", "text": "#242424", "textMuted": "#686868", "heading": "#0c0c0c", "primary": "#111111", "primaryHover": "#383838", "primaryContrast": "#ffffff", "secondary": "#6b6b6b", "secondaryHover": "#444444", "secondaryContrast": "#ffffff", "border": "#cececa", "focus": "#6b6b6b"}
		headerBG, footerBG, footerText = "#f7f7f5", "#111111", "#f2f2f0"
	case PaletteClay:
		palette = map[string]string{"background": "#fbf8f4", "surface": "#fffdf9", "surfaceMuted": "#f1e9e1", "text": "#382f2b", "textMuted": "#75645d", "heading": "#241c19", "primary": "#8b3a3a", "primaryHover": "#6f2d2d", "primaryContrast": "#ffffff", "secondary": "#5f514b", "secondaryHover": "#453a35", "secondaryContrast": "#ffffff", "border": "#d8ccc3", "focus": "#9a5a5a"}
		headerBG, footerBG, footerText = "#fbf8f4", "#2f2521", "#f5eee8"
	case PaletteForest:
		palette = map[string]string{"background": "#fbfaf6", "surface": "#ffffff", "surfaceMuted": "#f1eee4", "text": "#33413c", "textMuted": "#6b756f", "heading": "#1e302a", "primary": "#356859", "primaryHover": "#285044", "primaryContrast": "#ffffff", "secondary": "#a4693b", "secondaryHover": "#82522e", "secondaryContrast": "#ffffff", "border": "#d7d5c9", "focus": "#3a6b5e"}
		headerBG, footerBG, footerText = "#fbfaf6", "#263b34", "#f4f1e8"
	case PaletteIndigo:
		palette = map[string]string{"background": "#faf8ff", "surface": "#ffffff", "surfaceMuted": "#f0ebfa", "text": "#302a3a", "textMuted": "#6d6479", "heading": "#211a2d", "primary": "#6842a8", "primaryHover": "#533287", "primaryContrast": "#ffffff", "secondary": "#cf9b32", "secondaryHover": "#ab7c22", "secondaryContrast": "#211a2d", "border": "#d8cfe5", "focus": "#7a4fb5"}
		headerBG, footerBG, footerText = "#faf8ff", "#211a2d", "#f7f1ff"
	case PaletteOcean:
		palette = map[string]string{"background": "#f0f7fb", "surface": "#ffffff", "surfaceMuted": "#d7e8f3", "text": "#12314a", "textMuted": "#4a6a86", "heading": "#0f2a40", "primary": "#0f4c75", "primaryHover": "#0c3d5e", "primaryContrast": "#ffffff", "secondary": "#3282b8", "secondaryHover": "#256a9b", "secondaryContrast": "#ffffff", "border": "#b8d0e6", "focus": "#3a8ab8"}
		headerBG, footerBG, footerText = "#f0f7fb", "#0f2a40", "#e6f0f8"
	case PaletteSand:
		palette = map[string]string{"background": "#fdf8f3", "surface": "#ffffff", "surfaceMuted": "#f2e6d9", "text": "#4a2f1a", "textMuted": "#7a5a3c", "heading": "#3d2310", "primary": "#a05a2c", "primaryHover": "#7d4621", "primaryContrast": "#ffffff", "secondary": "#6a7a5a", "secondaryHover": "#526149", "secondaryContrast": "#ffffff", "border": "#e6d2bf", "focus": "#c9a87a"}
		headerBG, footerBG, footerText = "#fdf8f3", "#3d2310", "#fdf0e3"
	case PaletteBerry:
		palette = map[string]string{"background": "#fdf2f5", "surface": "#ffffff", "surfaceMuted": "#f3d5dd", "text": "#4a1020", "textMuted": "#7a3a4f", "heading": "#380a18", "primary": "#8b1e3f", "primaryHover": "#6b1630", "primaryContrast": "#ffffff", "secondary": "#b85a5a", "secondaryHover": "#9a4a4a", "secondaryContrast": "#ffffff", "border": "#e8b8c3", "focus": "#c93a5a"}
		headerBG, footerBG, footerText = "#fdf2f5", "#380a18", "#fde6eb"
	case PaletteMidnight:
		palette = map[string]string{"background": "#0e1a2b", "surface": "#16233d", "surfaceMuted": "#1e2e4a", "text": "#e6eaf0", "textMuted": "#9aa7bd", "heading": "#ffffff", "primary": "#5ea9ff", "primaryHover": "#3d8de6", "primaryContrast": "#0e1a2b", "secondary": "#8b9bff", "secondaryHover": "#6b7de0", "secondaryContrast": "#0e1a2b", "border": "#2a3d5c", "focus": "#5ea9ff"}
		headerBG, footerBG, footerText = "#0e1a2b", "#0a1220", "#e6eaf0"
	default:
		palette = map[string]string{"background": "#ffffff", "surface": "#ffffff", "surfaceMuted": "#f4f6f8", "text": "#17212b", "textMuted": "#667085", "heading": "#101828", "primary": "#2563eb", "primaryHover": "#1d4ed8", "primaryContrast": "#ffffff", "secondary": "#475467", "secondaryHover": "#344054", "secondaryContrast": "#ffffff", "border": "#d0d5dd", "focus": "#84adff"}
		headerBG, footerBG, footerText = "#ffffff", "#101828", "#eaecf0"
	}
	styles := map[string]any{}
	for k, v := range palette {
		styles["colors."+k] = v
	}
	styles["header.background"] = headerBG
	styles["footer.background"] = footerBG
	styles["footer.textColor"] = footerText
	// Keep link colors coherent with palette so no Default Theme blue leaks into
	// primary interactions or focus ring.
	styles["links.color"] = palette["primary"]
	styles["links.hoverColor"] = palette["primaryHover"]
	styles["colors.focus"] = palette["focus"]
	return styles
}

func headerStyles(id HeaderStyleID) map[string]any {
	switch id {
	case HeaderMinimal:
		return map[string]any{"header.layout": "minimal", "header.width": "wide", "header.height": 64, "header.border": false, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": false, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "right", "navigation.gap": 28, "navigation.fontSize": 15, "navigation.fontWeight": 600}
	case HeaderCentered:
		return map[string]any{"header.layout": "centered", "header.width": "wide", "header.height": 96, "header.border": false, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": false, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "center", "navigation.gap": 26, "navigation.fontSize": 15, "navigation.fontWeight": 600}
	case HeaderStacked:
		return map[string]any{"header.layout": "stacked", "header.width": "wide", "header.height": 112, "header.border": true, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": true, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "left", "navigation.gap": 24, "navigation.fontSize": 14, "navigation.fontWeight": 600}
	case HeaderBold:
		return map[string]any{"header.layout": "left", "header.width": "wide", "header.height": 88, "header.border": false, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": false, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "right", "navigation.gap": 28, "navigation.fontSize": 16, "navigation.fontWeight": 700}
	case HeaderEditorial:
		return map[string]any{"header.layout": "split", "header.width": "wide", "header.height": 132, "header.border": true, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": true, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "left", "navigation.gap": 22, "navigation.fontSize": 14, "navigation.fontWeight": 600}
	default: // classic
		return map[string]any{"header.layout": "left", "header.width": "wide", "header.height": 82, "header.border": true, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": true, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "right", "navigation.gap": 26, "navigation.fontSize": 15, "navigation.fontWeight": 600}
	}
}

func footerStyles(id FooterStyleID) map[string]any {
	switch id {
	case FooterSimple:
		return map[string]any{"footer.layout": "simple", "footer.width": "wide", "footer.spacing": 48, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	case FooterCentered:
		return map[string]any{"footer.layout": "centered", "footer.width": "wide", "footer.spacing": 64, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	case FooterStacked:
		return map[string]any{"footer.layout": "simple", "footer.width": "wide", "footer.spacing": 56, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	case FooterEditorial:
		return map[string]any{"footer.layout": "columns", "footer.width": "wide", "footer.spacing": 72, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	default: // split
		return map[string]any{"footer.layout": "split", "footer.width": "wide", "footer.spacing": 72, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	}
}

func composedStyles(preset PresetID, palette PaletteID, header HeaderStyleID, footer FooterStyleID) map[string]any {
	styles := structuralStarterStyles(preset)
	for k, v := range paletteStyles(palette) {
		styles[k] = v
	}
	for k, v := range headerStyles(header) {
		styles[k] = v
	}
	for k, v := range footerStyles(footer) {
		styles[k] = v
	}
	return styles
}

// presetStyles is kept for backward compat: produce the same map as composedStyles
// with the palette/header/footer defaults that were implicitly baked into the old per-preset switch.
func presetStyles(id PresetID) map[string]any {
	return composedStyles(id, DefaultPaletteForPreset(id), DefaultHeaderForPreset(id), DefaultFooterForPreset(id))
}

func specFor(preset Preset) presetSpec {
	return specForWithStyles(preset, composedStyles(preset.ID, DefaultPaletteForPreset(preset.ID), DefaultHeaderForPreset(preset.ID), DefaultFooterForPreset(preset.ID)))
}

func specForWithStyles(preset Preset, styles map[string]any) presetSpec {
	commonPage := []pageSpec{{Title: "About", Slug: "about", Body: "Use this page to introduce your work, values and approach."}}
	switch preset.ID {
	case PresetBlog:
		return presetSpec{preset: preset, archivePath: "/blog", pages: commonPage, seedEntries: []entrySpec{
			{Title: "Getting started", Slug: "getting-started", Excerpt: "A straightforward place to begin.", Body: "Start with the essentials, make one useful change, and build from there."},
			{Title: "A practical guide", Slug: "a-practical-guide", Excerpt: "A few useful principles for everyday work.", Body: "Clear priorities and small, deliberate steps make complicated work easier to manage."},
			{Title: "Behind the scenes", Slug: "behind-the-scenes", Excerpt: "A look at the process behind the finished work.", Body: "Good results usually come from careful preparation, useful feedback and patient revision."},
			{Title: "What we've learned", Slug: "what-we-have-learned", Excerpt: "Notes from recent work.", Body: "The most durable lessons are often simple: listen closely, test assumptions and keep the result useful."},
			{Title: "A short update", Slug: "a-short-update", Excerpt: "Recent news and what comes next.", Body: "Here is a concise update for readers, with more details to follow soon."},
		}, styles: styles}
	case PresetPortfolio:
		ct := customType("project", "Project", "Projects", "/work", true, true, []content.FieldDefinition{{Key: "client", Label: "Client", Type: content.FieldText}, {Key: "year", Label: "Year", Type: content.FieldText}, {Key: "services", Label: "Services", Type: content.FieldText}, {Key: "project_url", Label: "Project URL", Type: content.FieldURL}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/work", images: true, pages: append(commonPage, pageSpec{Title: "Contact", Slug: "contact", Body: "Tell us what you are working on and what kind of help you need.", Form: true}), form: &formSpec{Name: "Project enquiry"}, seedEntries: projectEntries(), styles: styles}
	case PresetLanding:
		ct := customType("testimonial", "Testimonial", "Testimonials", "", false, false, []content.FieldDefinition{{Key: "quote", Label: "Quote", Type: content.FieldTextarea}, {Key: "person", Label: "Person", Type: content.FieldText}, {Key: "role", Label: "Role", Type: content.FieldText}, {Key: "company", Label: "Company", Type: content.FieldText}})
		ct.Config.Features = content.ContentTypeFeatures{}
		return presetSpec{preset: preset, contentType: &ct, form: &formSpec{Name: "Request information"}, seedEntries: []entrySpec{
			{Title: "Maya Chen", Slug: "maya-chen", Fields: map[string]any{"quote": "The process was clear, thoughtful and easy to follow.", "person": "Maya Chen", "role": "Operations lead", "company": "Northline"}},
			{Title: "Sam Rivera", Slug: "sam-rivera", Fields: map[string]any{"quote": "We moved from an idea to a useful result without unnecessary complexity.", "person": "Sam Rivera", "role": "Founder", "company": "Common Field"}},
			{Title: "Alex Morgan", Slug: "alex-morgan", Fields: map[string]any{"quote": "Every decision was explained and the finished work feels like ours.", "person": "Alex Morgan", "role": "Director", "company": "Studio Lane"}},
			{Title: "Jordan Lee", Slug: "jordan-lee", Fields: map[string]any{"quote": "A practical partner from the first conversation to launch.", "person": "Jordan Lee", "role": "Team lead", "company": "Fieldwork"}},
		}, styles: styles}
	case PresetProducts:
		ct := customType("product", "Product", "Products", "/products", true, true, []content.FieldDefinition{{Key: "sku", Label: "SKU", Type: content.FieldText}, {Key: "price_display", Label: "Price", Type: content.FieldText}, {Key: "short_description", Label: "Short description", Type: content.FieldTextarea}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/products", images: true, pages: append(commonPage, pageSpec{Title: "Contact", Slug: "contact", Body: "Ask about specifications, availability or a custom requirement."}), seedEntries: productEntries(), styles: styles}
	case PresetSimpleSite:
		return presetSpec{preset: preset, pages: []pageSpec{
			{Title: "About", Slug: "about", Body: "Use this page to introduce your work, values and approach."},
			{Title: "Contact", Slug: "contact", Body: "Tell us what you need and we will get back to you.", Form: true},
		}, form: &formSpec{Name: "Contact"}, styles: styles}
	case PresetMagazine:
		return presetSpec{preset: preset, archivePath: "/blog", pages: commonPage, seedEntries: []entrySpec{
			{Title: "The future of thoughtful design", Slug: "future-thoughtful-design", Excerpt: "Where craft and clarity meet the new tools.", Body: "A look at how designers keep quality when everything moves faster."},
			{Title: "City stories: backstreets and frontlines", Slug: "city-stories", Excerpt: "Small narratives that shape the urban experience.", Body: "Conversations with makers working quietly across the city."},
			{Title: "Material matters", Slug: "material-matters", Excerpt: "Why the physical still matters.", Body: "From paper to pixels, materials shape how we read and feel."},
			{Title: "Slow work, sharp results", Slug: "slow-work", Excerpt: "Taking time as a competitive advantage.", Body: "How teams use constraint to create more resonant work."},
			{Title: "Editor's picks: this week", Slug: "editors-picks", Excerpt: "Five links worth your attention.", Body: "A curated list of reads that stayed with us this week."},
			{Title: "The interview: keeping it honest", Slug: "interview-honest", Excerpt: "Notes on listening well.", Body: "What we ask shapes what we hear."},
			{Title: "Weekend reading", Slug: "weekend-reading", Excerpt: "Go deeper when the feed quiets.", Body: "A small collection of long reads for slow weekends."},
			{Title: "Field report", Slug: "field-report", Excerpt: "Dispatches from a day outside the studio.", Body: "Photos and notes from a walk that became a story."},
		}, styles: styles}
	case PresetAgency:
		ct := customType("case_study", "Case Study", "Case Studies", "/case-studies", true, true, []content.FieldDefinition{{Key: "client", Label: "Client", Type: content.FieldText}, {Key: "year", Label: "Year", Type: content.FieldText}, {Key: "services", Label: "Services", Type: content.FieldText}, {Key: "summary", Label: "Summary", Type: content.FieldTextarea}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/case-studies", images: true, pages: []pageSpec{
			{Title: "Services", Slug: "services", Body: "We help teams clarify, design and ship useful work."},
			{Title: "About", Slug: "about", Body: "A small practical team focused on useful outcomes."},
			{Title: "Contact", Slug: "contact", Body: "Tell us what you are building.", Form: true},
		}, form: &formSpec{Name: "Contact"}, seedEntries: []entrySpec{
			{Title: "Northline launch", Slug: "northline-launch", Excerpt: "Brand and site for a new operations platform.", Body: "We shaped the story, system and site for launch.", Fields: map[string]any{"client": "Northline", "year": "2026", "services": "Brand, site", "summary": "A focused launch for a new platform."}},
			{Title: "Field Notes identity", Slug: "field-notes-identity", Excerpt: "Identity and editorial site for a research studio.", Body: "A restrained system for a research-driven studio.", Fields: map[string]any{"client": "Field Notes", "year": "2026", "services": "Identity, editorial", "summary": "Identity shaped by research."}},
			{Title: "Common Ground platform", Slug: "common-ground", Excerpt: "A shared workspace for distributed teams.", Body: "A calm tool for shared ownership.", Fields: map[string]any{"client": "Common Ground", "year": "2025", "services": "Product, site", "summary": "A calm platform for shared work."}},
			{Title: "Atlas rebrand", Slug: "atlas-rebrand", Excerpt: "A broader identity for an established maker.", Body: "We kept what worked and clarified the rest.", Fields: map[string]any{"client": "Atlas", "year": "2025", "services": "Rebrand", "summary": "A clearer identity, kept familiar."}},
			{Title: "Studio One site", Slug: "studio-one-site", Excerpt: "Portfolio site for a creative studio.", Body: "A site that lets the work lead.", Fields: map[string]any{"client": "Studio One", "year": "2025", "services": "Site", "summary": "Work first, ornament last."}},
			{Title: "Signal campaign", Slug: "signal-campaign", Excerpt: "Launch campaign for a focused product.", Body: "A campaign that stayed practical.", Fields: map[string]any{"client": "Signal", "year": "2026", "services": "Campaign, site", "summary": "A campaign that shipped on time."}},
		}, styles: styles}
	case PresetKnowledgeBase:
		ct := customType("article", "Article", "Articles", "/knowledge", true, true, []content.FieldDefinition{{Key: "summary", Label: "Summary", Type: content.FieldTextarea}, {Key: "category", Label: "Category", Type: content.FieldText}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/knowledge", pages: []pageSpec{
			{Title: "About", Slug: "about", Body: "Use this page to introduce your work, values and approach."},
		}, seedEntries: []entrySpec{
			{Title: "Getting started", Slug: "getting-started", Excerpt: "Everything you need for a first setup.", Body: "Follow the steps to get your site running quickly.", Fields: map[string]any{"summary": "A quick start for new users.", "category": "Guides"}},
			{Title: "Managing content", Slug: "managing-content", Excerpt: "How to create and organize pages and posts.", Body: "Content lives as Entries and revisions; publish when ready.", Fields: map[string]any{"summary": "Create and publish content safely.", "category": "Guides"}},
			{Title: "Custom content types", Slug: "custom-content-types", Excerpt: "Add structured types without code.", Body: "Define fields and keep presentation in templates.", Fields: map[string]any{"summary": "Extend content with fields.", "category": "Reference"}},
			{Title: "Media management", Slug: "media-management", Excerpt: "Upload, replace and organize images.", Body: "The media library stores variants and usage.", Fields: map[string]any{"summary": "Handle images and assets.", "category": "Guides"}},
			{Title: "SEO and sitemaps", Slug: "seo-sitemaps", Excerpt: "How search visibility is managed.", Body: "Managed robots and sitemaps keep crawling predictable.", Fields: map[string]any{"summary": "Control how search sees your site.", "category": "Guides"}},
			{Title: "Troubleshooting", Slug: "troubleshooting", Excerpt: "Common issues and fixes.", Body: "Start here when something seems off.", Fields: map[string]any{"summary": "Fix common setup problems.", "category": "Help"}},
		}, styles: styles}
	default:
		ct := customType("service", "Service", "Services", "/services", true, true, []content.FieldDefinition{{Key: "short_summary", Label: "Short summary", Type: content.FieldTextarea}, {Key: "service_area", Label: "Service area", Type: content.FieldText}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/services", pages: append(commonPage, pageSpec{Title: "Contact", Slug: "contact", Body: "Share a few details and we will respond with a practical next step.", Form: true}), form: &formSpec{Name: "Contact", Phone: true}, seedEntries: []entrySpec{
			{Title: "Consultation", Slug: "consultation", Excerpt: "A focused conversation to understand the work and recommend next steps.", Body: "We review what you need, answer initial questions and outline a practical way forward.", Fields: map[string]any{"short_summary": "Clear advice and a practical next step.", "service_area": "Local area and remote"}},
			{Title: "Installation", Slug: "installation", Excerpt: "Careful setup with clear communication.", Body: "Installation is planned around your space, priorities and agreed schedule.", Fields: map[string]any{"short_summary": "Careful setup from start to finish.", "service_area": "Local area"}},
			{Title: "Maintenance", Slug: "maintenance", Excerpt: "Routine care that keeps things working well.", Body: "Regular maintenance helps identify small issues early and supports reliable day-to-day use.", Fields: map[string]any{"short_summary": "Routine care for reliable operation.", "service_area": "Local area"}},
			{Title: "Emergency support", Slug: "emergency-support", Excerpt: "Responsive help when an urgent issue needs attention.", Body: "Contact us with the details and we will explain the available response options.", Fields: map[string]any{"short_summary": "Responsive help for urgent issues.", "service_area": "Selected local areas"}},
			{Title: "Custom service", Slug: "custom-service", Excerpt: "A flexible option for work that does not fit a standard package.", Body: "We can scope a tailored service after a short conversation about your requirements.", Fields: map[string]any{"short_summary": "A tailored approach for specific requirements.", "service_area": "By arrangement"}},
		}, styles: styles}
	}
}

func specForPlan(plan Plan) presetSpec {
	styles := composedStyles(plan.Preset.ID, plan.Input.PaletteID, plan.Input.HeaderStyleID, plan.Input.FooterStyleID)
	spec := specForWithStyles(plan.Preset, styles)
	// Localize structural copy for Creator language catalog (EN/PL)
	lang := plan.Input.Language
	if lang == "" {
		lang = "en"
	}
	spec.seedEntries = localizedSeedEntries(plan.Preset.ID, lang)
	spec.pages = localizedPages(plan.Preset.ID, lang)
	// Localize Creator-owned content type labels for PL (IDs remain stable)
	if spec.contentType != nil && lang == "pl" {
		id := string(spec.contentType.ID)
		if v := copyFor(lang, "contenttype."+id+".singular"); v != "contenttype."+id+".singular" {
			spec.contentType.Name = v
		}
		if v := copyFor(lang, "contenttype."+id+".plural"); v != "contenttype."+id+".plural" {
			spec.contentType.PluralName = v
		}
	}
	// Localize form name via catalog
	if spec.form != nil {
		switch spec.preset.ID {
		case PresetPortfolio:
			spec.form.Name = copyFor(lang, "form.project_enquiry")
		case PresetLanding:
			spec.form.Name = copyFor(lang, "form.request_info")
		default:
			if spec.form.Name == "Contact" {
				spec.form.Name = copyFor(lang, "form.contact")
			}
		}
	}
	return spec
}

func localizedSeedEntries(preset PresetID, lang string) []entrySpec {
	switch preset {
	case PresetBlog:
		return localizedBlogEntries(lang)
	case PresetPortfolio:
		return localizedProjectEntries(lang)
	case PresetLanding:
		return localizedTestimonialEntries(lang)
	case PresetProducts:
		return localizedProductEntries(lang)
	case PresetSimpleSite:
		return []entrySpec{}
	case PresetMagazine:
		return localizedBlogEntries(lang) // magazine uses same copy for v1, richer homepage handled via docs
	case PresetAgency:
		return localizedAgencyEntries(lang)
	case PresetKnowledgeBase:
		return localizedKnowledgeEntries(lang)
	default:
		// Local Business
		return localizedServiceEntries(lang)
	}
}

func localizedPages(preset PresetID, lang string) []pageSpec {
	baseBodyEn := "Use this page to introduce your work, values and approach."
	baseBodyPl := copyFor(lang, "page.about.body")
	if lang != "pl" {
		baseBodyPl = baseBodyEn
	}
	switch preset {
	case PresetBlog:
		return []pageSpec{{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl}}
	case PresetPortfolio:
		return []pageSpec{
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: copyFor(lang, "page.contact.body.portfolio"), Form: true},
		}
	case PresetLanding:
		return []pageSpec{}
	case PresetProducts:
		return []pageSpec{
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: copyFor(lang, "page.contact.body.product")},
		}
	case PresetSimpleSite:
		return []pageSpec{
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: copyFor(lang, "page.contact.body.simple"), Form: true},
		}
	case PresetMagazine:
		return []pageSpec{{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl}}
	case PresetAgency:
		return []pageSpec{
			{Title: copyFor(lang, "heading.services"), Slug: "services", Body: copyFor(lang, "page.services.body")},
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: copyFor(lang, "page.contact.body.agency"), Form: true},
		}
	case PresetKnowledgeBase:
		return []pageSpec{{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl}}
	default:
		return []pageSpec{
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: copyFor(lang, "page.contact.body.local"), Form: true},
		}
	}
}

func customType(id, singular, plural, base string, single, archive bool, fields []content.FieldDefinition) content.ContentTypeInput {
	return content.ContentTypeInput{ID: content.ContentTypeID(id), Name: singular, PluralName: plural, Config: content.ContentTypeConfig{SchemaVersion: 2, Fields: fields, Features: content.ContentTypeFeatures{Content: true, Excerpt: true, FeaturedMedia: true, SEO: true}, Routing: content.ContentTypeRouting{Single: single, Archive: archive, BasePath: base}}}
}

func projectEntries() []entrySpec {
	names := []string{"North House", "Field Notes", "Atlas Identity", "Studio One", "Common Ground", "Signal"}
	out := make([]entrySpec, 0, len(names))
	for i, name := range names {
		out = append(out, entrySpec{Title: name, Slug: []string{"north-house", "field-notes", "atlas-identity", "studio-one", "common-ground", "signal"}[i], Excerpt: "A concise example project showing the brief, process and outcome.", Body: "This project page is ready for your own story, images and project details.", Fields: map[string]any{"client": "Fictional client", "year": "2026", "services": "Direction, design"}})
	}
	return out
}

func productEntries() []entrySpec {
	names := []string{"Form Chair", "Arc Lamp", "Studio Table", "Field Shelf", "Mono Tray", "Line Stool"}
	out := make([]entrySpec, 0, len(names))
	for i, name := range names {
		out = append(out, entrySpec{Title: name, Slug: []string{"form-chair", "arc-lamp", "studio-table", "field-shelf", "mono-tray", "line-stool"}[i], Excerpt: "A considered object for everyday spaces.", Body: "Add materials, dimensions and other product details here. Enquiries can be handled through your normal contact channels.", Fields: map[string]any{"sku": "SAMPLE-" + string(rune('A'+i)), "price_display": []string{"$240", "$180", "From $640", "$320", "$75", "$210"}[i], "short_description": "A simple, durable design intended for everyday use."}})
	}
	return out
}
