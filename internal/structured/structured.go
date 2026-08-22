// Package structured builds Stratum's first-party Schema.org JSON-LD for
// Page/Post documents. Only entities that are actually useful today are
// modelled here; this deliberately does not aspire to be a universal
// Schema.org builder.
//
// The payload is assembled from typed Go structs with encoding/json — never by
// string concatenation — so the output is always well-formed and safely
// escaped for embedding inside <script type="application/ld+json">. All
// entities of one document live in a single @graph and reference each other
// through deterministic, URL-based @ids.
package structured

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/seo"
)

const schemaContext = "https://schema.org"

// Values for Site.Represents.
const (
	RepresentsOrganization = "organization"
	RepresentsPerson       = "person"
)

// Mode selects which page-level entity a document generates. It is the local
// override knob (Automatic | Disabled | WebPage | AboutPage | ContactPage);
// callers that have no override UI yet simply use ModeAutomatic.
type Mode string

const (
	ModeAutomatic   Mode = ""
	ModeDisabled    Mode = "disabled"
	ModeWebPage     Mode = "webpage"
	ModeAboutPage   Mode = "aboutpage"
	ModeContactPage Mode = "contactpage"
)

// --- Wire format -----------------------------------------------------------

type document struct {
	Context string `json:"@context"`
	Graph   []any  `json:"@graph"`
}

// ref points at another node in the same @graph by its stable @id.
type ref struct {
	ID string `json:"@id"`
}

type webSiteNode struct {
	Type       string `json:"@type"`
	ID         string `json:"@id"`
	Name       string `json:"name,omitempty"`
	URL        string `json:"url,omitempty"`
	InLanguage string `json:"inLanguage,omitempty"`
	Publisher  *ref   `json:"publisher,omitempty"`
}

type organizationNode struct {
	Type          string     `json:"@type"`
	ID            string     `json:"@id"`
	Name          string     `json:"name,omitempty"`
	AlternateName string     `json:"alternateName,omitempty"`
	URL           string     `json:"url,omitempty"`
	Logo          *imageNode `json:"logo,omitempty"`
	SameAs        []string   `json:"sameAs,omitempty"`
}

type personNode struct {
	Type   string   `json:"@type"`
	ID     string   `json:"@id,omitempty"`
	Name   string   `json:"name,omitempty"`
	URL    string   `json:"url,omitempty"`
	Image  string   `json:"image,omitempty"`
	Bio    string   `json:"description,omitempty"`
	SameAs []string `json:"sameAs,omitempty"`
}

type imageNode struct {
	Type        string `json:"@type"`
	ID          string `json:"@id,omitempty"`
	URL         string `json:"url,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Caption     string `json:"caption,omitempty"`
	Description string `json:"description,omitempty"`
}

type webPageNode struct {
	Type               string `json:"@type"`
	ID                 string `json:"@id"`
	URL                string `json:"url,omitempty"`
	Name               string `json:"name,omitempty"`
	Description        string `json:"description,omitempty"`
	InLanguage         string `json:"inLanguage,omitempty"`
	IsPartOf           *ref   `json:"isPartOf,omitempty"`
	PrimaryImageOfPage *ref   `json:"primaryImageOfPage,omitempty"`
	Breadcrumb         *ref   `json:"breadcrumb,omitempty"`
}

type blogPostingNode struct {
	Type             string `json:"@type"`
	ID               string `json:"@id"`
	Headline         string `json:"headline,omitempty"`
	Description      string `json:"description,omitempty"`
	Image            []*ref `json:"image,omitempty"`
	DatePublished    string `json:"datePublished,omitempty"`
	DateModified     string `json:"dateModified,omitempty"`
	Author           any    `json:"author,omitempty"`
	Publisher        *ref   `json:"publisher,omitempty"`
	MainEntityOfPage *ref   `json:"mainEntityOfPage,omitempty"`
}

type breadcrumbNode struct {
	Type            string         `json:"@type"`
	ID              string         `json:"@id"`
	ItemListElement []listItemNode `json:"itemListElement"`
}

type listItemNode struct {
	Type     string `json:"@type"`
	Position int    `json:"position"`
	Name     string `json:"name,omitempty"`
	Item     string `json:"item,omitempty"`
}

// --- Inputs ----------------------------------------------------------------

// Site carries the global, site-level inputs for the graph.
type Site struct {
	Title      string
	URL        string
	Origin     string   // request origin, used only when URL is empty
	Language   string   // BCP 47 tag, e.g. "en" or "pl-PL"
	Represents string   // RepresentsOrganization or RepresentsPerson
	LogoURL    string   // absolute URL to the Organization logo
	SocialURLs []string // absolute social profile URLs feeding sameAs
}

// Image describes the featured/social image of a document.
type Image struct {
	URL         string
	Width       int
	Height      int
	Caption     string
	Description string
}

// Author is the slot for a future PUBLIC author profile. Nothing here may be
// sourced from private admin user data; with only a display name available the
// builder emits a bare inline Person instead of a full profile node.
type Author struct {
	DisplayName string
	URL         string
	AvatarURL   string
	Bio         string
	SameAs      []string
}

// Page describes one rendered document (a published Entry).
type Page struct {
	Path          string // route path, e.g. "/about"
	ContentTypeID string // "page" or "post"
	Name          string
	Description   string
	CanonicalURL  string
	Mode          Mode
	Image         *Image
	Author        *Author
	PublishedUnix int64 // FIRST publication of the entry; 0 = unknown
	ModifiedUnix  int64 // publication of the current published revision; 0 = unknown
	Timezone      string
}

// Build encodes the complete JSON-LD document for one page. It returns an
// empty string when nothing should be emitted (no absolute base URL known, or
// the page-level data is disabled); the global WebSite/publisher entities are
// still emitted whenever a base URL exists.
func Build(siteInput Site, pageInput Page) (string, error) {
	base := baseURL(siteInput)
	if base == "" {
		return "", nil
	}

	publisher := publisherRef(base, siteInput)
	graph := []any{
		publisher.node,
		webSiteNode{
			Type:       "WebSite",
			ID:         base + "/#website",
			Name:       siteInput.Title,
			URL:        base,
			InLanguage: siteInput.Language,
			Publisher:  &ref{ID: publisher.id},
		},
	}

	if pageInput.Mode == ModeDisabled {
		return encode(document{Context: schemaContext, Graph: graph})
	}

	webpageID := base + fragment(pageInput.Path, "webpage")
	primaryImageID := base + fragment(pageInput.Path, "primaryimage")

	var primary *ref
	if img := imageFor(primaryImageID, pageInput.Image); img != nil {
		graph = append(graph, *img)
		primary = &ref{ID: primaryImageID}
	}

	var breadcrumb *ref
	if crumb := breadcrumbFor(base, siteInput, pageInput); crumb != nil {
		graph = append(graph, *crumb)
		breadcrumb = &ref{ID: base + fragment(pageInput.Path, "breadcrumb")}
	}

	graph = append(graph, webPageNode{
		Type:               pageTypeName(pageInput.Mode),
		ID:                 webpageID,
		URL:                canonicalURL(base, pageInput),
		Name:               pageInput.Name,
		Description:        pageInput.Description,
		InLanguage:         siteInput.Language,
		IsPartOf:           &ref{ID: base + "/#website"},
		PrimaryImageOfPage: primary,
		Breadcrumb:         breadcrumb,
	})

	if isPost(pageInput.ContentTypeID) {
		article := blogPostingNode{
			Type:             "BlogPosting",
			ID:               base + fragment(pageInput.Path, "article"),
			Headline:         pageInput.Name,
			Description:      pageInput.Description,
			DatePublished:    formatDate(pageInput.PublishedUnix, pageInput.Timezone),
			DateModified:     formatDate(pageInput.ModifiedUnix, pageInput.Timezone),
			Publisher:        &ref{ID: publisher.id},
			MainEntityOfPage: &ref{ID: webpageID},
		}
		if primary != nil {
			article.Image = []*ref{primary}
		}
		if author, profile := authorFor(base, pageInput.Author); profile != nil {
			graph = append(graph, *profile)
			article.Author = &ref{ID: profile.ID}
		} else if author != nil {
			article.Author = author
		}
		graph = append(graph, article)
	}

	return encode(document{Context: schemaContext, Graph: graph})
}

// --- Helpers ---------------------------------------------------------------

type publisherResult struct {
	id   string
	node any
}

func publisherRef(base string, siteInput Site) publisherResult {
	sameAs := nonEmpty(siteInput.SocialURLs)
	if strings.TrimSpace(siteInput.Represents) == RepresentsPerson {
		return publisherResult{
			id: base + "/#person",
			node: personNode{
				Type:   "Person",
				ID:     base + "/#person",
				Name:   siteInput.Title,
				URL:    base,
				SameAs: sameAs,
			},
		}
	}
	var logo *imageNode
	if url := strings.TrimSpace(siteInput.LogoURL); url != "" {
		logo = &imageNode{Type: "ImageObject", ID: base + "/#logo", URL: url}
	}
	return publisherResult{
		id: base + "/#organization",
		node: organizationNode{
			Type:   "Organization",
			ID:     base + "/#organization",
			Name:   siteInput.Title,
			URL:    base,
			Logo:   logo,
			SameAs: sameAs,
		},
	}
}

// authorFor returns the inline value for BlogPosting.author plus, when the
// author has enough public profile information to warrant a full node, that
// node (which the caller adds to the graph).
func authorFor(base string, a *Author) (inline any, profile *personNode) {
	if a == nil || strings.TrimSpace(a.DisplayName) == "" {
		return nil, nil
	}
	hasProfile := a.URL != "" || a.AvatarURL != "" || a.Bio != "" || len(nonEmpty(a.SameAs)) > 0
	if !hasProfile {
		// Public display name only: never invent or leak admin user details.
		return personNode{Type: "Person", Name: a.DisplayName}, nil
	}
	id := a.URL
	if !strings.HasPrefix(id, "http://") && !strings.HasPrefix(id, "https://") {
		id = base + "/#author"
	}
	return nil, &personNode{
		Type:   "Person",
		ID:     id,
		Name:   a.DisplayName,
		URL:    a.URL,
		Image:  a.AvatarURL,
		Bio:    a.Bio,
		SameAs: nonEmpty(a.SameAs),
	}
}

func imageFor(id string, in *Image) *imageNode {
	if in == nil || strings.TrimSpace(in.URL) == "" {
		return nil
	}
	return &imageNode{
		Type:        "ImageObject",
		ID:          id,
		URL:         in.URL,
		Width:       in.Width,
		Height:      in.Height,
		Caption:     in.Caption,
		Description: in.Description,
	}
}

func breadcrumbFor(base string, siteInput Site, pageInput Page) *breadcrumbNode {
	if pageInput.Path == "/" || pageInput.Path == "" || pageInput.CanonicalURL == "" {
		return nil
	}
	name := pageInput.Name
	if name == "" {
		name = pageInput.CanonicalURL
	}
	return &breadcrumbNode{
		Type: "BreadcrumbList",
		ID:   base + fragment(pageInput.Path, "breadcrumb"),
		ItemListElement: []listItemNode{
			{Type: "ListItem", Position: 1, Name: homeName(siteInput), Item: base + "/"},
			{Type: "ListItem", Position: 2, Name: name, Item: pageInput.CanonicalURL},
		},
	}
}

func homeName(siteInput Site) string {
	if title := strings.TrimSpace(siteInput.Title); title != "" {
		return title
	}
	return "Home"
}

func pageTypeName(mode Mode) string {
	switch mode {
	case ModeAboutPage:
		return "AboutPage"
	case ModeContactPage:
		return "ContactPage"
	case ModeWebPage:
		return "WebPage"
	default:
		return "WebPage"
	}
}

func isPost(contentTypeID string) bool {
	return strings.EqualFold(strings.TrimSpace(contentTypeID), "post")
}

func canonicalURL(base string, pageInput Page) string {
	// Routed through the central canonical builder so JSON-LD URLs share the
	// exact normalization policy (site URL preference, trailing-slash
	// handling, root path, override resolution) as <link rel=canonical> and OG.
	return seo.Canonical(base, "", pageInput.Path, pageInput.CanonicalURL)
}

func fragment(path, name string) string {
	return strings.TrimRight(path, "/") + "/#" + name
}

func baseURL(siteInput Site) string {
	// The base origin is resolved by the central SEO URL builder so JSON-LD,
	// canonical links and social tags always agree (site URL first, request
	// origin as development fallback).
	return seo.BaseURL(siteInput.URL, siteInput.Origin)
}

func formatDate(unix int64, timezone string) string {
	if unix <= 0 {
		return ""
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	return time.Unix(unix, 0).In(loc).Format(time.RFC3339)
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func encode(doc document) (string, error) {
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
