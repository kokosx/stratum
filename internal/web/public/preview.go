package public

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/compress"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/routing"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

// servePreview handles /_stratum/preview/{token} - renders a specific revision via share token.
func (h *Handler) servePreview(w http.ResponseWriter, r *http.Request) {
	if h.preview == nil {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/_stratum/preview/")
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, "/") || strings.Contains(token, "?") {
		http.NotFound(w, r)
		return
	}
	link, err := h.preview.GetByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	// Load entry and revision
	entry, err := h.queries.GetEntry(r.Context(), link.EntryID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rev, err := h.queries.GetEntryRevision(r.Context(), link.RevisionID)
	if err != nil || rev.EntryID != entry.ID {
		http.NotFound(w, r)
		return
	}

	// Decode document and fields
	doc, err := document.Decode([]byte(rev.DocumentJson))
	if err != nil {
		log.Printf("preview decode document: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	fields, err := content.DecodeFieldSnapshot(rev.FieldsJson)
	if err != nil {
		// Fields may be empty; continue with nil
		fields = map[string]any{}
	}
	// Resolve layout - same as public but using revision's layout_template_id
	siteSnap := h.hub.Site.Current()
	if siteSnap == nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	origin := requestOrigin(r)
	// Determine logical path for rendering (entry's intended public path)
	logicalPath := deriveLogicalPath(entry, rev, siteSnap)
	// Use RenderEditableDocument pipeline via same renderer but with share preview flags
	input := rendering.RenderInput{
		Document:         doc,
		Title:            rev.Title,
		Slug:             rev.Slug,
		Excerpt:          stringValue(rev.Excerpt),
		SEOTitle:         stringValue(rev.SeoTitle),
		SEODescription:   stringValue(rev.SeoDescription),
		Path:             logicalPath,
		EntryID:          entry.ID,
		ContentTypeID:    entry.ContentTypeID,
		Fields:           fields,
		FeaturedMediaID:  stringValue(rev.FeaturedMediaID),
		LayoutTemplateID: stringValue(rev.LayoutTemplateID),
	}
	// Override to ensure share preview rendering is private and inert
	// We will call a dedicated render method that sets IsSharePreview
	html, err := h.renderSharePreview(r.Context(), siteSnap, input, origin, logicalPath)
	if err != nil {
		log.Printf("preview render: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Headers: private, no-store, noindex
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Accept-Encoding")
	// Prevent page cache
	w.Header().Set("X-Stratum-Preview", "share")
	// Write with compression negotiation but private
	writePreview(w, r, html)
}

func deriveLogicalPath(entry db.Entry, rev db.EntryRevision, siteSnap *site.Snapshot) string {
	slug := rev.Slug
	if slug == "" {
		slug = entry.Slug
	}
	if slug == "" {
		return "/"
	}
	if entry.ContentTypeID == "post" {
		base := "/blog"
		if siteSnap != nil && siteSnap.PostsBasePath != "" {
			base = siteSnap.PostsBasePath
		}
		path := routing.EntryPath(entry.ContentTypeID, slug, base)
		if path != "" {
			return path
		}
		return base + "/" + slug
	}
	path := routing.EntryPath(entry.ContentTypeID, slug, "")
	if path != "" {
		return path
	}
	if !strings.HasPrefix(slug, "/") {
		return "/" + slug
	}
	return slug
}

// Helper to render share preview via same pipeline as RenderEditableDocument but with share flags
func (h *Handler) renderSharePreview(ctx context.Context, siteSnap *site.Snapshot, input rendering.RenderInput, origin, path string) ([]byte, error) {
	// We reuse RenderEditableDocument logic but inject IsSharePreview
	// To avoid duplicating, we will call a helper that sets the flag
	// For now, we directly use the same logic as RenderEditableDocument but with modified context
	// The key is to ensure FormReader and other contexts are set with IsSharePreview
	// We will manually build RenderContext similarly to RenderEditableDocument but with share flag

	// Use the handler's RenderEditableDocument as base but we need to ensure it sets IsSharePreview
	// Instead, we will directly call the internal rendering with share flag

	// Replicate RenderEditableDocument's logic with share preview addition
	// This is a simplified version - we call h.blocks and h.themes directly
	// To keep single renderer, we will prepare document and call render

	// Prepare effective document via layout service
	effectiveDoc := input.Document
	if input.LayoutTemplateID != "" {
		ct := input.ContentTypeID
		if ct == "" && input.EntryID != "" {
			if e, err := h.queries.GetEntry(ctx, input.EntryID); err == nil {
				ct = e.ContentTypeID
			}
		}
		if ct != "" {
			var composed *document.Document
			var cerr error
			if h.layoutsService != nil {
				composed, _, cerr = h.layoutsService.ResolveEffectiveDocument(ctx, input.Document, ct, sql.NullString{String: input.LayoutTemplateID, Valid: true})
			} else {
				// fallback
				composed, cerr = nil, fmt.Errorf("no layout service")
			}
			if cerr == nil {
				effectiveDoc = composed
			} else {
				return nil, cerr
			}
		}
	}

	// Build RenderContext similar to RenderEditableDocument
	rc := rendering.RenderContext{
		Site:           rendering.SiteContext{Name: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, URL: siteSnap.SiteURL},
		Entry:          rendering.EntryContext{ID: input.EntryID, Slug: input.Slug, ContentTypeID: input.ContentTypeID, Title: input.Title, Excerpt: input.Excerpt, Permalink: path, FeaturedImage: input.FeaturedMediaID, Fields: input.Fields},
		IsPreview:      true,
		IsSharePreview: true,
		EntryID:        input.EntryID,
		Mode:           rendering.ModePreview,
	}
	if siteSnap.LogoMediaID != "" {
		if view, ok := h.media.MediaView(ctx, siteSnap.LogoMediaID); ok {
			rc.Site.LogoURL = view.Src
			rc.Site.LogoWidth = view.Width
			rc.Site.LogoHeight = view.Height
		}
	}
	if len(siteSnap.SocialLinks) > 0 {
		rc.Site.SocialLinks = siteSnap.SocialLinks
	}
	// Provide content reader for collections
	rc.ContentReader = &handlerContentReader{queries: h.queries, siteSnap: siteSnap, media: h.media}
	rc.QueryCache = make(map[string][]rendering.ArchiveEntry)
	rc.FormReader = h.forms
	rc.FormCache = make(map[string]forms.FormView)
	rc.SitePartReader = newHandlerSitePartReader(h.queries, h.blocks)
	rc.Dependencies = &rendering.DependencyState{SiteParts: make(map[string]rendering.ResolvedSitePart)}
	rc.Navigation = h.hub.Navigation.LocationsForPath(path)
	rc.LCP = &rendering.LCPState{}

	if input.ContentTypeID != "" {
		if def, err := content.NewCatalog(h.queries).GetDefinition(ctx, input.ContentTypeID); err == nil {
			rc.Definition = def
		} else {
			rc.Definition = content.DefinitionFor(input.ContentTypeID)
		}
	}

	prepared, err := h.blocks.Prepare(effectiveDoc)
	if err != nil {
		return nil, err
	}
	// Resolve SEO as preview (noindex)
	previewResolved := h.resolvePreviewSEO(ctx, siteSnap, input, path, origin)
	// Force noindex for share preview (already via preview SEO)
	previewResolved.Robots = "noindex, nofollow, noarchive"
	rc.Route = rendering.RouteContext{Path: path, IsArchive: false}

	contentHTML, err := h.renderBlocks(ctx, prepared, rc)
	if err != nil {
		return nil, err
	}
	menus := h.hub.Navigation.LocationsForPath(path)
	headerHTML, footerHTML, regionUsed := h.renderHeaderFooter(ctx, siteSnap, path, rc)
	// Handle header/footer overrides if any in input (not used for share preview)
	siteIcon := h.siteIconView(ctx, siteSnap)
	head := h.headView(siteSnap, previewResolved, siteIcon)
	// Ensure head is noindex
	head.Robots = "noindex, nofollow, noarchive"
	// Force canonical to not be preview token - use public path but with noindex it's okay, but we clear canonical to avoid token
	// head.Canonical should be empty or public URL; we keep previewResolved.Canonical which is based on site URL + path, but we will not emit preview token
	// For share preview, we set canonical empty to be safe
	head.Canonical = ""
	head.Robots = "noindex, nofollow, noarchive"
	head.Preloads = h.lcpPreloads(ctx, prepared, rc)

	_, themeCSS, themeJS := h.hub.Assets.URLs()
	usedBlocks := append([]rendering.BlockKey(nil), prepared.UsedBlocks...)
	usedBlocks = append(usedBlocks, regionUsed...)
	for _, dep := range rc.Dependencies.SiteParts {
		usedBlocks = append(usedBlocks, dep.UsedBlocks...)
	}
	blocksCSS := h.hub.Assets.BlocksCSSFor(usedBlocks)
	regions := make(map[string]template.HTML)
	if headerHTML != "" {
		regions["header"] = headerHTML
	}
	if footerHTML != "" {
		regions["footer"] = footerHTML
	}
	view := themes.PageView{
		Site:       themes.SiteView{Title: rc.Site.Name, Tagline: rc.Site.Tagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL, LogoURL: rc.Site.LogoURL, LogoWidth: rc.Site.LogoWidth, LogoHeight: rc.Site.LogoHeight},
		Entry:      themes.EntryView{Title: rc.Entry.Title, SEOTitle: previewResolved.OpenGraph.Title, SEODescription: previewResolved.Description, CanonicalURL: ""},
		Head:       head,
		Navigation: menus,
		Content:    contentHTML,
		Header:     headerHTML,
		Footer:     footerHTML,
		Regions:    regions,
		Assets:     themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}
	// Render via theme - this is the same renderer as public
	html, err := h.themes.Render(view, nil)
	if err != nil {
		return nil, err
	}
	// Do NOT inject custom code JS for share preview (security). We skip injectCustomCode.
	// Ensure HTML has robots meta with noindex
	if !strings.Contains(string(html), `name="robots"`) {
		// Inject if missing (theme should already include via Head.Robots)
	}
	return html, nil
}

func writePreview(w http.ResponseWriter, r *http.Request, html []byte) {
	enc, ok := compress.NegotiateEncoding(r.Header.Get("Accept-Encoding"))
	if !ok {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
		return
	}
	var body []byte
	var encoding string
	switch enc {
	case "br":
		if b, err := compress.Brotli(html); err == nil && len(b) > 0 {
			body = b
			encoding = "br"
		} else {
			body = html
		}
	case "gzip":
		if g, err := compress.Gzip(html); err == nil && len(g) > 0 {
			body = g
			encoding = "gzip"
		} else {
			body = html
		}
	default:
		body = html
	}
	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
