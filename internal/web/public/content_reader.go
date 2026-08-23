package public

import (
	"context"

	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// handlerContentReader implements rendering.ContentReader via the stable
// EntryQuery shapes. It is the public-handler host for Collection blocks.
type handlerContentReader struct {
	queries  *db.Queries
	siteSnap *site.Snapshot
	media    interface {
		MediaView(ctx context.Context, id string) (rendering.MediaView, bool)
	}
}

func (r *handlerContentReader) Query(ctx context.Context, contentType string, limit, offset int, order string, excludeIDs []string) ([]rendering.ArchiveEntry, error) {
	if limit <= 0 {
		limit = 3
	}
	if limit > 50 {
		limit = 50
	}
	// Stable sort: currently only published_desc is supported by the DB query.
	// The order param is kept for memo key stability; actual SQL ordering is fixed.
	rows, err := r.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{
		ContentTypeID: contentType,
		Limit:         int64(limit + len(excludeIDs) + 5), // overfetch to allow filtering
		Offset:        int64(offset),
	})
	if err != nil {
		return nil, err
	}
	// Filter excludeIDs
	exclude := make(map[string]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		exclude[id] = struct{}{}
	}
	// Batch media for filtered rows
	featIDs := make([]string, 0)
	for _, row := range rows {
		if _, ok := exclude[row.ID]; ok {
			continue
		}
		if row.FeaturedMediaID.Valid && row.FeaturedMediaID.String != "" {
			featIDs = append(featIDs, row.FeaturedMediaID.String)
		}
	}
	mediaCache := map[string]rendering.MediaView{}
	if r.media != nil {
		for _, id := range featIDs {
			if v, ok := r.media.MediaView(ctx, id); ok {
				mediaCache[id] = v
			}
		}
	}
	out := make([]rendering.ArchiveEntry, 0, limit)
	for _, row := range rows {
		if _, ok := exclude[row.ID]; ok {
			continue
		}
		ae := rendering.ArchiveEntry{
			ID:           row.ID,
			Title:        row.Title,
			Excerpt:      stringValue(row.Excerpt),
			URL:          row.RoutePath,
			PublishedAt:  formatEntryDate(row.FirstPublishedAt, r.siteSnap.TimezoneName, false),
			PublishedISO: formatEntryDate(row.FirstPublishedAt, r.siteSnap.TimezoneName, true),
		}
		if row.FeaturedMediaID.Valid {
			if mv, ok := mediaCache[row.FeaturedMediaID.String]; ok {
				ae.FeaturedImage = mv
			}
		}
		out = append(out, ae)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
