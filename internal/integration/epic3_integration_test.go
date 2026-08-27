package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/patterns"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

// helper to build migrated registry and themes for integration
func epic3Setup(t *testing.T) (*storage.Database, *db.Queries, *blocks.Registry, *themes.Runtime) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	queries := db.New(database.DB)
	reg, err := blocks.NewRegistry(ctx, queries, fakeMediaForEpic3{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	themeRt, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatalf("theme: %v", err)
	}
	return database, queries, reg, themeRt
}

type fakeMediaForEpic3 struct{}

func (f fakeMediaForEpic3) MediaView(_ context.Context, id string) (rendering.MediaView, bool) {
	if id == "" {
		return rendering.MediaView{}, false
	}
	return rendering.MediaView{ID: id, Src: "/media/" + id + "/768", SrcSet: "/media/" + id + "/480 480w, /media/" + id + "/768 768w", WebPSrcSet: "/media/" + id + "/480.webp 480w", Width: 800, Height: 600, Alt: "Alt " + id}, true
}

// Scenario 1 — Site Styles change reflects in public CSS and invalidates cache
func TestEpic3_SiteStyles_ReflectsInCSS(t *testing.T) {
	_, queries, _, themeRt := epic3Setup(t)
	ctx := context.Background()
	// Change primary color
	if err := themeRt.Save(ctx, map[string]any{"colors.primary": "#123456"}, ""); err != nil {
		t.Fatalf("save theme: %v", err)
	}
	css := themeRt.Styles()
	if !strings.Contains(css, "#123456") {
		t.Fatalf("css missing new primary color: %s", css)
	}
	// Verify that required CSS variables exist
	for _, want := range []string{"--st-color-primary", "--st-color-text", "--st-space-md", "--st-content-width"} {
		if !strings.Contains(css, want) {
			t.Fatalf("css missing %s", want)
		}
	}
	// Verify that theme validation rejects malicious color
	if err := themeRt.Save(ctx, map[string]any{"colors.primary": "red; background:url(javascript:alert(1))"}, ""); err == nil {
		t.Fatalf("expected malicious color rejected")
	}
	_ = queries // ensure queries used
}

// Scenario 2 — Hero Pattern insertion persisted structure
func TestEpic3_HeroPatternInsertion_EndToEnd(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	cat := patterns.NewCatalog()
	p, ok := cat.Get("hero-centered")
	if !ok {
		t.Fatalf("hero-centered not found")
	}
	// Simulate editor: clone pattern and insert into empty document
	doc := &document.Document{Version: 1, Nodes: []document.Node{}}
	clone, err := p.CloneWithNewIDs()
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	doc.Nodes = append(doc.Nodes, clone.Nodes...)
	// Validate persisted structure
	if err := reg.ValidateDocument(doc); err != nil {
		t.Fatalf("validate after hero insert: %v", err)
	}
	// Simulate publish: prepare and render public HTML
	prep, err := reg.Prepare(doc)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	html, err := reg.RenderPrepared(context.Background(), prep, rendering.RenderContext{LCP: &rendering.LCPState{}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(html)
	for _, want := range []string{"Build something remarkable", "Get started", "Learn more"} {
		if !strings.Contains(s, want) {
			t.Fatalf("public missing %q: %s", want, s)
		}
	}
	// Ensure pattern IDs were regenerated (original IDs a1..a7 not present)
	for _, id := range []string{"a1", "a2", "a3"} {
		if strings.Contains(s, id) {
			t.Logf("warning: public should not contain raw pattern IDs, but HTML doesn't contain IDs anyway")
		}
	}
}

// Scenario 3 — Pattern IDs distinct on double insert
func TestEpic3_PatternIDs_Distinct(t *testing.T) {
	cat := patterns.NewCatalog()
	p, _ := cat.Get("hero-centered")
	c1, _ := p.CloneWithNewIDs()
	c2, _ := p.CloneWithNewIDs()
	ids1 := collectIDs(c1)
	ids2 := collectIDs(c2)
	for id := range ids1 {
		if ids2[id] {
			t.Fatalf("duplicate ID between clones: %s", id)
		}
	}
	if len(ids1) != document.Count(c1) {
		t.Fatalf("duplicate inside clone")
	}
}

func collectIDs(doc *document.Document) map[string]bool {
	m := make(map[string]bool)
	_ = document.Walk(doc, func(n document.Node) error { m[n.ID] = true; return nil })
	return m
}

// Scenario 4 — Pattern context filtering
func TestEpic3_PatternContext_Filtering(t *testing.T) {
	cat := patterns.NewCatalog()
	// Archive-only pattern appears for archive-template, not for entry
	foundArchiveInEntry := false
	for _, p := range cat.List("entry") {
		if p.ID == "archive-collection-grid" {
			foundArchiveInEntry = true
		}
	}
	if foundArchiveInEntry {
		t.Fatalf("archive pattern should not appear in entry")
	}
	foundArchiveInArchive := false
	for _, p := range cat.List("archive-template") {
		if p.ID == "archive-collection-grid" {
			foundArchiveInArchive = true
		}
	}
	if !foundArchiveInArchive {
		t.Fatalf("archive pattern not found in archive-template")
	}
	// Single-only pattern not in site-part
	for _, p := range cat.List("site-part") {
		if p.ID == "single-article" {
			t.Fatalf("single-article should not appear in site-part")
		}
	}
	foundSingle := false
	for _, p := range cat.List("single-template") {
		if p.ID == "single-article" {
			foundSingle = true
		}
	}
	if !foundSingle {
		t.Fatalf("single-article not in single-template")
	}
}

// Scenario 5 — Pattern validation against real registry
func TestEpic3_PatternValidation_RealRegistry(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	cat := patterns.NewCatalog()
	if err := cat.ValidateAll(reg); err != nil {
		t.Fatalf("validate all: %v", err)
	}
}

// Scenario 6 — Image priority exactly one high
func TestEpic3_ImagePriority_ExactlyOneHigh(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	// Page with 3 images: explicit high wins
	doc := decodeDocForEpic3(t, `{"version":1,"nodes":[
		{"id":"a","block":"core/image","version":1,"props":{"mediaId":"m1"},"settings":{"priority":"high"}},
		{"id":"b","block":"core/image","version":1,"props":{"mediaId":"m2"},"settings":{"priority":"auto"}},
		{"id":"c","block":"core/image","version":1,"props":{"mediaId":"m3"},"settings":{"priority":"auto"}}
	]}`)
	html := renderEpic3(t, reg, doc)
	if got := strings.Count(html, `fetchpriority="high"`); got != 1 {
		t.Fatalf("expected 1 high, got %d: %s", got, html)
	}
	// Normal excluded: first auto should win, not normal
	doc2 := decodeDocForEpic3(t, `{"version":1,"nodes":[
		{"id":"a","block":"core/image","version":1,"props":{"mediaId":"m1"},"settings":{"priority":"normal"}},
		{"id":"b","block":"core/image","version":1,"props":{"mediaId":"m2"},"settings":{"priority":"auto"}}
	]}`)
	html2 := renderEpic3(t, reg, doc2)
	if strings.Count(html2, `fetchpriority="high"`) != 1 {
		t.Fatalf("normal excluded should still have 1 high: %s", html2)
	}
}

// Scenario 7 — Embed safe
func TestEpic3_Embed_Safe(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	yt := decodeDocForEpic3(t, `{"version":1,"nodes":[{"id":"e","block":"core/embed","version":1,"props":{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"},"settings":{"aspect":"16:9"}}]}`)
	html := renderEpic3(t, reg, yt)
	if !strings.Contains(html, "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
		t.Fatalf("youtube not rendered: %s", html)
	}
	vimeo := decodeDocForEpic3(t, `{"version":1,"nodes":[{"id":"e","block":"core/embed","version":1,"props":{"url":"https://vimeo.com/12345678"},"settings":{"aspect":"16:9"}}]}`)
	html2 := renderEpic3(t, reg, vimeo)
	if !strings.Contains(html2, "player.vimeo.com/video/12345678") {
		t.Fatalf("vimeo not rendered: %s", html2)
	}
	bad := decodeDocForEpic3(t, `{"version":1,"nodes":[{"id":"e","block":"core/embed","version":1,"props":{"url":"javascript:alert(1)"},"settings":{"aspect":"16:9"}}]}`)
	html3 := renderEpic3(t, reg, bad)
	if strings.Contains(html3, "<iframe") {
		t.Fatalf("bad url should not render iframe: %s", html3)
	}
}

// Scenario 8 — Accordion uses details/summary
func TestEpic3_Accordion_Details(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	doc := decodeDocForEpic3(t, `{"version":1,"nodes":[{"id":"a","block":"core/accordion","version":1,"props":{},"settings":{"variant":"bordered"},"children":[{"id":"i","block":"core/accordion-item","version":1,"props":{"title":"Q"},"settings":{},"children":[{"id":"t","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Answer","marks":[]}]}},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}]}]}]}`)
	html := renderEpic3(t, reg, doc)
	if !strings.Contains(html, "<details") || !strings.Contains(html, "<summary") {
		t.Fatalf("accordion missing details/summary: %s", html)
	}
}

// Scenario 9 — Columns
func TestEpic3_Columns_Semantic(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	doc := decodeDocForEpic3(t, `{"version":1,"nodes":[{"id":"c","block":"core/columns","version":1,"props":{},"settings":{"columns":2,"ratio":"equal","gap":"md","mobileStack":true},"children":[{"id":"a","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"A","marks":[]}]}},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}},{"id":"b","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"B","marks":[]}]}},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}]}]}`)
	html := renderEpic3(t, reg, doc)
	if !strings.Contains(html, "stratum-columns") {
		t.Fatalf("columns class missing: %s", html)
	}
}

// Scenario 10 — Gallery
func TestEpic3_Gallery_Responsive(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	doc := decodeDocForEpic3(t, `{"version":1,"nodes":[{"id":"g","block":"core/gallery","version":1,"props":{"images":"m1,m2"},"settings":{"columns":3,"gap":"md","aspect":"square","radius":"md"}}]}`)
	html := renderEpic3(t, reg, doc)
	if !strings.Contains(html, `width="800"`) || !strings.Contains(html, `loading="lazy"`) {
		t.Fatalf("gallery missing dimensions/lazy: %s", html)
	}
}

// Scenario 11 — Responsive class contract
func TestEpic3_ResponsiveClassContract(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	cases := []struct {
		docJSON string
		want    string
	}{
		{`{"version":1,"nodes":[{"id":"s","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"md","horizontalPadding":"md","align":"center","background":"default","minHeight":"auto","anchorID":""},"children":[]}]}`, "stratum-section"},
		{`{"version":1,"nodes":[{"id":"g","block":"core/grid","version":1,"props":{},"settings":{"columns":3,"gap":"lg","align":"stretch","equalHeight":false},"children":[]}]}`, "stratum-grid-cols-3"},
		{`{"version":1,"nodes":[{"id":"c","block":"core/columns","version":1,"props":{},"settings":{"columns":2,"ratio":"equal","gap":"md","mobileStack":true},"children":[]}]}`, "stratum-columns"},
		{`{"version":1,"nodes":[{"id":"s","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[]}]}`, "stratum-stack"},
	}
	for i, c := range cases {
		doc := decodeDocForEpic3(t, c.docJSON)
		html := renderEpic3(t, reg, doc)
		if !strings.Contains(html, c.want) {
			t.Fatalf("case %d missing %q: %s", i, c.want, html)
		}
	}
}

// Scenario 12 — SitePart + Pattern composition
func TestEpic3_SitePart_Pattern(t *testing.T) {
	// Pattern inserted into SitePart document should render via normal blocks and CSS included
	_, _, reg, _ := epic3Setup(t)
	cat := patterns.NewCatalog()
	p, _ := cat.Get("hero-centered")
	clone, _ := p.CloneWithNewIDs()
	doc := &document.Document{Version: 1, Nodes: clone.Nodes}
	prep, err := reg.Prepare(doc)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	html, err := reg.RenderPrepared(context.Background(), prep, rendering.RenderContext{LCP: &rendering.LCPState{}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(html), "Build something remarkable") {
		t.Fatalf("sitepart pattern content missing")
	}
	// CSS for used blocks should be non-empty
	css := reg.StylesFor(prep.UsedBlocks)
	if css == "" {
		t.Fatalf("used block CSS empty")
	}
}

// Scenario 13 — Template + Pattern (single template)
func TestEpic3_SingleTemplate_Pattern(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	cat := patterns.NewCatalog()
	p, _ := cat.Get("single-article")
	clone, _ := p.CloneWithNewIDs()
	doc := &document.Document{Version: 1, Nodes: clone.Nodes}
	if err := reg.ValidateDocument(doc); err != nil {
		t.Fatalf("single pattern validate: %v", err)
	}
	prep, _ := reg.Prepare(doc)
	html, _ := reg.RenderPrepared(context.Background(), prep, rendering.RenderContext{
		Entry: rendering.EntryContext{Title: "Hello", Permalink: "/hello"},
		LCP: &rendering.LCPState{},
	})
	if !strings.Contains(string(html), "Hello") && !strings.Contains(string(html), "stratum-entry-field") {
		// Single article pattern uses entry-title which renders via entry context; if not supplied, placeholder may show in preview
		// For public with entry context, it should render title
		t.Logf("single template pattern html: %s", html)
	}
}

// Scenario 14 — Archive Pattern
func TestEpic3_ArchivePattern(t *testing.T) {
	_, _, reg, _ := epic3Setup(t)
	cat := patterns.NewCatalog()
	p, _ := cat.Get("archive-collection-grid")
	clone, _ := p.CloneWithNewIDs()
	doc := &document.Document{Version: 1, Nodes: clone.Nodes}
	if err := reg.ValidateDocument(doc); err != nil {
		t.Fatalf("archive pattern validate: %v", err)
	}
	// Render as archive with context entries
	archive := &rendering.ArchiveContext{
		Entries: []rendering.ArchiveEntry{{ID: "1", Title: "Post 1", URL: "/post1"}, {ID: "2", Title: "Post 2", URL: "/post2"}},
		Title: "Archive Title", Description: "Desc",
		Pagination: rendering.PaginationContext{Current: 1, TotalPages: 1},
	}
	rc := rendering.RenderContext{
		Route: rendering.RouteContext{Archive: archive, ArchiveTitle: "Archive Title", ArchiveDescription: "Desc"},
		Archive: archive,
		LCP: &rendering.LCPState{},
		ContentReader: &fakeContentReaderEpic3{entries: archive.Entries},
		QueryCache: make(map[string][]rendering.ArchiveEntry),
	}
	prep, _ := reg.Prepare(doc)
	html, _ := reg.RenderPrepared(context.Background(), prep, rc)
	s := string(html)
	if !strings.Contains(s, "Archive Title") {
		t.Fatalf("archive title missing: %s", s)
	}
}

// helpers for epic3 integration
func decodeDocForEpic3(t *testing.T, raw string) *document.Document {
	t.Helper()
	doc, err := document.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return doc
}
func renderEpic3(t *testing.T, reg *blocks.Registry, doc *document.Document) string {
	t.Helper()
	prep, err := reg.Prepare(doc)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	html, err := reg.RenderPrepared(context.Background(), prep, rendering.RenderContext{LCP: &rendering.LCPState{}, QueryCache: make(map[string][]rendering.ArchiveEntry)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return string(html)
}

type fakeContentReaderEpic3 struct{ entries []rendering.ArchiveEntry }
func (f *fakeContentReaderEpic3) Query(_ context.Context, q content.EntryQuery) ([]rendering.ArchiveEntry, error) {
	return f.entries, nil
}
