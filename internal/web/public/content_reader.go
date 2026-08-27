package public

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
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

func (r *handlerContentReader) Definition(ctx context.Context, contentType string) (content.ContentTypeDefinition, error) {
	return content.NewCatalog(r.queries).GetDefinition(ctx, contentType)
}

// handlerSitePartReader implements rendering.SitePartReader via published site part revisions.
type handlerSitePartReader struct {
	queries *db.Queries
	blocks  *blocks.Registry
}

func (r *handlerSitePartReader) GetSitePart(ctx context.Context, id string) (*rendering.PreparedDocument, string, error) {
	row, err := r.queries.GetSitePartWithPublishedRevision(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", errors.New("site part not found or unpublished")
		}
		return nil, "", err
	}
	doc, err := document.Decode([]byte(row.DocumentJson))
	if err != nil {
		return nil, "", err
	}
	pd, err := r.blocks.PreparedCache(row.RevisionID, doc)
	if err != nil {
		pd, err = r.blocks.Prepare(doc)
		if err != nil {
			return nil, "", err
		}
	}
	return pd, row.RevisionID, nil
}

func (r *handlerContentReader) Query(ctx context.Context, query content.EntryQuery) ([]rendering.ArchiveEntry, error) {
	// Delegate to the generic content.Repository so query semantics (limit clamp,
	// order, exclude handling, overfetch) are defined in one place. Rendering
	// never touches sqlc directly – it uses this host capability.
	repo := content.NewRepository(r.queries)
	entries, err := repo.QueryPublished(ctx, query)
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
		if batcher, ok := r.media.(interface {
			MediaViews(ctx context.Context, ids []string) map[string]rendering.MediaView
		}); ok {
			mediaCache = batcher.MediaViews(ctx, featIDs)
			if mediaCache == nil {
				mediaCache = map[string]rendering.MediaView{}
			}
		} else {
			for _, id := range featIDs {
				if v, ok := r.media.MediaView(ctx, id); ok {
					mediaCache[id] = v
				}
			}
		}
	}
	out := make([]rendering.ArchiveEntry, 0, query.Limit)
	for _, e := range entries {
		fields, err := content.DecodeFieldSnapshot(e.FieldsJSON)
		if err != nil {
			return nil, err
		}
		ae := rendering.ArchiveEntry{
			ID:            e.ID,
			Slug:          e.Slug,
			ContentTypeID: string(e.ContentTypeID),
			Title:         e.Title,
			Excerpt:       e.Excerpt,
			URL:           e.RoutePath,
			PublishedAt:   formatEntryDate(e.FirstPublishedAt, r.siteSnap.TimezoneName, false),
			PublishedISO:  formatEntryDate(e.FirstPublishedAt, r.siteSnap.TimezoneName, true),
			Fields:        fields,
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
