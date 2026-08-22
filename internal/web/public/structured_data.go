package public

import (
	"context"
	"log"

	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/seo"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/structured"
)

// buildStructuredData renders the first-party JSON-LD graph for a published
// entry. It is additive SEO output: failures are logged and never break the
// page render. The payload is produced by encoding/json in the structured
// package, so what reaches the theme is already safe to embed.
func (h *Handler) buildStructuredData(ctx context.Context, siteSnap *site.Snapshot, entry *db.GetPublishedEntryByPathRow, path, origin string, resolved seo.Resolved) string {
	in := structured.Site{
		Title:      siteSnap.SiteTitle,
		URL:        siteSnap.SiteURL,
		Origin:     origin,
		Language:   siteSnap.Language,
		Represents: siteSnap.SiteRepresents,
		SocialURLs: socialURLs(siteSnap.SocialLinks),
	}
	if siteSnap.LogoMediaID != "" && h.media != nil {
		if view, ok := h.media.MediaView(ctx, siteSnap.LogoMediaID); ok {
			in.LogoURL = seo.BaseURL(siteSnap.SiteURL, origin) + view.Src
		}
	}

	page := structured.Page{
		Path:          path,
		ContentTypeID: entry.ContentTypeID,
		Name:          resolved.OpenGraph.Title,
		Description:   resolved.Description,
		CanonicalURL:  resolved.Canonical,
		PublishedUnix: firstPublishedAt(entry),
		ModifiedUnix:  modifiedAt(entry),
		Timezone:      siteSnap.TimezoneName,
	}
	if resolved.OpenGraph.Image != "" {
		page.Image = &structured.Image{
			URL:         resolved.OpenGraph.Image,
			Width:       resolved.OpenGraph.ImageWidth,
			Height:      resolved.OpenGraph.ImageHeight,
			Description: resolved.OpenGraph.ImageAlt,
		}
	}

	payload, err := structured.Build(in, page)
	if err != nil {
		log.Printf("structured data: %v", err)
		return ""
	}
	return payload
}

// socialURLs extracts the absolute profile URLs feeding publisher sameAs.
func socialURLs(links []rendering.SiteSocialLink) []string {
	out := make([]string, 0, len(links))
	for _, link := range links {
		if link.URL != "" {
			out = append(out, link.URL)
		}
	}
	return out
}

// firstPublishedAt returns the stable FIRST publication timestamp. Rows that
// predate the column keep working through the published_at fallback.
func firstPublishedAt(entry *db.GetPublishedEntryByPathRow) int64 {
	if entry.FirstPublishedAt.Valid {
		return entry.FirstPublishedAt.Int64
	}
	if entry.PublishedAt.Valid {
		return entry.PublishedAt.Int64
	}
	return 0
}

// modifiedAt returns the publication time of the current published revision.
func modifiedAt(entry *db.GetPublishedEntryByPathRow) int64 {
	if entry.PublishedAt.Valid {
		return entry.PublishedAt.Int64
	}
	return 0
}
