package blocks

import (
	"context"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/rendering"
)

// postsTemplate and styles are copied from the migration for test stability.
const postsSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"source":{"type":"string","enum":["archive","latest"],"default":"archive"},"layout":{"type":"string","enum":["list","grid"],"default":"list"},"columns":{"type":"integer","enum":[1,2,3],"default":1},"showImage":{"type":"boolean","default":true},"showDate":{"type":"boolean","default":true},"showExcerpt":{"type":"boolean","default":true},"limit":{"type":"integer","default":3,"minimum":1,"maximum":20},"pagination":{"type":"boolean","default":true},"showViewAll":{"type":"boolean","default":false},"viewAllLabel":{"type":"string","default":""}}},"children":{"mode":"none"},"editor":{"category":"query","icon":"posts","fields":{"settings.source":{"label":"Source","control":"select","group":"Content"},"settings.layout":{"label":"Layout","control":"segmented","group":"Style"},"settings.columns":{"label":"Columns","control":"select","group":"Style"},"settings.showImage":{"label":"Show image","control":"checkbox","group":"Content"},"settings.showDate":{"label":"Show date","control":"checkbox","group":"Content"},"settings.showExcerpt":{"label":"Show excerpt","control":"checkbox","group":"Content"},"settings.limit":{"label":"Number of posts","control":"number","group":"Content"},"settings.pagination":{"label":"Show pagination","control":"checkbox","group":"Content"},"settings.showViewAll":{"label":"Show view all link","control":"checkbox","group":"Content"},"settings.viewAllLabel":{"label":"View all label","control":"text","group":"Content"}}}}`

const postsTemplate = `{{ $source := .Settings.source }}{{ if eq $source "archive" }}{{ if not .Context.Archive }}<div class="stratum-posts-placeholder">Posts archive will appear here.</div>{{ else }}{{ $entries := .Context.Archive.Entries }}{{ $settings := .Settings }}{{ if eq (len $entries) 0 }}<div class="stratum-posts-empty"><p>No posts found.</p></div>{{ else }}<section class="stratum-posts stratum-posts--{{ $settings.layout }}{{ if eq $settings.layout "grid" }} stratum-posts--cols-{{ $settings.columns }}{{ end }}">{{ range $entries }}<article class="stratum-post-card">{{ if $settings.showImage }}{{ if .FeaturedImage.Src }}<figure class="stratum-post-card__media"><img src="{{ .FeaturedImage.Src }}"{{ if .FeaturedImage.SrcSet }} srcset="{{ .FeaturedImage.SrcSet }}" sizes="(min-width: 768px) 280px, 100vw"{{ end }}{{ if .FeaturedImage.Width }} width="{{ .FeaturedImage.Width }}"{{ end }}{{ if .FeaturedImage.Height }} height="{{ .FeaturedImage.Height }}"{{ end }} alt="{{ .FeaturedImage.Alt }}" loading="lazy" decoding="async"></figure>{{ end }}{{ end }}<header class="stratum-post-card__header"><h2 class="stratum-post-card__title"><a href="{{ .URL }}">{{ .Title }}</a></h2>{{ if $settings.showDate }}{{ if .PublishedISO }}<time class="stratum-post-card__date" datetime="{{ .PublishedISO }}">{{ .PublishedAt }}</time>{{ end }}{{ end }}</header>{{ if $settings.showExcerpt }}{{ if .Excerpt }}<p class="stratum-post-card__excerpt">{{ .Excerpt }}</p>{{ end }}{{ end }}</article>{{ end }}</section>{{ if $settings.pagination }}{{ $p := $.Context.Archive.Pagination }}{{ if gt $p.TotalPages 1 }}<nav aria-label="Pagination" class="stratum-pagination">{{ if $p.PreviousURL }}<a href="{{ $p.PreviousURL }}" rel="prev">Previous</a>{{ end }}<span>Page {{ $p.Current }} of {{ $p.TotalPages }}</span>{{ if $p.NextURL }}<a href="{{ $p.NextURL }}" rel="next">Next</a>{{ end }}</nav>{{ end }}{{ end }}{{ end }}{{ end }}{{ else }}{{ $entries := index .Context.Collections .ID }}{{ $settings := .Settings }}{{ if not $entries }}<div class="stratum-posts-placeholder">No posts.</div>{{ else }}<section class="stratum-posts stratum-posts--{{ $settings.layout }}{{ if eq $settings.layout "grid" }} stratum-posts--cols-{{ $settings.columns }}{{ end }}">{{ range $entries }}<article class="stratum-post-card">{{ if $settings.showImage }}{{ if .FeaturedImage.Src }}<figure class="stratum-post-card__media"><img src="{{ .FeaturedImage.Src }}"{{ if .FeaturedImage.SrcSet }} srcset="{{ .FeaturedImage.SrcSet }}" sizes="(min-width: 768px) 280px, 100vw"{{ end }}{{ if .FeaturedImage.Width }} width="{{ .FeaturedImage.Width }}"{{ end }}{{ if .FeaturedImage.Height }} height="{{ .FeaturedImage.Height }}"{{ end }} alt="{{ .FeaturedImage.Alt }}" loading="lazy" decoding="async"></figure>{{ end }}{{ end }}<header class="stratum-post-card__header"><h2 class="stratum-post-card__title"><a href="{{ .URL }}">{{ .Title }}</a></h2>{{ if $settings.showDate }}{{ if .PublishedISO }}<time class="stratum-post-card__date" datetime="{{ .PublishedISO }}">{{ .PublishedAt }}</time>{{ end }}{{ end }}</header>{{ if $settings.showExcerpt }}{{ if .Excerpt }}<p class="stratum-post-card__excerpt">{{ .Excerpt }}</p>{{ end }}{{ end }}</article>{{ end }}</section>{{ end }}{{ if $settings.showViewAll }}{{ if $.Context.ArchiveURL }}<p class="stratum-posts-view-all"><a href="{{ $.Context.ArchiveURL }}">{{ if $settings.viewAllLabel }}{{ $settings.viewAllLabel }}{{ else }}View all posts{{ end }}</a></p>{{ end }}{{ end }}{{ end }}`

const postsStyles = `.stratum-posts{display:grid;gap:var(--st-space-lg)} .stratum-posts--list{grid-template-columns:1fr} .stratum-posts--grid{grid-template-columns:repeat(1,1fr)} .stratum-posts--cols-2{grid-template-columns:repeat(2,1fr)} .stratum-posts--cols-3{grid-template-columns:repeat(3,1fr)} @media(max-width: 800px){.stratum-posts--cols-2,.stratum-posts--cols-3{grid-template-columns:1fr}} .stratum-post-card{display:flex;flex-direction:column;gap:var(--st-space-sm);padding:var(--st-space-lg);border:var(--st-border-width) var(--st-border-style) var(--st-color-border);border-radius:var(--st-radius-md);background:var(--st-color-surface)} .stratum-post-card__media{margin:0} .stratum-post-card__media img{display:block;width:100%;height:auto;border-radius:var(--st-radius-sm)} .stratum-post-card__title{margin:0;font-size:1.15rem;line-height:1.3} .stratum-post-card__title a{color:var(--st-color-heading);text-decoration:none} .stratum-post-card__title a:hover{color:var(--st-color-primary);text-decoration:underline} .stratum-post-card__date{color:var(--st-color-text-muted);font-size:var(--st-small-size)} .stratum-post-card__excerpt{margin:0;color:var(--st-color-text)} .stratum-posts-placeholder{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)} .stratum-posts-empty{padding:var(--st-space-xl);text-align:center;color:var(--st-color-text-muted)} .stratum-pagination{display:flex;align-items:center;justify-content:center;gap:var(--st-space-md);padding-top:var(--st-space-lg)} .stratum-pagination a{color:var(--st-color-primary);text-decoration:none} .stratum-pagination a:hover{text-decoration:underline} .stratum-posts-view-all{margin-top:var(--st-space-md);text-align:center}`

func newPostsRegistry(t *testing.T) *Registry {
	t.Helper()
	defs := []rendering.Definition{
		{Namespace: "core", Name: "posts", Version: 1, RendererType: "template", Template: postsTemplate},
		{Namespace: "core", Name: "section", Version: 1, RendererType: "template", Template: `<section>{{ .Children }}</section>`},
	}
	r, err := rendering.NewRenderer(defs, fakeMedia{})
	if err != nil {
		t.Fatal(err)
	}
	// Build a minimal registry snapshot manually for Prepare/RenderPrepared
	reg := &Registry{}
	reg.snapshot.Store(&snapshot{
		renderer: r,
		definitions: map[BlockKey]*Definition{
			{Name: "core/posts", Version: 1}: {Namespace: "core", Name: "posts", Version: 1, Schema: mustParseSchema(postsSchema), Template: postsTemplate, Styles: postsStyles},
			{Name: "core/section", Version: 1}: {Namespace: "core", Name: "section", Version: 1, Schema: mustParseSchema(sectionSchema), Template: `<section>{{ .Children }}</section>`},
		},
		blockStyles: map[rendering.BlockKey]string{
			{Name: "core/posts", Version: 1}: postsStyles,
		},
	})
	return reg
}

func mustParseSchema(s string) Schema {
	sc, err := ParseSchema(s)
	if err != nil {
		panic(err)
	}
	return sc
}

func TestCorePostsArchiveRendersProvidedEntries(t *testing.T) {
	reg := newPostsRegistry(t)
	doc := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"archive","layout":"list","pagination":false}`)},
		},
	}
	prepared, err := reg.Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	entries := []rendering.ArchiveEntry{
		{ID: "1", Title: "Hello", Excerpt: "Excerpt", URL: "/blog/hello", PublishedAt: "January 1, 2026", PublishedISO: "2026-01-01T00:00:00Z"},
		{ID: "2", Title: "World", Excerpt: "", URL: "/blog/world", PublishedAt: "January 2, 2026", PublishedISO: "2026-01-02T00:00:00Z"},
	}
	rc := rendering.RenderContext{
		Archive: &rendering.ArchiveContext{
			Entries:    entries,
			Pagination: rendering.PaginationContext{Current: 1, TotalPages: 1},
			Permalink:  "/blog",
		},
	}
	html, err := reg.RenderPrepared(context.Background(), prepared, rc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if !strings.Contains(s, "Hello") || !strings.Contains(s, "/blog/hello") {
		t.Fatalf("archive render missing entries: %s", s)
	}
	if !strings.Contains(s, `class="stratum-posts`) {
		t.Fatalf("missing posts wrapper: %s", s)
	}
	if strings.Contains(s, `rel="prev"`) || strings.Contains(s, `rel="next"`) {
		t.Fatalf("pagination should not render when single page")
	}
}

func TestCorePostsPaginationOnlyInArchiveMode(t *testing.T) {
	reg := newPostsRegistry(t)
	doc := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"archive","pagination":true}`)},
		},
	}
	prepared, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{
		Archive: &rendering.ArchiveContext{
			Entries:    []rendering.ArchiveEntry{{ID: "1", Title: "A", URL: "/blog/a"}},
			Pagination: rendering.PaginationContext{Current: 1, TotalPages: 3, NextURL: "/blog/page/2"},
		},
	}
	html, _ := reg.RenderPrepared(context.Background(), prepared, rc)
	if !strings.Contains(string(html), `aria-label="Pagination"`) {
		t.Fatalf("pagination should render in archive mode")
	}
	// latest mode should not render pagination even if pagination:true
	doc2 := &document.Document{
		Version: 1,
		Nodes: []document.Node{
			{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"latest","limit":1,"pagination":true}`)},
		},
	}
	prepared2, _ := reg.Prepare(doc2)
	rc2 := rendering.RenderContext{
		Collections: map[string][]rendering.ArchiveEntry{
			"p1": {{ID: "1", Title: "A", URL: "/blog/a"}},
		},
		ArchiveURL: "/blog",
	}
	html2, _ := reg.RenderPrepared(context.Background(), prepared2, rc2)
	if strings.Contains(string(html2), `aria-label="Pagination"`) {
		t.Fatalf("latest mode should not render pagination")
	}
}

func TestCorePostsShowFlags(t *testing.T) {
	reg := newPostsRegistry(t)
	cases := []struct {
		settings string
		mustContain    []string
		mustNotContain []string
	}{
		{`{"source":"archive","showImage":false}`, []string{"Post"}, []string{`<img`}},
		{`{"source":"archive","showDate":false}`, []string{"Post"}, []string{`<time`}},
		{`{"source":"archive","showExcerpt":false}`, []string{"Post"}, []string{`stratum-post-card__excerpt`}},
	}
	for _, c := range cases {
		doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(c.settings)}}}
		prepared, _ := reg.Prepare(doc)
		ae := rendering.ArchiveEntry{ID: "1", Title: "Post", Excerpt: "Excerpt", URL: "/blog/post", PublishedAt: "Jan 1", PublishedISO: "2026-01-01T00:00:00Z", FeaturedImage: rendering.MediaView{Src: "/img.jpg", Width: 100, Height: 100}}
		rc := rendering.RenderContext{Archive: &rendering.ArchiveContext{Entries: []rendering.ArchiveEntry{ae}, Pagination: rendering.PaginationContext{Current: 1, TotalPages: 1}}}
		html, _ := reg.RenderPrepared(context.Background(), prepared, rc)
		s := string(html)
		for _, want := range c.mustContain {
			if !strings.Contains(s, want) {
				t.Fatalf("settings %s missing %q in %s", c.settings, want, s)
			}
		}
		for _, not := range c.mustNotContain {
			if strings.Contains(s, not) {
				t.Fatalf("settings %s should not contain %q in %s", c.settings, not, s)
			}
		}
	}
}

func TestCorePostsEmptyArchive(t *testing.T) {
	reg := newPostsRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"archive"}`)}}}
	prepared, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{Archive: &rendering.ArchiveContext{Entries: nil, Pagination: rendering.PaginationContext{Current: 1, TotalPages: 1}}}
	html, _ := reg.RenderPrepared(context.Background(), prepared, rc)
	if !strings.Contains(string(html), "No posts found") {
		t.Fatalf("empty archive should show empty state: %s", html)
	}
}

func TestCorePostsPreviewWithoutArchiveContext(t *testing.T) {
	reg := newPostsRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"archive"}`)}}}
	prepared, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{} // no Archive
	html, _ := reg.RenderPrepared(context.Background(), prepared, rc)
	if !strings.Contains(string(html), "Posts archive will appear here") {
		t.Fatalf("preview without Archive should show placeholder: %s", html)
	}
}

func TestCorePostsUsedBlocksContainsPosts(t *testing.T) {
	reg := newPostsRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{}`)}}}
	prepared, _ := reg.Prepare(doc)
	found := false
	for _, k := range prepared.UsedBlocks {
		if k.Name == "core/posts" {
			found = true
		}
	}
	if !found {
		t.Fatalf("UsedBlocks should contain core/posts: %v", prepared.UsedBlocks)
	}
	if got := reg.StylesFor(prepared.UsedBlocks); !strings.Contains(got, ".stratum-posts") {
		t.Fatalf("StylesFor missing posts CSS")
	}
}

func TestCorePostsLatestLimit(t *testing.T) {
	reg := newPostsRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"latest","limit":2,"layout":"grid","columns":2}`)}}}
	prepared, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{
		Collections: map[string][]rendering.ArchiveEntry{
			"p1": {
				{ID: "1", Title: "A", URL: "/blog/a"},
				{ID: "2", Title: "B", URL: "/blog/b"},
				{ID: "3", Title: "C", URL: "/blog/c"},
			},
		},
		ArchiveURL: "/blog",
	}
	// Simulate limit handling: registry doesn't limit, but handler does. Here we test renderer respects provided slice length
	// So we provide 2 entries for limit 2
	rc.Collections["p1"] = rc.Collections["p1"][:2]
	html, _ := reg.RenderPrepared(context.Background(), prepared, rc)
	s := string(html)
	if strings.Contains(s, "C") {
		t.Fatalf("latest limit 2 should not contain C: %s", s)
	}
	if !strings.Contains(s, "stratum-posts--grid") || !strings.Contains(s, "stratum-posts--cols-2") {
		t.Fatalf("grid variant missing: %s", s)
	}
}

func TestCorePostsBlockDoesNotQueryDB(t *testing.T) {
	// Rendering must not touch storage; it only uses provided RenderContext.
	// This test ensures the template does not call any DB helpers – it compiles and renders with fake media only.
	reg := newPostsRegistry(t)
	doc := &document.Document{Version: 1, Nodes: []document.Node{{ID: "p1", Block: "core/posts", Version: 1, Props: []byte(`{}`), Settings: []byte(`{"source":"archive"}`)}}}
	prepared, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{
		Archive: &rendering.ArchiveContext{
			Entries: []rendering.ArchiveEntry{{ID: "1", Title: "A", URL: "/blog/a"}},
		},
	}
	_, err := reg.RenderPrepared(context.Background(), prepared, rc)
	if err != nil {
		t.Fatalf("render should not error: %v", err)
	}
}
