package seo

import "testing"

func TestResolve_TitleSeparator(t *testing.T) {
	r := New()
	in := Input{
		Site:     SiteSEO{Title: "StratumCMS", TitleSeparator: "—", IndexingEnabled: true},
		Revision: RevisionSEO{Title: "Hello", SeoTitle: "Custom Title"},
		Path:     "/hello",
	}
	res := r.Resolve(in)
	want := "Custom Title — StratumCMS"
	if res.Title != want {
		t.Fatalf("title = %q, want %q", res.Title, want)
	}
	if res.RawTitle != "Custom Title" {
		t.Fatalf("raw = %q", res.RawTitle)
	}
}

func TestResolve_TitleFallbackToTitle(t *testing.T) {
	r := New()
	in := Input{
		Site:     SiteSEO{Title: "Site", TitleSeparator: "|", IndexingEnabled: true},
		Revision: RevisionSEO{Title: "Page Title"},
		Path:     "/p",
	}
	res := r.Resolve(in)
	want := "Page Title | Site"
	if res.Title != want {
		t.Fatalf("title = %q want %q", res.Title, want)
	}
}

func TestResolve_DescriptionFallback(t *testing.T) {
	r := New()
	cases := []struct {
		name string
		rev  RevisionSEO
		want string
	}{
		{"seo wins", RevisionSEO{SeoDescription: "seo", Excerpt: "excerpt"}, "seo"},
		{"excerpt fallback", RevisionSEO{Excerpt: "excerpt"}, "excerpt"},
		{"none", RevisionSEO{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: c.rev, Path: "/x"})
			if res.Description != c.want {
				t.Fatalf("desc = %q want %q", res.Description, c.want)
			}
		})
	}
}

func TestResolve_RobotsInheritance(t *testing.T) {
	r := New()
	// site allows, revision overrides to noindex
	indexFalse := false
	followFalse := false
	indexTrue := true

	// site indexing disabled -> noindex,nofollow
	res := r.Resolve(Input{Site: SiteSEO{IndexingEnabled: false}, Revision: RevisionSEO{}, Path: "/"})
	if res.Robots != "noindex,nofollow" {
		t.Fatalf("site disabled robots = %q want noindex,nofollow", res.Robots)
	}

	// site enabled, revision overrides index to false
	res = r.Resolve(Input{
		Site:     SiteSEO{IndexingEnabled: true},
		Revision: RevisionSEO{RobotsIndex: &indexFalse},
		Path:     "/",
	})
	if res.Robots != "noindex" {
		t.Fatalf("index false -> %q want noindex", res.Robots)
	}

	// site disabled, revision overrides to index true -> allow (follow still false? need both)
	// revision overrides only index, follow still inherits from site (false)
	res = r.Resolve(Input{
		Site:     SiteSEO{IndexingEnabled: false},
		Revision: RevisionSEO{RobotsIndex: &indexTrue},
		Path:     "/",
	})
	if res.Robots != "nofollow" {
		t.Fatalf("index true but follow inherited false -> %q want nofollow", res.Robots)
	}

	// content type overrides then revision overrides
	ctIndexFalse := false
	res = r.Resolve(Input{
		Site:        SiteSEO{IndexingEnabled: true},
		ContentType: &ContentTypeSEO{RobotsIndex: &ctIndexFalse},
		Revision:    RevisionSEO{RobotsIndex: &indexTrue},
		Path:        "/",
	})
	if res.Robots != "max-image-preview:large" {
		t.Fatalf("revision should win over content type: %q want max-image-preview:large", res.Robots)
	}

	// follow override
	res = r.Resolve(Input{
		Site:     SiteSEO{IndexingEnabled: true},
		Revision: RevisionSEO{RobotsFollow: &followFalse},
		Path:     "/",
	})
	if res.Robots != "nofollow" {
		t.Fatalf("follow false -> %q", res.Robots)
	}

	// both false
	f := false
	res = r.Resolve(Input{
		Site:     SiteSEO{IndexingEnabled: true},
		Revision: RevisionSEO{RobotsIndex: &f, RobotsFollow: &f},
		Path:     "/",
	})
	if res.Robots != "noindex,nofollow" {
		t.Fatalf("both false -> %q", res.Robots)
	}
}

func TestResolve_Canonical(t *testing.T) {
	r := New()
	// site URL + path
	res := r.Resolve(Input{Site: SiteSEO{SiteURL: "https://example.com", IndexingEnabled: true}, Revision: RevisionSEO{}, Path: "/about", Origin: "https://fallback.com"})
	if res.Canonical != "https://example.com/about" {
		t.Fatalf("canonical = %q", res.Canonical)
	}
	// override absolute
	res = r.Resolve(Input{Site: SiteSEO{SiteURL: "https://example.com", IndexingEnabled: true}, Revision: RevisionSEO{CanonicalURL: "https://other.example/x"}, Path: "/about"})
	if res.Canonical != "https://other.example/x" {
		t.Fatalf("override = %q", res.Canonical)
	}
	// override root-relative
	res = r.Resolve(Input{Site: SiteSEO{SiteURL: "https://example.com", IndexingEnabled: true}, Revision: RevisionSEO{CanonicalURL: "/custom"}, Path: "/about"})
	if res.Canonical != "https://example.com/custom" {
		t.Fatalf("root-relative = %q", res.Canonical)
	}
	// no site URL uses origin
	res = r.Resolve(Input{Site: SiteSEO{SiteURL: "", IndexingEnabled: true}, Revision: RevisionSEO{}, Path: "/about", Origin: "https://origin.example"})
	if res.Canonical != "https://origin.example/about" {
		t.Fatalf("origin fallback = %q", res.Canonical)
	}
}

func TestResolve_OpenGraphImagePrecedence(t *testing.T) {
	r := New()
	res := r.Resolve(Input{
		Site:     SiteSEO{SiteURL: "https://example.com", IndexingEnabled: true},
		Revision: RevisionSEO{FeaturedMediaID: "feat", SocialMediaID: "social"},
		Path:     "/p",
	})
	if res.OpenGraph.Image != "https://example.com/media/social/social" {
		t.Fatalf("og image = %q want https://example.com/media/social/social", res.OpenGraph.Image)
	}
	if res.OGImageID != "social" {
		t.Fatalf("og image id = %q want social", res.OGImageID)
	}
	res = r.Resolve(Input{
		Site:     SiteSEO{SiteURL: "https://example.com", IndexingEnabled: true},
		Revision: RevisionSEO{FeaturedMediaID: "feat"},
		Path:     "/p",
	})
	if res.OpenGraph.Image != "https://example.com/media/feat/social" {
		t.Fatalf("og image = %q want https://example.com/media/feat/social", res.OpenGraph.Image)
	}
	res = r.Resolve(Input{
		Site:     SiteSEO{SiteURL: "https://example.com", GlobalSocialMediaID: "global", IndexingEnabled: true},
		Revision: RevisionSEO{},
		Path:     "/p",
	})
	if res.OpenGraph.Image != "https://example.com/media/global/social" {
		t.Fatalf("og global fallback = %q want https://example.com/media/global/social", res.OpenGraph.Image)
	}
	res = r.Resolve(Input{
		Site:     SiteSEO{IndexingEnabled: true},
		Revision: RevisionSEO{},
		Path:     "/p",
	})
	if res.OpenGraph.Image != "" {
		t.Fatalf("og image should be empty, got %q", res.OpenGraph.Image)
	}
}

func TestResolve_OGTypePageVsPost(t *testing.T) {
	r := New()
	// Page → website
	res := r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, ContentTypeID: "page", Path: "/p"})
	if res.OpenGraph.Type != "website" {
		t.Fatalf("page og type = %q want website", res.OpenGraph.Type)
	}
	// Post → article
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, ContentTypeID: "post", Path: "/p"})
	if res.OpenGraph.Type != "article" {
		t.Fatalf("post og type = %q want article", res.OpenGraph.Type)
	}
	// Unknown defaults to website
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, ContentTypeID: "", Path: "/p"})
	if res.OpenGraph.Type != "website" {
		t.Fatalf("unknown og type = %q want website", res.OpenGraph.Type)
	}
}

func TestResolve_OGTitleDescriptionFallback(t *testing.T) {
	r := New()
	// OG title: seo_title wins
	res := r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: RevisionSEO{Title: "Entry", SeoTitle: "SEO Title"}, Path: "/p"})
	if res.OpenGraph.Title != "SEO Title" {
		t.Fatalf("og title = %q want SEO Title", res.OpenGraph.Title)
	}
	// fallback to entry title
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: RevisionSEO{Title: "Entry"}, Path: "/p"})
	if res.OpenGraph.Title != "Entry" {
		t.Fatalf("og title fallback = %q want Entry", res.OpenGraph.Title)
	}
	// OG description: seo_description wins
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: RevisionSEO{SeoDescription: "SEO Desc", Excerpt: "Excerpt"}, Path: "/p"})
	if res.OpenGraph.Description != "SEO Desc" {
		t.Fatalf("og desc = %q want SEO Desc", res.OpenGraph.Description)
	}
	// fallback to excerpt
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: RevisionSEO{Excerpt: "Excerpt"}, Path: "/p"})
	if res.OpenGraph.Description != "Excerpt" {
		t.Fatalf("og desc fallback = %q want Excerpt", res.OpenGraph.Description)
	}
	// Twitter should mirror OG
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: RevisionSEO{Title: "T", Excerpt: "E"}, Path: "/p"})
	if res.Twitter.Title != res.OpenGraph.Title || res.Twitter.Description != res.OpenGraph.Description || res.Twitter.Image != res.OpenGraph.Image {
		t.Fatalf("twitter should mirror OG: twitter=%+v og=%+v", res.Twitter, res.OpenGraph)
	}
	if res.Twitter.Card != "summary_large_image" {
		t.Fatalf("twitter card = %q want summary_large_image", res.Twitter.Card)
	}
}

func TestResolve_SocialAbsoluteURLs(t *testing.T) {
	r := New()
	// canonical absolute via SiteURL
	res := r.Resolve(Input{Site: SiteSEO{SiteURL: "https://example.com", IndexingEnabled: true}, Revision: RevisionSEO{FeaturedMediaID: "img"}, Path: "/p"})
	if res.OpenGraph.URL != "https://example.com/p" {
		t.Fatalf("og url = %q want https://example.com/p", res.OpenGraph.URL)
	}
	if res.OpenGraph.Image != "https://example.com/media/img/social" {
		t.Fatalf("og image absolute = %q", res.OpenGraph.Image)
	}
	// absolute via Origin when SiteURL empty
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Origin: "https://origin.test", Revision: RevisionSEO{FeaturedMediaID: "img"}, Path: "/p"})
	if res.OpenGraph.URL != "https://origin.test/p" {
		t.Fatalf("og url via origin = %q", res.OpenGraph.URL)
	}
	if res.OpenGraph.Image != "https://origin.test/media/img/social" {
		t.Fatalf("og image via origin = %q", res.OpenGraph.Image)
	}
	// twitter site optional
	res = r.Resolve(Input{Site: SiteSEO{SiteURL: "https://example.com", TwitterSite: "@handle", IndexingEnabled: true}, Revision: RevisionSEO{Title: "T"}, Path: "/p"})
	if res.Twitter.Site != "@handle" {
		t.Fatalf("twitter site = %q want @handle", res.Twitter.Site)
	}
	res = r.Resolve(Input{Site: SiteSEO{IndexingEnabled: true}, Revision: RevisionSEO{Title: "T"}, Path: "/p"})
	if res.Twitter.Site != "" {
		t.Fatalf("twitter site should be empty when not set, got %q", res.Twitter.Site)
	}
}
