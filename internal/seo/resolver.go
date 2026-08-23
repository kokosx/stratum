package seo

import (
	"strings"

	"github.com/kokosx/stratum/internal/content"
)

// SiteSEO holds the global site-level SEO defaults.
// It is derived from site_settings and the runtime snapshot.
type SiteSEO struct {
	Title               string
	TitleSeparator      string
	SiteURL             string
	Language            string
	IndexingEnabled     bool // true = allow indexing
	GlobalSocialMediaID string
	TwitterSite         string // e.g. "@stratum" or "https://x.com/stratum"; empty means no tag
}

// ContentTypeSEO holds optional per-content-type defaults.
// nil values mean "inherit" from the site level. The resolver treats
// nil as inheritable so the final precedence is
// Site → ContentType → Revision.
type ContentTypeSEO struct {
	RobotsIndex  *bool // nil = inherit, true = index, false = noindex
	RobotsFollow *bool // nil = inherit, true = follow, false = nofollow
}

// RevisionSEO holds the per-revision overrides stored on entry_revisions.
// Empty strings and nil pointers mean "inherit / not set".
type RevisionSEO struct {
	Title          string
	Excerpt        string
	SeoTitle       string
	SeoDescription string
	CanonicalURL   string

	FeaturedMediaID string
	SocialMediaID   string

	RobotsIndex  *bool // nil = inherit
	RobotsFollow *bool // nil = inherit
}

// Input is the complete, typed SEO input for a single page render.
// The resolver never touches the database; callers build this from
// site snapshot, content type, and the published revision.
type Input struct {
	Site          SiteSEO
	ContentType   *ContentTypeSEO
	Revision      RevisionSEO
	ContentTypeID string // "page", "post", or "" (unknown defaults to website)
	Path          string // request path, e.g. "/about"
	Origin        string // request origin, e.g. "https://example.com" (used only when SiteURL is empty)
}

// OpenGraphView is the typed contract for Open Graph tags.
// The theme renders these; the resolver populates them. Image is always an
// absolute URL (including SiteURL/Origin prefix) and Width/Height/Type/Alt
// describe that single image (the social preview derivative, not a srcset).
type OpenGraphView struct {
	Title       string
	Description string
	URL         string // absolute canonical URL
	Type        string // "website" or "article"
	Image       string // absolute URL to the 1200x630 variant or fallback; empty when none
	ImageSecure string // set when Image is https
	ImageWidth  int
	ImageHeight int
	ImageType   string // mime type, e.g. "image/jpeg"
	ImageAlt    string
	SiteName    string
	Locale      string
}

// TwitterView is the typed contract for Twitter Card tags.
// It mirrors the Open Graph payload (single social SEO system) and always
// uses summary_large_image. Site is optional (twitter:site).
type TwitterView struct {
	Card        string // always "summary_large_image"
	Title       string
	Description string
	Image       string // absolute URL, same as OpenGraph.Image
	ImageAlt    string
	Site        string // optional "@handle" for twitter:site
}

// AlternateView is a typed alternate link (hreflang, canonical variant).
type AlternateView struct {
	Href     string
	HrefLang string
	Type     string
}

// Resolved is the final, precedence-resolved SEO model.
// Theme templates receive this (via HeadView/SEOView) and only render it.
type Resolved struct {
	Title       string // fully resolved title including site suffix and separator
	RawTitle    string // seo_title or title, without site suffix
	Description string // seo_description → excerpt → "" (never tagline)
	Canonical   string
	Robots      string // resolved robots directive; "max-image-preview:large" when indexable, e.g. "noindex,nofollow" otherwise
	// Structured tri-state for callers that need it:
	RobotsIndex  bool
	RobotsFollow bool

	FeaturedMediaID     string // from published revision only
	SocialMediaID       string
	GlobalSocialMediaID string // from site settings

	OGImageID string // chosen image id (social → featured → global), empty if none

	OpenGraph      OpenGraphView
	Twitter        TwitterView
	StructuredData string // JSON-LD, empty when not generated
	Alternates     []AlternateView
}

// Resolver is the central SEO resolver. It has no dependencies and is safe
// to use from any goroutine. The precedence is:
//
//	Site defaults → Content Type defaults → Entry Revision overrides → resolved
type Resolver struct{}

// New returns a resolver.
func New() *Resolver { return &Resolver{} }

// Resolve computes the final SEO view for a page.
func (r *Resolver) Resolve(in Input) Resolved {
	raw := strings.TrimSpace(in.Revision.SeoTitle)
	if raw == "" {
		raw = strings.TrimSpace(in.Revision.Title)
	}

	separator := strings.TrimSpace(in.Site.TitleSeparator)
	if separator == "" {
		separator = "–"
	}

	title := raw
	if raw != "" && strings.TrimSpace(in.Site.Title) != "" {
		// Do not duplicate the site title when the raw already equals it.
		if raw != in.Site.Title {
			title = raw + " " + separator + " " + in.Site.Title
		} else {
			title = raw
		}
	} else if raw == "" {
		title = strings.TrimSpace(in.Site.Title)
	}

	// Description fallback: seo_description → excerpt → "" (never site tagline).
	desc := strings.TrimSpace(in.Revision.SeoDescription)
	if desc == "" {
		desc = strings.TrimSpace(in.Revision.Excerpt)
	}

	canonical := Canonical(in.Site.SiteURL, in.Origin, in.Path, strings.TrimSpace(in.Revision.CanonicalURL))

	// Robots: site → content type → revision, each nullable.
	index := in.Site.IndexingEnabled
	follow := in.Site.IndexingEnabled
	if in.ContentType != nil {
		if in.ContentType.RobotsIndex != nil {
			index = *in.ContentType.RobotsIndex
		}
		if in.ContentType.RobotsFollow != nil {
			follow = *in.ContentType.RobotsFollow
		}
	}
	if in.Revision.RobotsIndex != nil {
		index = *in.Revision.RobotsIndex
	}
	if in.Revision.RobotsFollow != nil {
		follow = *in.Revision.RobotsFollow
	}
	robots := robotsString(index, follow)

	featured := strings.TrimSpace(in.Revision.FeaturedMediaID)
	social := strings.TrimSpace(in.Revision.SocialMediaID)
	global := strings.TrimSpace(in.Site.GlobalSocialMediaID)

	ogImageID := social
	if ogImageID == "" {
		ogImageID = featured
	}
	if ogImageID == "" {
		ogImageID = global
	}

	// Absolute image URL for social meta: join origin with the dedicated
	// 1200x630 preview path. The public handler will later replace this with
	// the true variant URL (including fallback logic for older rows/GIFs) and
	// fill width/height/type/alt from the media service, but the resolver
	// already guarantees an absolute URL.
	ogImage := ""
	if ogImageID != "" {
		base := BaseURL(in.Site.SiteURL, in.Origin)
		// Prefer the social derivative path; media service fallback is handled later.
		ogImage = base + "/media/" + ogImageID + "/social"
	}

	def := content.DefinitionFor(strings.TrimSpace(in.ContentTypeID))
	ogType := def.SEO.OpenGraphType
	if ogType == "" {
		ogType = "website"
	}

	twitterCard := "summary_large_image"

		og := OpenGraphView{
			Title:       raw,
			Description: desc,
			URL:         canonical,
			Type:        ogType,
			Image:       ogImage,
			SiteName:    strings.TrimSpace(in.Site.Title),
			Locale:      OGLocale(in.Site.Language),
		}
		tw := TwitterView{
			Card:        twitterCard,
			Title:       raw,
			Description: desc,
			Image:       ogImage,
			Site:        strings.TrimSpace(in.Site.TwitterSite),
		}

	return Resolved{
		Title:               title,
		RawTitle:            raw,
		Description:         desc,
		Canonical:           canonical,
		Robots:              robots,
		RobotsIndex:         index,
		RobotsFollow:        follow,
		FeaturedMediaID:     featured,
		SocialMediaID:       social,
		GlobalSocialMediaID: global,
		OGImageID:           ogImageID,
		OpenGraph:           og,
		Twitter:             tw,
		Alternates:          nil,
	}
}

func robotsString(index, follow bool) string {
	// Indexable public pages advertise large image previews by default; the
	// directive is harmless for crawlers that do not support it.
	if index && follow {
		return "max-image-preview:large"
	}
	var parts []string
	if !index {
		parts = append(parts, "noindex")
	}
	if !follow {
		parts = append(parts, "nofollow")
	}
	return strings.Join(parts, ",")
}

// OGLocale converts a BCP 47 language tag into the underscore form Open Graph
// expects (en_US). Bare primary tags (en, pl) pass through unchanged.
func OGLocale(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return ""
	}
	return strings.ReplaceAll(lang, "-", "_")
}

// Helper for tri-state bool pointer creation.
func BoolPtr(b bool) *bool { return &b }

// NullBoolToPtr converts a sql.NullInt64 / integer nullable to *bool.
// Returns nil for invalid/null, otherwise pointer to true/false.
func NullIntToBoolPtr(valid bool, v int64) *bool {
	if !valid {
		return nil
	}
	b := v != 0
	return &b
}
