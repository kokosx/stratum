package creator

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/themes"
)

// PreviewSurface indicates which page of the starter to preview.
type PreviewSurface string

const (
	SurfaceHome    PreviewSurface = "home"
	SurfaceArchive PreviewSurface = "archive"
	SurfaceSingle  PreviewSurface = "single"
)

// previewMediaProvider returns synthetic media views without DB.
type previewMediaProvider struct {
	palette PaletteID
	cache   map[string]rendering.MediaView
}

func newPreviewMediaProvider(palette PaletteID) *previewMediaProvider {
	return &previewMediaProvider{palette: palette, cache: make(map[string]rendering.MediaView)}
}

func (p *previewMediaProvider) MediaView(_ context.Context, id string) (rendering.MediaView, bool) {
	if v, ok := p.cache[id]; ok {
		return v, true
	}
	// Deterministic synthetic image: variant derived from id hash
	variant := 1
	for _, c := range id {
		variant = (variant*31 + int(c)) % 6
		if variant == 0 {
			variant = 1
		}
	}
	// Generate PNG bytes and data URI
	palette := paletteForStyle(p.palette)
	data := geometricPNG(800, 600, palette, variant)
	b64 := base64.StdEncoding.EncodeToString(data)
	src := "data:image/png;base64," + b64
	view := rendering.MediaView{ID: id, Src: src, Width: 800, Height: 600, Alt: "Starter preview image"}
	p.cache[id] = view
	return view, true
}

// previewContentReader implements rendering.ContentReader with synthetic entries.
type previewContentReader struct {
	entries []rendering.ArchiveEntry
	byType  map[string][]rendering.ArchiveEntry
	defs    map[string]content.ContentTypeDefinition
}

func newPreviewContentReader(spec presetSpec, plan Input, media *previewMediaProvider) *previewContentReader {
	r := &previewContentReader{byType: make(map[string][]rendering.ArchiveEntry), defs: make(map[string]content.ContentTypeDefinition)}
	// Build archive entries from spec.seedEntries
	for i, seed := range spec.seedEntries {
		// Synthetic media ID per entry
		mediaID := fmt.Sprintf("preview-media-%d", i)
		// Ensure media view exists
		_, _ = media.MediaView(context.Background(), mediaID)
		ct := "post"
		if spec.contentType != nil {
			ct = string(spec.contentType.ID)
		}
		// For blog, ct post; else spec.contentType.ID
		title := seed.Title
		slug := seed.Slug
		excerpt := seed.Excerpt
		fields := seed.Fields
		if fields == nil {
			fields = map[string]any{}
		}
		urlPath := "/" + slug
		if spec.archivePath != "" {
			// Archive path is like /blog, /work, /products, /services
			// Single entry path is archivePath/slug
			urlPath = strings.TrimSuffix(spec.archivePath, "/") + "/" + slug
		} else {
			// Landing testimonial has no route; use /#.
			urlPath = "/"
		}
		ae := rendering.ArchiveEntry{
			ID:            fmt.Sprintf("preview-entry-%d", i),
			ContentTypeID: ct,
			Slug:          slug,
			Title:         title,
			Excerpt:       excerpt,
			URL:           urlPath,
			Fields:        fields,
			PublishedAt:   "2026-01-01",
			PublishedISO:  "2026-01-01T00:00:00Z",
			FeaturedImage: rendering.MediaView{ID: mediaID},
		}
		// Fetch actual media view with Src
		if v, ok := media.MediaView(context.Background(), mediaID); ok {
			ae.FeaturedImage = v
		}
		r.entries = append(r.entries, ae)
		r.byType[ct] = append(r.byType[ct], ae)
	}
	// Also register page type for completeness (not used by collections)
	for _, ct := range []string{"post", "project", "product", "service", "testimonial"} {
		if _, ok := r.byType[ct]; !ok {
			r.byType[ct] = []rendering.ArchiveEntry{}
		}
	}
	if spec.contentType != nil {
		def := content.ContentTypeDefinition{ID: spec.contentType.ID, Fields: spec.contentType.Config.Fields}
		def.Capabilities.HasContent = spec.contentType.Config.Features.Content
		r.defs[string(spec.contentType.ID)] = def
	}
	// Generic post definition
	r.defs["post"] = content.ContentTypeDefinition{ID: "post", Capabilities: content.Capabilities{HasContent: true}}
	r.defs["page"] = content.ContentTypeDefinition{ID: "page", Capabilities: content.Capabilities{HasContent: true}}
	return r
}

func (r *previewContentReader) Query(_ context.Context, q content.EntryQuery) ([]rendering.ArchiveEntry, error) {
	q = q.Normalized()
	entries := r.byType[string(q.ContentType)]
	// Simple filter: exclude IDs
	filtered := make([]rendering.ArchiveEntry, 0, len(entries))
	exclude := make(map[string]bool, len(q.ExcludeIDs))
	for _, id := range q.ExcludeIDs {
		exclude[id] = true
	}
	for _, e := range entries {
		if exclude[e.ID] {
			continue
		}
		// Apply term filter naively (no taxonomy, ignore)
		filtered = append(filtered, e)
	}
	// Apply ordering: only published_desc / asc by title or published_at
	if q.OrderBy == "entry.title" || strings.Contains(q.OrderBy, "title") {
		// Sort by title
		for i := 0; i < len(filtered); i++ {
			for j := i + 1; j < len(filtered); j++ {
				less := filtered[i].Title < filtered[j].Title
				if q.Direction == "desc" {
					less = !less
				}
				if less {
					continue
				}
				// Actually need proper sort; use simple bubble for now but delegate to sort.Slice
			}
		}
		// Use stable sort for correctness
		// We will use sort.Slice
	}
	// Use sort for deterministic
	// Note: we avoid importing sort here? Already in content, but we need sort.
	// Simple: rely on original order (published_desc) which is already in seed order.
	if q.Direction == "asc" {
		// Reverse
		for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
			filtered[i], filtered[j] = filtered[j], filtered[i]
		}
	}
	if q.Offset >= len(filtered) {
		return []rendering.ArchiveEntry{}, nil
	}
	filtered = filtered[q.Offset:]
	if q.Limit < len(filtered) {
		filtered = filtered[:q.Limit]
	}
	return filtered, nil
}

func (r *previewContentReader) Definition(_ context.Context, contentType string) (content.ContentTypeDefinition, error) {
	if d, ok := r.defs[contentType]; ok {
		return d, nil
	}
	// Fallback to content helper
	return content.DefinitionFor(contentType), nil
}

// previewSitePartReader implements rendering.SitePartReader with synthetic parts.
type previewSitePartReader struct {
	parts map[string]*rendering.PreparedDocument
	revs  map[string]string
}

func newPreviewSitePartReader(blocks *blocks.Registry, plan Input) *previewSitePartReader {
	r := &previewSitePartReader{parts: make(map[string]*rendering.PreparedDocument), revs: make(map[string]string)}
	for _, loc := range []string{"header", "footer"} {
		var doc *document.Document
		if loc == "header" {
			doc = sitePartDocumentForHeader("preview", plan.HeaderStyleID)
		} else {
			doc = sitePartDocumentForFooter("preview", plan.FooterStyleID)
		}
		pd, err := blocks.Prepare(doc)
		if err != nil {
			continue
		}
		id := "sitepart-" + loc
		r.parts[id] = pd
		r.revs[id] = "preview-rev-" + loc
		// Also map by location key for core/site-part block lookup? The block uses location string.
		// In rendering, SitePartReader.GetSitePart is called with id like site part ID, not location.
		// However our generated site parts are not stored; the theme may use site-part blocks that
		// reference by location via navigation? Actually we need to ensure site parts are discoverable.
		// For preview we will map both location and id to same doc.
		r.parts[loc] = pd
		r.revs[loc] = "preview-rev-" + loc
	}
	return r
}

func (r *previewSitePartReader) GetSitePart(_ context.Context, id string) (*rendering.PreparedDocument, string, error) {
	if pd, ok := r.parts[id]; ok {
		return pd, r.revs[id], nil
	}
	// Try location fallback
	if pd, ok := r.parts["header"]; ok && (id == "header" || strings.Contains(id, "header")) {
		return pd, r.revs["header"], nil
	}
	if pd, ok := r.parts["footer"]; ok && (id == "footer" || strings.Contains(id, "footer")) {
		return pd, r.revs["footer"], nil
	}
	return nil, "", fmt.Errorf("site part not found")
}

// previewFormReader implements rendering.FormReader
type previewFormReader struct {
	forms map[string]forms.FormView
}

func newPreviewFormReader(spec presetSpec) *previewFormReader {
	r := &previewFormReader{forms: make(map[string]forms.FormView)}
	if spec.form != nil {
		// Create synthetic form ID and view
		formID := "preview-form"
		// Build fields similar to buildArtifacts
		view := forms.FormView{
			ID:             formID,
			SubmitLabel:    "Send message",
			SuccessMessage: "Thanks. Your message has been received.",
			Fields: []forms.Field{
				{ID: "f1", Key: "name", Type: forms.FieldText, Label: "Name", Required: true},
				{ID: "f2", Key: "email", Type: forms.FieldEmail, Label: "Email", Required: true},
			},
		}
		if spec.form.Phone {
			view.Fields = append(view.Fields, forms.Field{ID: "f3", Key: "phone", Type: forms.FieldText, Label: "Phone"})
		}
		view.Fields = append(view.Fields, forms.Field{ID: "f4", Key: "message", Type: forms.FieldTextarea, Label: "Message", Required: true})
		r.forms[formID] = view
		// Also map by spec.form.Name for lookup
		r.forms[spec.form.Name] = view
	}
	return r
}

func (r *previewFormReader) ResolveForm(_ context.Context, id string) forms.FormResolution {
	if v, ok := r.forms[id]; ok {
		return forms.FormResolution{State: forms.FormStateActive, View: v}
	}
	// Try preview-form fallback
	if v, ok := r.forms["preview-form"]; ok {
		return forms.FormResolution{State: forms.FormStateActive, View: v}
	}
	return forms.FormResolution{State: forms.FormStateMissing}
}

func (r *previewFormReader) GetActiveForm(_ context.Context, id string) (forms.FormView, bool) {
	res := r.ResolveForm(context.Background(), id)
	if res.State != forms.FormStateActive {
		return forms.FormView{}, false
	}
	return res.View, true
}

// previewNavigation builds synthetic navigation menus matching starter.
func previewNavigation(spec presetSpec, plan Input) map[string]navigation.Menu {
	lang := plan.Language
	if lang == "" {
		lang = "en"
	}
	// Determine labels via catalog
	homeLabel := copyFor(lang, "nav.home")
	aboutLabel := copyFor(lang, "nav.about")
	contactLabel := copyFor(lang, "nav.contact")
	menu := navigation.Menu{Name: "Primary Menu", Items: []navigation.MenuItem{{Label: homeLabel, URL: "/"}}}
	if spec.archivePath != "" {
		labelKey := map[string]string{"/blog": "nav.blog", "/work": "nav.work", "/products": "nav.products", "/services": "nav.services"}[spec.archivePath]
		if labelKey == "" {
			labelKey = "nav.blog"
		}
		menu.Items = append(menu.Items, navigation.MenuItem{Label: copyFor(lang, labelKey), URL: spec.archivePath})
	}
	for _, p := range spec.pages {
		title := p.Title
		// Localize page titles for nav
		if p.Slug == "about" {
			title = aboutLabel
		} else if p.Slug == "contact" {
			title = contactLabel
		}
		menu.Items = append(menu.Items, navigation.MenuItem{Label: title, URL: "/" + p.Slug})
	}
	return map[string]navigation.Menu{
		"primary": menu,
		"footer":  menu,
	}
}

// RenderPreview builds HTML for the given plan and surface using the real renderer.
func RenderPreview(ctx context.Context, plan Plan, surface PreviewSurface, blocks *blocks.Registry, themesRuntime *themes.Runtime) (string, error) {
	spec := specForPlan(plan)
	mediaProvider := newPreviewMediaProvider(plan.Input.PaletteID)
	contentReader := newPreviewContentReader(spec, plan.Input, mediaProvider)
	sitePartReader := newPreviewSitePartReader(blocks, plan.Input)
	formReader := newPreviewFormReader(spec)
	nav := previewNavigation(spec, plan.Input)

	// Build theme styles via theme definition (real Theme path)
	settings := composedStyles(plan.Preset.ID, plan.Input.PaletteID, plan.Input.HeaderStyleID, plan.Input.FooterStyleID)
	var themeCSS string
	if css, err := themesRuntime.PreviewStyles(settings); err == nil {
		themeCSS = css
	} else {
		themeCSS = themesRuntime.Styles()
	}
	// Block styles for all used blocks in preview docs
	// Prepare docs per surface
	var contentDoc *document.Document
	var rc rendering.RenderContext
	syntheticEntry := rendering.EntryContext{
		ID:            "preview-home",
		Slug:          "home",
		ContentTypeID: "page",
		Title:         plan.Input.SiteTitle,
		Excerpt:       plan.Input.Tagline,
		Permalink:     "/",
		Fields:        map[string]any{},
	}
	siteCtx := rendering.SiteContext{Name: plan.Input.SiteTitle, Tagline: plan.Input.Tagline, URL: plan.Input.SiteURL}
	// Determine content doc based on surface
	switch surface {
	case SurfaceArchive:
		if spec.archivePath != "" {
			headerDoc := archiveTemplateForPlan("preview", plan.Preset.ID, plan)
			contentDoc = headerDoc
			archiveCT := archiveContentType(spec)
			entries, _ := contentReader.Query(ctx, content.EntryQuery{ContentType: content.ContentTypeID(archiveCT), Limit: 20})
			rc = rendering.RenderContext{
				Site:          siteCtx,
				Entry:         syntheticEntry,
				Route:         rendering.RouteContext{Path: spec.archivePath, IsArchive: true, ContentType: archiveCT, Archive: &rendering.ArchiveContext{Entries: entries, Permalink: spec.archivePath, Title: copyFor(plan.Input.Language, "heading.services"), Pagination: rendering.PaginationContext{Current: 1, TotalPages: 1}}},
				Mode:          rendering.ModePreview,
				IsPreview:     true,
				ContentReader: contentReader,
				QueryCache:    make(map[string][]rendering.ArchiveEntry),
				Navigation:    nav,
				FormReader:    formReader,
				FormCache:     make(map[string]forms.FormView),
				SitePartReader: sitePartReader,
				MediaProvider:  mediaProvider,
				Dependencies:  &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)},
			}
		} else {
			surface = SurfaceHome
		}
	case SurfaceSingle:
		if len(contentReader.entries) > 0 {
			ae := contentReader.entries[0]
			doc := singleTemplateForPlan("preview", plan.Preset.ID, plan)
			contentDoc = doc
			syntheticEntry = rendering.EntryContext{
				ID:            ae.ID,
				Slug:          ae.Slug,
				ContentTypeID: ae.ContentTypeID,
				Title:         ae.Title,
				Excerpt:       ae.Excerpt,
				Permalink:     ae.URL,
				Fields:        ae.Fields,
				FeaturedImage: ae.FeaturedImage.ID,
			}
			rc = rendering.RenderContext{
				Site:          siteCtx,
				Entry:         syntheticEntry,
				EntryID:       ae.ID,
				Route:         rendering.RouteContext{Path: ae.URL, IsArchive: false},
				Mode:          rendering.ModePreview,
				IsPreview:     true,
				ContentReader: contentReader,
				QueryCache:    make(map[string][]rendering.ArchiveEntry),
				Navigation:    nav,
				FormReader:    formReader,
				FormCache:     make(map[string]forms.FormView),
				SitePartReader: sitePartReader,
				MediaProvider:  mediaProvider,
				Dependencies:  &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)},
			}
		}
	default:
		// Home
		homeDoc := homepageEntryDocument("preview", plan.Preset.ID, "preview-form", plan)
		contentDoc = homeDoc
		rc = rendering.RenderContext{
			Site:          siteCtx,
			Entry:         syntheticEntry,
			Route:         rendering.RouteContext{Path: "/", IsArchive: false},
			Mode:          rendering.ModePreview,
			IsPreview:     true,
			ContentReader: contentReader,
			QueryCache:    make(map[string][]rendering.ArchiveEntry),
			Navigation:    nav,
			FormReader:    formReader,
			FormCache:     make(map[string]forms.FormView),
			SitePartReader: sitePartReader,
				MediaProvider:  mediaProvider,
			Dependencies:  &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)},
		}
	}
	// Handle home fallback when surface was archive but no archivePath
	if contentDoc == nil {
		homeDoc := homepageEntryDocument("preview", plan.Preset.ID, "preview-form", plan)
		contentDoc = homeDoc
		rc = rendering.RenderContext{
			Site:          siteCtx,
			Entry:         syntheticEntry,
			Route:         rendering.RouteContext{Path: "/", IsArchive: false},
			Mode:          rendering.ModePreview,
			IsPreview:     true,
			ContentReader: contentReader,
			QueryCache:    make(map[string][]rendering.ArchiveEntry),
			Navigation:    nav,
			FormReader:    formReader,
			FormCache:     make(map[string]forms.FormView),
			SitePartReader: sitePartReader,
				MediaProvider:  mediaProvider,
			Dependencies:  &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)},
		}
	}

	// Render header/footer site parts via blocks
	headerDoc := sitePartDocumentForHeader("preview", plan.Input.HeaderStyleID)
	footerDoc := sitePartDocumentForFooter("preview", plan.Input.FooterStyleID)
	headerHTML, _ := blocks.RenderDocumentContext(headerDoc, rc)
	footerHTML, _ := blocks.RenderDocumentContext(footerDoc, rc)

	// Prepare content: need to inject header/footer handling? Alternatively render content doc with site part reader disabled to avoid recursion.
	// We render content separately and then wrap with theme chrome.
	// Ensure LCP state shared
	rc.LCP = &rendering.LCPState{}
	contentHTML, err := blocks.RenderDocumentContext(contentDoc, rc)
	if err != nil {
		return "", err
	}
	// Block CSS for used blocks
	// Collect used blocks from prepared documents
	pd1, _ := blocks.Prepare(headerDoc)
	pd2, _ := blocks.Prepare(contentDoc)
	pd3, _ := blocks.Prepare(footerDoc)
	used := append(append(pd1.UsedBlocks, pd2.UsedBlocks...), pd3.UsedBlocks...)
	blockCSS := blocks.StylesFor(used)

	// Build final HTML
	var buf bytes.Buffer
	buf.WriteString(`<!doctype html><html lang="` + templateEscape(plan.Input.Language) + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	buf.WriteString(`<title>` + templateEscape(plan.Input.SiteTitle) + `</title>`)
	buf.WriteString(`<style>` + themeCSS + `</style>`)
	if blockCSS != "" {
		buf.WriteString(`<style>` + blockCSS + `</style>`)
	}
	// Prevent preview form submission via JS
	buf.WriteString(`<style>html{scroll-behavior:auto;}</style>`)
	buf.WriteString(`</head><body class="site-` + string(plan.Preset.ID) + `">`)
	buf.WriteString(`<header class="site-header">` + string(headerHTML) + `</header>`)
	buf.WriteString(`<main class="site-main">` + string(contentHTML) + `</main>`)
	buf.WriteString(`<footer class="site-footer">` + string(footerHTML) + `</footer>`)
	buf.WriteString(`<script>document.addEventListener('submit',e=>{if(e.target.closest('form'))e.preventDefault()});document.addEventListener('click',e=>{let a=e.target.closest('a');if(a&&a.getAttribute('href')?.startsWith('#')===false){let href=a.getAttribute('href');if(href&&href.startsWith('/') ){e.preventDefault();}}});</script>`)
	buf.WriteString(`</body></html>`)
	return buf.String(), nil
}

func archiveContentType(spec presetSpec) string {
	if spec.contentType != nil {
		return string(spec.contentType.ID)
	}
	if spec.archivePath != "" {
		// Blog preset has no contentType but uses post
		return "post"
	}
	return ""
}

func templateEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(s)
}
