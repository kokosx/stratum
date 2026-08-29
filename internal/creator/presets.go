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

type Input struct {
	SiteTitle string
	Tagline   string
	PresetID  PresetID
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

func specFor(preset Preset) presetSpec {
	commonPage := []pageSpec{{Title: "About", Slug: "about", Body: "Use this page to introduce your work, values and approach."}}
	switch preset.ID {
	case PresetBlog:
		return presetSpec{preset: preset, archivePath: "/blog", pages: commonPage, seedEntries: []entrySpec{
			{Title: "Getting started", Slug: "getting-started", Excerpt: "A straightforward place to begin.", Body: "Start with the essentials, make one useful change, and build from there."},
			{Title: "A practical guide", Slug: "a-practical-guide", Excerpt: "A few useful principles for everyday work.", Body: "Clear priorities and small, deliberate steps make complicated work easier to manage."},
			{Title: "Behind the scenes", Slug: "behind-the-scenes", Excerpt: "A look at the process behind the finished work.", Body: "Good results usually come from careful preparation, useful feedback and patient revision."},
			{Title: "What we've learned", Slug: "what-we-have-learned", Excerpt: "Notes from recent work.", Body: "The most durable lessons are often simple: listen closely, test assumptions and keep the result useful."},
			{Title: "A short update", Slug: "a-short-update", Excerpt: "Recent news and what comes next.", Body: "Here is a concise update for readers, with more details to follow soon."},
		}, styles: presetStyles(PresetBlog)}
	case PresetPortfolio:
		ct := customType("project", "Project", "Projects", "/work", true, true, []content.FieldDefinition{{Key: "client", Label: "Client", Type: content.FieldText}, {Key: "year", Label: "Year", Type: content.FieldText}, {Key: "services", Label: "Services", Type: content.FieldText}, {Key: "project_url", Label: "Project URL", Type: content.FieldURL}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/work", images: true, pages: append(commonPage, pageSpec{Title: "Contact", Slug: "contact", Body: "Tell us what you are working on and what kind of help you need.", Form: true}), form: &formSpec{Name: "Project enquiry"}, seedEntries: projectEntries(), styles: presetStyles(PresetPortfolio)}
	case PresetLanding:
		ct := customType("testimonial", "Testimonial", "Testimonials", "", false, false, []content.FieldDefinition{{Key: "quote", Label: "Quote", Type: content.FieldTextarea}, {Key: "person", Label: "Person", Type: content.FieldText}, {Key: "role", Label: "Role", Type: content.FieldText}, {Key: "company", Label: "Company", Type: content.FieldText}})
		ct.Config.Features = content.ContentTypeFeatures{}
		return presetSpec{preset: preset, contentType: &ct, form: &formSpec{Name: "Request information"}, seedEntries: []entrySpec{
			{Title: "Maya Chen", Slug: "maya-chen", Fields: map[string]any{"quote": "The process was clear, thoughtful and easy to follow.", "person": "Maya Chen", "role": "Operations lead", "company": "Northline"}},
			{Title: "Sam Rivera", Slug: "sam-rivera", Fields: map[string]any{"quote": "We moved from an idea to a useful result without unnecessary complexity.", "person": "Sam Rivera", "role": "Founder", "company": "Common Field"}},
			{Title: "Alex Morgan", Slug: "alex-morgan", Fields: map[string]any{"quote": "Every decision was explained and the finished work feels like ours.", "person": "Alex Morgan", "role": "Director", "company": "Studio Lane"}},
			{Title: "Jordan Lee", Slug: "jordan-lee", Fields: map[string]any{"quote": "A practical partner from the first conversation to launch.", "person": "Jordan Lee", "role": "Team lead", "company": "Fieldwork"}},
		}, styles: presetStyles(PresetLanding)}
	case PresetProducts:
		ct := customType("product", "Product", "Products", "/products", true, true, []content.FieldDefinition{{Key: "sku", Label: "SKU", Type: content.FieldText}, {Key: "price_display", Label: "Price", Type: content.FieldText}, {Key: "short_description", Label: "Short description", Type: content.FieldTextarea}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/products", images: true, pages: append(commonPage, pageSpec{Title: "Contact", Slug: "contact", Body: "Ask about specifications, availability or a custom requirement."}), seedEntries: productEntries(), styles: presetStyles(PresetProducts)}
	default:
		ct := customType("service", "Service", "Services", "/services", true, true, []content.FieldDefinition{{Key: "short_summary", Label: "Short summary", Type: content.FieldTextarea}, {Key: "service_area", Label: "Service area", Type: content.FieldText}})
		return presetSpec{preset: preset, contentType: &ct, archivePath: "/services", pages: append(commonPage, pageSpec{Title: "Contact", Slug: "contact", Body: "Share a few details and we will respond with a practical next step.", Form: true}), form: &formSpec{Name: "Contact", Phone: true}, seedEntries: []entrySpec{
			{Title: "Consultation", Slug: "consultation", Excerpt: "A focused conversation to understand the work and recommend next steps.", Body: "We review what you need, answer initial questions and outline a practical way forward.", Fields: map[string]any{"short_summary": "Clear advice and a practical next step.", "service_area": "Local area and remote"}},
			{Title: "Installation", Slug: "installation", Excerpt: "Careful setup with clear communication.", Body: "Installation is planned around your space, priorities and agreed schedule.", Fields: map[string]any{"short_summary": "Careful setup from start to finish.", "service_area": "Local area"}},
			{Title: "Maintenance", Slug: "maintenance", Excerpt: "Routine care that keeps things working well.", Body: "Regular maintenance helps identify small issues early and supports reliable day-to-day use.", Fields: map[string]any{"short_summary": "Routine care for reliable operation.", "service_area": "Local area"}},
			{Title: "Emergency support", Slug: "emergency-support", Excerpt: "Responsive help when an urgent issue needs attention.", Body: "Contact us with the details and we will explain the available response options.", Fields: map[string]any{"short_summary": "Responsive help for urgent issues.", "service_area": "Selected local areas"}},
			{Title: "Custom service", Slug: "custom-service", Excerpt: "A flexible option for work that does not fit a standard package.", Body: "We can scope a tailored service after a short conversation about your requirements.", Fields: map[string]any{"short_summary": "A tailored approach for specific requirements.", "service_area": "By arrangement"}},
		}, styles: presetStyles(PresetLocalBusiness)}
	}
}

func presetStyles(id PresetID) map[string]any {
	styles := map[string]any{
		"typography.fontBody": "systemSans", "typography.fontHeading": "systemSans", "typography.bodySize": 17, "typography.headingWeight": 700, "typography.h1Size": 56,
		"layout.contentWidth": 1040, "layout.wideWidth": 1440, "layout.pagePadding": 24,
		"header.layout": "left", "header.width": "wide", "header.height": 72, "header.background": "#ffffff", "header.border": true, "header.shadow": "none", "header.showSiteTitle": true, "header.showTagline": false,
		"navigation.layout": "horizontal", "navigation.style": "plain", "navigation.align": "right", "navigation.gap": 24,
		"footer.layout": "split", "footer.width": "wide", "footer.background": "#17212b", "footer.textColor": "#f4f6f8", "footer.spacing": 40,
		"buttons.primaryStyle": "solid", "buttons.secondaryStyle": "outline", "buttons.radius": "sm",
		"radius.sm": 3, "radius.md": 6, "radius.lg": 10, "shadow.sm": "none", "shadow.md": "none", "shadow.lg": "none",
	}
	palette := map[string]string{}
	switch id {
	case PresetBlog:
		palette = map[string]string{"background": "#fbf8f4", "surface": "#fffdf9", "surfaceMuted": "#f1e9e1", "text": "#382f2b", "textMuted": "#75645d", "heading": "#241c19", "primary": "#8b3a3a", "primaryHover": "#6f2d2d", "primaryContrast": "#ffffff", "secondary": "#5f514b", "secondaryHover": "#453a35", "secondaryContrast": "#ffffff", "border": "#d8ccc3", "focus": "#c78484"}
		styles["typography.fontBody"], styles["typography.fontHeading"], styles["typography.bodySize"], styles["typography.headingWeight"], styles["typography.h1Size"] = "editorialSerif", "editorialSerif", 18, 600, 60
		styles["layout.contentWidth"], styles["layout.wideWidth"], styles["layout.pagePadding"] = 820, 1180, 24
		styles["header.layout"], styles["header.width"], styles["header.background"], styles["header.height"] = "minimal", "content", "#fbf8f4", 70
		styles["navigation.gap"], styles["footer.layout"], styles["footer.width"], styles["footer.background"], styles["footer.textColor"], styles["footer.spacing"] = 20, "simple", "content", "#2f2521", "#f5eee8", 34
		styles["radius.md"], styles["radius.lg"] = 2, 4
	case PresetPortfolio:
		palette = map[string]string{"background": "#f7f7f5", "surface": "#ffffff", "surfaceMuted": "#ececea", "text": "#242424", "textMuted": "#686868", "heading": "#0c0c0c", "primary": "#111111", "primaryHover": "#383838", "primaryContrast": "#ffffff", "secondary": "#6b6b6b", "secondaryHover": "#444444", "secondaryContrast": "#ffffff", "border": "#cececa", "focus": "#777777"}
		styles["typography.fontBody"], styles["typography.fontHeading"], styles["typography.bodySize"], styles["typography.headingWeight"], styles["typography.h1Size"] = "modernSans", "modernSans", 17, 600, 72
		styles["layout.contentWidth"], styles["layout.wideWidth"], styles["layout.pagePadding"] = 1120, 1560, 32
		styles["header.layout"], styles["header.width"], styles["header.background"], styles["header.height"], styles["header.border"] = "minimal", "wide", "#f7f7f5", 68, false
		styles["navigation.gap"], styles["footer.background"], styles["footer.textColor"], styles["footer.spacing"] = 28, "#111111", "#f2f2f0", 36
		styles["radius.sm"], styles["radius.md"], styles["radius.lg"] = 0, 0, 0
		styles["buttons.radius"] = "none"
	case PresetLanding:
		palette = map[string]string{"background": "#faf8ff", "surface": "#ffffff", "surfaceMuted": "#f0ebfa", "text": "#302a3a", "textMuted": "#6d6479", "heading": "#211a2d", "primary": "#6842a8", "primaryHover": "#533287", "primaryContrast": "#ffffff", "secondary": "#cf9b32", "secondaryHover": "#ab7c22", "secondaryContrast": "#211a2d", "border": "#d8cfe5", "focus": "#a98bd2"}
		styles["typography.fontBody"], styles["typography.fontHeading"], styles["typography.bodySize"], styles["typography.headingWeight"], styles["typography.h1Size"] = "humanistSans", "humanistSans", 18, 750, 66
		styles["layout.contentWidth"], styles["layout.wideWidth"], styles["layout.pagePadding"] = 940, 1280, 24
		styles["header.layout"], styles["header.background"], styles["header.height"] = "minimal", "#faf8ff", 70
		styles["navigation.style"], styles["footer.layout"], styles["footer.background"], styles["footer.textColor"] = "underline", "simple", "#211a2d", "#f7f1ff"
		styles["radius.md"], styles["radius.lg"] = 4, 6
	case PresetProducts:
		palette = map[string]string{"background": "#f8fafc", "surface": "#ffffff", "surfaceMuted": "#edf1f5", "text": "#29333e", "textMuted": "#647180", "heading": "#17212b", "primary": "#334e68", "primaryHover": "#243b53", "primaryContrast": "#ffffff", "secondary": "#7b8794", "secondaryHover": "#566574", "secondaryContrast": "#ffffff", "border": "#cbd4dd", "focus": "#829ab1"}
		styles["typography.fontBody"], styles["typography.fontHeading"], styles["typography.bodySize"], styles["typography.headingWeight"], styles["typography.h1Size"] = "humanistSans", "humanistSans", 17, 650, 62
		styles["layout.contentWidth"], styles["layout.wideWidth"], styles["layout.pagePadding"] = 1120, 1500, 28
		styles["header.layout"], styles["header.background"], styles["header.height"] = "split", "#ffffff", 74
		styles["navigation.gap"], styles["footer.background"], styles["footer.textColor"] = 26, "#243342", "#edf2f7"
		styles["radius.md"], styles["radius.lg"], styles["shadow.sm"] = 6, 8, "soft"
	case PresetLocalBusiness:
		palette = map[string]string{"background": "#fbfaf6", "surface": "#ffffff", "surfaceMuted": "#f1eee4", "text": "#33413c", "textMuted": "#6b756f", "heading": "#1e302a", "primary": "#356859", "primaryHover": "#285044", "primaryContrast": "#ffffff", "secondary": "#a4693b", "secondaryHover": "#82522e", "secondaryContrast": "#ffffff", "border": "#d7d5c9", "focus": "#78a99a"}
		styles["typography.fontBody"], styles["typography.fontHeading"], styles["typography.bodySize"], styles["typography.headingWeight"], styles["typography.h1Size"] = "humanistSans", "humanistSans", 17, 700, 58
		styles["layout.contentWidth"], styles["layout.wideWidth"], styles["layout.pagePadding"] = 980, 1320, 24
		styles["header.layout"], styles["header.background"], styles["header.height"] = "left", "#fbfaf6", 74
		styles["navigation.gap"], styles["footer.background"], styles["footer.textColor"], styles["footer.spacing"] = 22, "#263b34", "#f4f1e8", 38
		styles["radius.md"], styles["radius.lg"] = 5, 8
	}
	for key, value := range palette {
		styles["colors."+key] = value
	}
	return styles
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
