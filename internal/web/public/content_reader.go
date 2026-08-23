package public

import (
	"context"

	"github.com/kokosx/stratum/internal/content"
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
	// Delegate to the generic content.Repository so query semantics (limit clamp,
	// order, exclude handling, overfetch) are defined in one place. Rendering
	// never touches sqlc directly – it uses this host capability.
	repo := content.NewRepository(r.queries)
	entries, err := repo.QueryPublished(ctx, content.EntryQuery{
		ContentType: content.ContentTypeID(contentType),
		Limit:       limit,
		Offset:      offset,
		Order:       order,
		ExcludeIDs:  excludeIDs,
	})
	if err != nil {
		return nil, err
	}
	// Batch media for filtered entries (repository already filtered).
	featIDs := make([]string, 0)
	for _, e := range entries {
		if e.FeaturedMediaID.Valid && e.FeaturedMediaID.String != "" {
			featIDs = append(featIDs, e.FeaturedMediaID.String)
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
	for _, e := range entries {
		ae := rendering.ArchiveEntry{
			ID:           e.ID,
			Title:        e.Title,
			Excerpt:      e.Excerpt,
			URL:          e.RoutePath,
			PublishedAt:  formatEntryDate(e.FirstPublishedAt, r.siteSnap.TimezoneName, false),
			PublishedISO: formatEntryDate(e.FirstPublishedAt, r.siteSnap.TimezoneName, true),
		}
		if e.FeaturedMediaID.Valid {
			if mv, ok := mediaCache[e.FeaturedMediaID.String]; ok {
				ae.FeaturedImage = mv
			}
		}
		out = append(out, ae)
	}
	return out, nil
}
