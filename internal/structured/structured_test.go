package structured

import (
	"encoding/json"
	"strings"
	"testing"
)

var testSite = Site{
	Title:      "Example Site",
	URL:        "https://example.com",
	Language:   "en",
	Represents: RepresentsOrganization,
	SocialURLs: []string{"https://x.com/example", " ", "https://github.com/example"},
}

func mustBuild(t *testing.T, siteInput Site, pageInput Page) map[string]any {
	t.Helper()
	payload, err := Build(siteInput, pageInput)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, payload)
	}
	if doc["@context"] != schemaContext {
		t.Fatalf("@context = %v, want %s", doc["@context"], schemaContext)
	}
	graph, ok := doc["@graph"].([]any)
	if !ok || len(graph) == 0 {
		t.Fatalf("missing non-empty @graph:\n%s", payload)
	}
	return doc
}

// node returns the graph node with the given @id.
func node(t *testing.T, doc map[string]any, id string) map[string]any {
	t.Helper()
	graph := doc["@graph"].([]any)
	for _, raw := range graph {
		n, ok := raw.(map[string]any)
		if ok && n["@id"] == id {
			return n
		}
	}
	t.Fatalf("no node with @id %q in graph:\n%v", id, doc)
	return nil
}

func TestBuildWebsite(t *testing.T) {
	doc := mustBuild(t, testSite, Page{Path: "/about"})
	siteNode := node(t, doc, "https://example.com/#website")
	if siteNode["@type"] != "WebSite" {
		t.Fatalf("@type = %v", siteNode["@type"])
	}
	if siteNode["name"] != "Example Site" || siteNode["url"] != "https://example.com" {
		t.Fatalf("website name/url = %v/%v", siteNode["name"], siteNode["url"])
	}
	if siteNode["inLanguage"] != "en" {
		t.Fatalf("inLanguage = %v", siteNode["inLanguage"])
	}
	publisher, ok := siteNode["publisher"].(map[string]any)
	if !ok || publisher["@id"] != "https://example.com/#organization" {
		t.Fatalf("publisher ref = %v", siteNode["publisher"])
	}
}

func TestBuildOrganizationDefault(t *testing.T) {
	logoSite := testSite
	logoSite.LogoURL = "https://example.com/media/logo/512"
	doc := mustBuild(t, logoSite, Page{Path: "/about"})
	org := node(t, doc, "https://example.com/#organization")
	if org["@type"] != "Organization" {
		t.Fatalf("@type = %v", org["@type"])
	}
	if org["name"] != "Example Site" || org["url"] != "https://example.com" {
		t.Fatalf("org name/url = %v/%v", org["name"], org["url"])
	}
	sameAs, ok := org["sameAs"].([]any)
	if !ok || len(sameAs) != 2 || sameAs[0] != "https://x.com/example" {
		t.Fatalf("sameAs = %v, want the two trimmed profile URLs", org["sameAs"])
	}
	logo, ok := org["logo"].(map[string]any)
	if !ok || logo["url"] != "https://example.com/media/logo/512" || logo["@id"] != "https://example.com/#logo" {
		t.Fatalf("logo = %v", org["logo"])
	}
}

func TestBuildPersonPublisher(t *testing.T) {
	personSite := testSite
	personSite.Represents = RepresentsPerson
	personSite.LogoURL = "https://example.com/media/logo/512"
	doc := mustBuild(t, personSite, Page{Path: "/about"})
	person := node(t, doc, "https://example.com/#person")
	if person["@type"] != "Person" || person["name"] != "Example Site" {
		t.Fatalf("person publisher = %v", person)
	}
	for _, raw := range doc["@graph"].([]any) {
		if n, ok := raw.(map[string]any); ok && n["@type"] == "Organization" {
			t.Fatalf("organization node must be absent when site represents a person")
		}
	}
}

func TestBuildWebPage(t *testing.T) {
	doc := mustBuild(t, testSite, Page{
		Path:          "/about",
		ContentTypeID: "page",
		Name:          "About Us",
		Description:   "Learn about us.",
		CanonicalURL:  "https://example.com/about",
	})
	page := node(t, doc, "https://example.com/about/#webpage")
	if page["@type"] != "WebPage" {
		t.Fatalf("@type = %v", page["@type"])
	}
	if page["url"] != "https://example.com/about" || page["name"] != "About Us" || page["description"] != "Learn about us." {
		t.Fatalf("webpage fields = %v", page)
	}
	if page["inLanguage"] != "en" {
		t.Fatalf("inLanguage = %v", page["inLanguage"])
	}
	isPartOf, ok := page["isPartOf"].(map[string]any)
	if !ok || isPartOf["@id"] != "https://example.com/#website" {
		t.Fatalf("isPartOf = %v", page["isPartOf"])
	}
	if _, hasImage := page["primaryImageOfPage"]; hasImage {
		t.Fatalf("primaryImageOfPage must be absent without an image: %v", page)
	}
	breadcrumb, ok := page["breadcrumb"].(map[string]any)
	if !ok || breadcrumb["@id"] != "https://example.com/about/#breadcrumb" {
		t.Fatalf("breadcrumb ref = %v", page["breadcrumb"])
	}
	crumb := node(t, doc, "https://example.com/about/#breadcrumb")
	items, ok := crumb["itemListElement"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("breadcrumb items = %v", crumb["itemListElement"])
	}
	first := items[0].(map[string]any)
	if first["item"] != "https://example.com/" || first["position"] != float64(1) {
		t.Fatalf("first breadcrumb item = %v", first)
	}
}

func TestBuildHomepageHasNoBreadcrumb(t *testing.T) {
	doc := mustBuild(t, testSite, Page{Path: "/", ContentTypeID: "page", Name: "Home", CanonicalURL: "https://example.com/"})
	page := node(t, doc, "https://example.com/#webpage")
	if _, has := page["breadcrumb"]; has {
		t.Fatalf("homepage must not carry a breadcrumb: %v", page)
	}
}

func TestBuildBlogPosting(t *testing.T) {
	doc := mustBuild(t, testSite, Page{
		Path:          "/blog/hello",
		ContentTypeID: "post",
		Name:          "Hello World",
		Description:   "First post.",
		CanonicalURL:  "https://example.com/blog/hello",
		PublishedUnix: 1700000000,
		ModifiedUnix:  1700100000,
		Timezone:      "UTC",
		Image:         &Image{URL: "https://example.com/media/abc/social", Width: 1200, Height: 630},
	})
	article := node(t, doc, "https://example.com/blog/hello/#article")
	if article["@type"] != "BlogPosting" {
		t.Fatalf("@type = %v", article["@type"])
	}
	if article["headline"] != "Hello World" || article["description"] != "First post." {
		t.Fatalf("headline/description = %v/%v", article["headline"], article["description"])
	}
	if article["datePublished"] != "2023-11-14T22:13:20Z" {
		t.Fatalf("datePublished = %v", article["datePublished"])
	}
	if article["dateModified"] != "2023-11-16T02:00:00Z" {
		t.Fatalf("dateModified = %v", article["dateModified"])
	}
	mainEntity, ok := article["mainEntityOfPage"].(map[string]any)
	if !ok || mainEntity["@id"] != "https://example.com/blog/hello/#webpage" {
		t.Fatalf("mainEntityOfPage = %v", article["mainEntityOfPage"])
	}
	publisher, ok := article["publisher"].(map[string]any)
	if !ok || publisher["@id"] != "https://example.com/#organization" {
		t.Fatalf("publisher = %v", article["publisher"])
	}
	images, ok := article["image"].([]any)
	if !ok || len(images) != 1 || images[0].(map[string]any)["@id"] != "https://example.com/blog/hello/#primaryimage" {
		t.Fatalf("image = %v", article["image"])
	}
	img := node(t, doc, "https://example.com/blog/hello/#primaryimage")
	if img["@type"] != "ImageObject" || img["url"] != "https://example.com/media/abc/social" {
		t.Fatalf("image object = %v", img)
	}
	if img["width"] != float64(1200) || img["height"] != float64(630) {
		t.Fatalf("image dimensions = %v/%v", img["width"], img["height"])
	}
	if _, has := article["author"]; has {
		t.Fatalf("author must be omitted without a public display name: %v", article["author"])
	}
	// Posts also get their WebPage node so refs resolve inside one graph.
	node(t, doc, "https://example.com/blog/hello/#webpage")
}

func TestBuildBlogPostingDatesInTimezone(t *testing.T) {
	doc := mustBuild(t, testSite, Page{
		Path:          "/blog/tz",
		ContentTypeID: "post",
		Name:          "TZ",
		PublishedUnix: 1700000000,
		ModifiedUnix:  1700000000,
		Timezone:      "Europe/Warsaw",
	})
	article := node(t, doc, "https://example.com/blog/tz/#article")
	if article["datePublished"] != "2023-11-14T23:13:20+01:00" {
		t.Fatalf("datePublished = %v, want Warsaw offset", article["datePublished"])
	}
}

func TestBuildAuthorDisplayNameOnlyIsInline(t *testing.T) {
	doc := mustBuild(t, testSite, Page{
		Path:          "/blog/byline",
		ContentTypeID: "post",
		Name:          "Byline",
		Author:        &Author{DisplayName: "Jane Doe"},
	})
	article := node(t, doc, "https://example.com/blog/byline/#article")
	author, ok := article["author"].(map[string]any)
	if !ok || author["@type"] != "Person" || author["name"] != "Jane Doe" {
		t.Fatalf("inline author = %v", article["author"])
	}
	if _, hasID := author["@id"]; hasID {
		t.Fatalf("display-name-only author must not become a graph node: %v", author)
	}
}

func TestBuildAuthorFullProfileBecomesNode(t *testing.T) {
	doc := mustBuild(t, testSite, Page{
		Path:          "/blog/profile",
		ContentTypeID: "post",
		Name:          "Profile",
		Author: &Author{
			DisplayName: "Jane Doe",
			URL:         "https://example.com/authors/jane",
			Bio:         "Writer.",
			SameAs:      []string{"https://x.com/jane"},
		},
	})
	article := node(t, doc, "https://example.com/blog/profile/#article")
	authorRef, ok := article["author"].(map[string]any)
	if !ok || authorRef["@id"] != "https://example.com/authors/jane" {
		t.Fatalf("author ref = %v", article["author"])
	}
	profile := node(t, doc, "https://example.com/authors/jane")
	if profile["name"] != "Jane Doe" || profile["description"] != "Writer." {
		t.Fatalf("author profile = %v", profile)
	}
	if sameAs, ok := profile["sameAs"].([]any); !ok || sameAs[0] != "https://x.com/jane" {
		t.Fatalf("author sameAs = %v", profile["sameAs"])
	}
}

func TestBuildDisabledModeOmitsPageEntities(t *testing.T) {
	doc := mustBuild(t, testSite, Page{Path: "/secret", ContentTypeID: "page", Name: "Secret", Mode: ModeDisabled})
	for _, raw := range doc["@graph"].([]any) {
		n := raw.(map[string]any)
		switch n["@type"] {
		case "WebPage", "BlogPosting", "BreadcrumbList", "ImageObject":
			t.Fatalf("disabled mode leaked a %s node: %v", n["@type"], n)
		}
	}
	// Global entities stay.
	node(t, doc, "https://example.com/#website")
	node(t, doc, "https://example.com/#organization")
}

func TestBuildModeOverridesPageType(t *testing.T) {
	for mode, want := range map[Mode]string{ModeAboutPage: "AboutPage", ModeContactPage: "ContactPage", ModeWebPage: "WebPage"} {
		doc := mustBuild(t, testSite, Page{Path: "/x", Mode: mode})
		if got := node(t, doc, "https://example.com/x/#webpage")["@type"]; got != want {
			t.Fatalf("mode %q @type = %v, want %s", mode, got, want)
		}
	}
}

func TestBuildStableIDsAcrossContentChanges(t *testing.T) {
	before := mustBuild(t, testSite, Page{Path: "/post", ContentTypeID: "post", Name: "Old", PublishedUnix: 100, ModifiedUnix: 100})
	after := mustBuild(t, testSite, Page{Path: "/post", ContentTypeID: "post", Name: "New Title", ModifiedUnix: 999})
	for _, id := range []string{"https://example.com/#website", "https://example.com/post/#webpage", "https://example.com/post/#article"} {
		node(t, before, id)
		node(t, after, id)
	}
}

func TestBuildWithoutBaseURLEmitsNothing(t *testing.T) {
	payload, err := Build(Site{}, Page{Path: "/about"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.TrimSpace(payload) != "" {
		t.Fatalf("expected empty payload without a base URL, got %q", payload)
	}
}

func TestBuildUsesOriginFallback(t *testing.T) {
	doc := mustBuild(t, Site{Title: "S", Origin: "https://fallback.test"}, Page{Path: "/p", ContentTypeID: "page"})
	node(t, doc, "https://fallback.test/#website")
	node(t, doc, "https://fallback.test/p/#webpage")
}
