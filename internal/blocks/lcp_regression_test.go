package blocks

import (
	"context"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/rendering"
)

func TestLCP_T1_CollectionAutoFiveEntriesOneHigh(t *testing.T) {
	reg := newCorrectiveRegistry(t)
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c1","block":"core/collection","version":1,"props":{},"settings":{"source":"query","limit":5},"children":[{"id":"f1","block":"core/featured-image","version":1,"props":{},"settings":{}}]}]}`)
	entries := []rendering.ArchiveEntry{
		{ID: "e1", Title: "T1", URL: "/1", FeaturedImage: rendering.MediaView{ID: "m1", Src: "/m1.jpg", SrcSet: "/m1 1x", Width: 10, Height: 10}},
		{ID: "e2", Title: "T2", URL: "/2", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", Width: 10, Height: 10}},
		{ID: "e3", Title: "T3", URL: "/3", FeaturedImage: rendering.MediaView{ID: "m3", Src: "/m3.jpg", Width: 10, Height: 10}},
		{ID: "e4", Title: "T4", URL: "/4", FeaturedImage: rendering.MediaView{ID: "m1", Src: "/m1.jpg", Width: 10, Height: 10}},
		{ID: "e5", Title: "T5", URL: "/5", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", Width: 10, Height: 10}},
	}
	reader := &fakeContentReader{entries: entries}
	prep, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{ContentReader: reader, QueryCache: make(map[string][]rendering.ArchiveEntry), LCP: &rendering.LCPState{}}
	html, _ := reg.RenderPrepared(context.Background(), prep, rc)
	s := string(html)
	countHigh := strings.Count(s, `fetchpriority="high"`)
	if countHigh != 1 {
		t.Fatalf("expected exactly 1 fetchpriority=high, got %d: %s", countHigh, s)
	}
	countEager := strings.Count(s, `loading="eager"`)
	if countEager != 1 {
		t.Fatalf("expected exactly 1 loading=eager, got %d: %s", countEager, s)
	}
	if rc.LCP.PreloadHref == "" {
		t.Fatalf("preload not set")
	}
	// Preload must equal the actual high image src
	if !strings.Contains(s, `src="`+rc.LCP.PreloadHref+`"`) {
		t.Fatalf("preload href %q not found in rendered html %s", rc.LCP.PreloadHref, s)
	}
}

func TestLCP_T2_FirstNoImageSecondHas(t *testing.T) {
	reg := newCorrectiveRegistry(t)
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c1","block":"core/collection","version":1,"props":{},"settings":{"source":"query","limit":2},"children":[{"id":"f1","block":"core/featured-image","version":1,"props":{},"settings":{}}]}]}`)
	entries := []rendering.ArchiveEntry{
		{ID: "e1", Title: "NoImg", URL: "/1", FeaturedImage: rendering.MediaView{}},
		{ID: "e2", Title: "HasImg", URL: "/2", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", SrcSet: "/m2 1x", Width: 10, Height: 10}},
	}
	reader := &fakeContentReader{entries: entries}
	prep, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{ContentReader: reader, QueryCache: make(map[string][]rendering.ArchiveEntry), LCP: &rendering.LCPState{}}
	html, _ := reg.RenderPrepared(context.Background(), prep, rc)
	s := string(html)
	if strings.Count(s, `fetchpriority="high"`) != 1 {
		t.Fatalf("expected 1 high, got %s", s)
	}
	if !strings.Contains(s, "/media/m2/") && !strings.Contains(s, "/m2.jpg") {
		t.Fatalf("second image not high: %s", s)
	}
	if !strings.Contains(rc.LCP.PreloadHref, "m2") {
		t.Fatalf("preload should be m2, got %q", rc.LCP.PreloadHref)
	}
}

func TestLCP_T3_CollectionHighWinsOverLaterAuto(t *testing.T) {
	reg := newCorrectiveRegistry(t)
	// Document: Collection with FeaturedImage priority=high, then later normal Image auto
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c1","block":"core/collection","version":1,"props":{},"settings":{"source":"query","limit":2},"children":[{"id":"f1","block":"core/featured-image","version":1,"props":{},"settings":{"priority":"high"}}]},{"id":"img1","block":"core/image","version":1,"props":{"mediaId":"m1"},"settings":{}}]}`)
	entries := []rendering.ArchiveEntry{
		{ID: "e1", Title: "E1", URL: "/e1", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", Width: 10, Height: 10}},
		{ID: "e2", Title: "E2", URL: "/e2", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", Width: 10, Height: 10}},
	}
	reader := &fakeContentReader{entries: entries}
	prep, _ := reg.Prepare(doc)
	// Outer page has no featured image
	rc := rendering.RenderContext{ContentReader: reader, QueryCache: make(map[string][]rendering.ArchiveEntry), Entry: rendering.EntryContext{FeaturedImage: ""}, LCP: &rendering.LCPState{}}
	html, _ := reg.RenderPrepared(context.Background(), prep, rc)
	s := string(html)
	// Collection high should win, so exactly one high and it should be the collection's featured image (m2), not the later image m1
	count := strings.Count(s, `fetchpriority="high"`)
	if count != 1 {
		t.Fatalf("expected 1 high, got %d: %s", count, s)
	}
	// The high image should be the featured one (which renders with media view m2)
	if !strings.Contains(rc.LCP.PreloadHref, "m2") {
		t.Fatalf("preload should be collection high m2, got %q, html %s", rc.LCP.PreloadHref, s)
	}
}

func TestLCP_T4_ExplicitHighWinsOverCollectionAuto(t *testing.T) {
	reg := newCorrectiveRegistry(t)
	// Explicit high featured-image before collection auto: high should win per policy (high list first)
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"featHigh","block":"core/featured-image","version":1,"props":{},"settings":{"priority":"high"}},{"id":"c1","block":"core/collection","version":1,"props":{},"settings":{"source":"query","limit":2},"children":[{"id":"f1","block":"core/featured-image","version":1,"props":{},"settings":{}}]}]}`)
	entries := []rendering.ArchiveEntry{
		{ID: "e1", Title: "E1", URL: "/e1", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", Width: 10, Height: 10}},
		{ID: "e2", Title: "E2", URL: "/e2", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/m2.jpg", Width: 10, Height: 10}},
	}
	reader := &fakeContentReader{entries: entries}
	prep, _ := reg.Prepare(doc)
	// Outer entry has m1 so featHigh is eligible
	rc := rendering.RenderContext{ContentReader: reader, QueryCache: make(map[string][]rendering.ArchiveEntry), Entry: rendering.EntryContext{FeaturedImage: "m1"}, LCP: &rendering.LCPState{}}
	html, _ := reg.RenderPrepared(context.Background(), prep, rc)
	s := string(html)
	if strings.Count(s, `fetchpriority="high"`) != 1 {
		t.Fatalf("expected 1 high, got %s", s)
	}
	if !strings.Contains(rc.LCP.PreloadHref, "m1") {
		t.Fatalf("explicit high m1 should win, preload %q, html %s", rc.LCP.PreloadHref, s)
	}
}

func TestLCP_T5_PreLoadMatchesActualHighImage(t *testing.T) {
	reg := newCorrectiveRegistry(t)
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c1","block":"core/collection","version":1,"props":{},"settings":{"source":"query","limit":3},"children":[{"id":"f1","block":"core/featured-image","version":1,"props":{},"settings":{}}]}]}`)
	entries := []rendering.ArchiveEntry{
		{ID: "e1", Title: "E1", URL: "/e1", FeaturedImage: rendering.MediaView{ID: "m1", Src: "/media/m1/768", SrcSet: "/media/m1/480 480w, /media/m1/768 768w", Width: 800, Height: 600}},
		{ID: "e2", Title: "E2", URL: "/e2", FeaturedImage: rendering.MediaView{ID: "m2", Src: "/media/m2/768", SrcSet: "/media/m2/480 480w", Width: 800, Height: 600}},
		{ID: "e3", Title: "E3", URL: "/e3", FeaturedImage: rendering.MediaView{ID: "m3", Src: "/media/m3/768", Width: 800, Height: 600}},
	}
	reader := &fakeContentReader{entries: entries}
	prep, _ := reg.Prepare(doc)
	rc := rendering.RenderContext{ContentReader: reader, QueryCache: make(map[string][]rendering.ArchiveEntry), LCP: &rendering.LCPState{}}
	html, _ := reg.RenderPrepared(context.Background(), prep, rc)
	s := string(html)
	// Find the img tag with fetchpriority high and extract its src
	idx := strings.Index(s, `fetchpriority="high"`)
	if idx == -1 {
		t.Fatalf("no high found %s", s)
	}
	// Ensure preload src appears in that same img tag context: look backward for src
	start := max(0, idx-500)
	end := idx + 500
	if end > len(s) {
		end = len(s)
	}
	segment := s[start:end]
	if !strings.Contains(segment, rc.LCP.PreloadHref) {
		t.Fatalf("preload href %q not near high image segment %q", rc.LCP.PreloadHref, segment)
	}
	if rc.LCP.PreloadSrcSet == "" {
		t.Fatalf("preload srcset empty")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
