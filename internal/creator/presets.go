package creator

import "github.com/kokosx/stratum/internal/content"

type PresetID string

const (
	PresetBlog          PresetID = "blog"
	PresetPortfolio     PresetID = "portfolio"
	PresetLanding       PresetID = "landing"
	PresetProducts      PresetID = "products"
	PresetLocalBusiness PresetID = "local-business"
)

type Preset struct {
	ID          PresetID
	Name        string
	Description string
	Pages       []string
	Dynamic     string
	Templates   []string
}

type PaletteID string

const (
	PaletteInk    PaletteID = "ink"
	PaletteClay   PaletteID = "clay"
	PaletteForest PaletteID = "forest"
	PaletteIndigo PaletteID = "indigo"
)

type HeaderStyleID string

const (
	HeaderMinimal  HeaderStyleID = "minimal"
	HeaderClassic  HeaderStyleID = "classic"
	HeaderCentered HeaderStyleID = "centered"
)

type FooterStyleID string

const (
	FooterSimple   FooterStyleID = "simple"
	FooterSplit    FooterStyleID = "split"
	FooterCentered FooterStyleID = "centered"
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
	BlogLatestCount              int    // 5 or 8
	BlogArchiveCount             int    // 10 or 20
	PortfolioColumns             int    // 2 or 3
	ProductColumns               int    // 3 or 4
	ProductMediaPosition         string // "left" or "right"
	LandingTestimonialsColumns   int    // 1 or 2
	ServiceColumns               int    // 2 or 3

	// K site details
	Language       string // "en" or "pl" for Creator v1
	Timezone       string // IANA
	SiteRepresents string // "organization" or "person"
	IndexingEnabled bool  // true = allow indexing, default false (discourage)
	SiteURL        string
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
	{ID: PresetBlog, Name: "Blog", Description: "A clean editorial site with real posts and a dynamic latest-posts collection.", Pages: []string{"Home", "About"}, Dynamic: "Posts - 5 examples", Templates: []string{"Page", "Homepage", "Post", "Posts Archive"}},
	{ID: PresetPortfolio, Name: "Portfolio", Description: "A project-focused site for studios, freelancers and creative teams.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Projects - 6 examples", Templates: []string{"Page", "Homepage", "Project", "Projects Archive"}},
	{ID: PresetLanding, Name: "Landing Page", Description: "A focused lead-generation site with dynamic, route-less testimonials.", Pages: []string{"Home"}, Dynamic: "Testimonials - 4 examples", Templates: []string{"Page", "Homepage"}},
	{ID: PresetProducts, Name: "Product Showcase", Description: "A catalog-style product website without cart, checkout or payments.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Products - 6 examples", Templates: []string{"Page", "Homepage", "Product", "Products Archive"}},
	{ID: PresetLocalBusiness, Name: "Local Business", Description: "A warm, practical local-service website with services and contact details.", Pages: []string{"Home", "About", "Contact"}, Dynamic: "Services - 5 examples", Templates: []string{"Page", "Homepage", "Service", "Services Archive"}},
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
}

var headerOptions = []HeaderOption{
	{ID: HeaderMinimal, Name: "Minimal", Description: "Compact, no border. Good for portfolios."},
	{ID: HeaderClassic, Name: "Classic", Description: "Balanced with subtle border. General-purpose."},
	{ID: HeaderCentered, Name: "Centered", Description: "Brand centered, navigation below."},
}

var footerOptions = []FooterOption{
	{ID: FooterSimple, Name: "Simple", Description: "Site name + copyright only."},
	{ID: FooterSplit, Name: "Split", Description: "Brand left, navigation right, copyright below."},
	{ID: FooterCentered, Name: "Centered", Description: "Centered stack with navigation."},
}

func Palettes() []Palette           { return append([]Palette(nil), palettes...) }
func HeaderOptions() []HeaderOption { return append([]HeaderOption(nil), headerOptions...) }
func FooterOptions() []FooterOption { return append([]FooterOption(nil), footerOptions...) }

var paletteSet = map[PaletteID]bool{PaletteInk: true, PaletteClay: true, PaletteForest: true, PaletteIndigo: true}
var headerSet = map[HeaderStyleID]bool{HeaderMinimal: true, HeaderClassic: true, HeaderCentered: true}
var footerSet = map[FooterStyleID]bool{FooterSimple: true, FooterSplit: true, FooterCentered: true}

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
	switch p {
	case PresetBlog:
		return "person"
	default:
		return "organization"
	}
}

func DefaultLanguageForPreset(p PresetID) string {
	_ = p
	return "en"
}

func DefaultTimezoneForPreset() string { return "UTC" }

func IsValidRepresents(v string) bool { return v == "organization" || v == "person" }
func IsValidProductMediaPosition(v string) bool { return v == "left" || v == "right" }
func IsValidCreatorLanguage(lang string) bool { return lang == "en" || lang == "pl" }

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
	Title string
	Slug  string
	Body  string
	Form  bool
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
		return map[string]any{"header.layout": "minimal", "header.width": "wide", "header.height": 68, "header.border": false, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": false, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "right", "navigation.gap": 28, "navigation.fontSize": 15, "navigation.fontWeight": 600}
	case HeaderCentered:
		return map[string]any{"header.layout": "centered", "header.width": "wide", "header.height": 104, "header.border": false, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": false, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "center", "navigation.gap": 26, "navigation.fontSize": 15, "navigation.fontWeight": 600}
	default: // classic
		return map[string]any{"header.layout": "left", "header.width": "wide", "header.height": 82, "header.border": true, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": true, "navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "right", "navigation.gap": 26, "navigation.fontSize": 15, "navigation.fontWeight": 600}
	}
}

func footerStyles(id FooterStyleID) map[string]any {
	switch id {
	case FooterSimple:
		return map[string]any{"footer.layout": "simple", "footer.width": "wide", "footer.spacing": 56, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	case FooterCentered:
		return map[string]any{"footer.layout": "centered", "footer.width": "wide", "footer.spacing": 72, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
	default: // split
		return map[string]any{"footer.layout": "split", "footer.width": "wide", "footer.spacing": 80, "footer.showSiteTitle": true, "footer.showCopyright": true, "footer.menuLayout": "inline"}
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
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: "Tell us what you are working on and what kind of help you need.", Form: true},
		}
	case PresetLanding:
		return []pageSpec{}
	case PresetProducts:
		return []pageSpec{
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: "Ask about specifications, availability or a custom requirement."},
		}
	default:
		return []pageSpec{
			{Title: copyFor(lang, "page.about.title"), Slug: "about", Body: baseBodyPl},
			{Title: copyFor(lang, "page.contact.title"), Slug: "contact", Body: "Share a few details and we will respond with a practical next step.", Form: true},
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
