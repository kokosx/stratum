package themes

import (
	"html/template"

	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/rendering"
)

type SiteView struct {
	Title    string
	Tagline  string
	Language string
	SiteURL  string
	LogoURL  string
	// LogoWidth/LogoHeight are the logo's intrinsic dimensions when known so
	// the header <img> can reserve layout space. Zero omits the attributes.
	LogoWidth  int
	LogoHeight int
}

type EntryView struct {
	Title          string
	SEOTitle       string
	SEODescription string
	CanonicalURL   string
}

// HeadView is the stable, semantic contract the theme uses to render the
// document <head>. The CMS supplies the data; the theme controls final markup.
type HeadView struct {
	Title       string
	Description string
	Canonical   string
	// Robots is empty when indexing is allowed; otherwise it holds the
	// comma-separated robots directive (e.g. "noindex,nofollow").
	Robots string
	// Speculation carries the generated Navigation Preloading configuration.
	Speculation SpeculationView
	// SiteIcon carries the generated favicon links, or nil when no Site Icon is set.
	SiteIcon *rendering.FaviconView
	// Preloads contains the single selected LCP image, when a document has one.
	Preloads []ImagePreload
	// OpenGraph carries the Open Graph view model (title, description, url,
	// type, image, site name, locale). Empty when not applicable.
	OpenGraph OpenGraphView
	// Twitter carries the Twitter Card view model.
	Twitter TwitterView
	// StructuredData is the JSON-LD payload (already encoded, safe for
	// <script type="application/ld+json">). Empty when not applicable.
	StructuredData template.JS
	// Alternates lists hreflang / alternate links.
	Alternates []AlternateView
	// SEO is the fully resolved SEO view (typed, semantic). The flat
	// Head fields above mirror SEO for backward compatibility; new code
	// should prefer Head.SEO.
	SEO SEOView
}

// SEOView is the typed, semantic SEO contract. The resolver populates it;
// the theme only renders it.
type SEOView struct {
	Title       string
	Description string
	Canonical   string
	Robots      string

	OpenGraph      OpenGraphView
	Twitter        TwitterView
	StructuredData template.JS
	Alternates     []AlternateView
	Favicon        *rendering.FaviconView
}

// OpenGraphView is the typed Open Graph model. Image is an absolute URL to
// the single 1200x630 social preview (never a srcset); dimensions and type
// come from the stored media variant so crawlers get width/height/type/alt.
type OpenGraphView struct {
	Title       string
	Description string
	URL         string // absolute canonical URL
	Type        string // "website" or "article"
	Image       string // absolute URL; empty when none
	ImageSecure string // https image URL when applicable
	ImageWidth  int
	ImageHeight int
	ImageType   string // e.g. "image/jpeg"
	ImageAlt    string
	SiteName    string
	Locale      string
}

// TwitterView is the typed Twitter Card model. It shares the same resolved
// social image as Open Graph (single Social SEO system).
type TwitterView struct {
	Card        string // always "summary_large_image"
	Title       string
	Description string
	Image       string // absolute URL, same as OpenGraph.Image
	ImageAlt    string
	Site        string // optional twitter:site handle
}

// AlternateView is a typed hreflang alternate.
type AlternateView struct {
	Href     string
	HrefLang string
	Type     string
}

type ImagePreload struct {
	Href   string
	SrcSet string
	Sizes  string
}

// SpeculationView exposes the safe, server-generated Speculation Rules payload.
// RulesJSON is produced with encoding/json and must never be built by string
// concatenation.
type SpeculationView struct {
	Enabled   bool
	Mode      string
	Eagerness string
	RulesJSON template.JS
}

type ThemeView struct {
	ID       string
	Version  int
	Settings map[string]any
}

// AssetsView carries the fingerprinted, immutable CSS/JS URLs. The public
// runtime computes these from real content, so a changed theme or block set
// yields new URLs.
type AssetsView struct {
	BlocksCSS string
	ThemeCSS  string
	ThemeJS   string
}

type PageView struct {
	Site       SiteView
	Entry      EntryView
	Head       HeadView
	Theme      ThemeView
	Navigation map[string]navigation.Menu
	Content    template.HTML
	// PreviewCSS is generated exclusively by Theme Runtime after server-side
	// validation. Public renders leave it empty and use /stratum/theme.css.
	PreviewCSS template.CSS
	// Assets holds the fingerprinted stylesheet and script URLs.
	Assets AssetsView
}
