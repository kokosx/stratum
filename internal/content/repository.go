package content

import (
	"context"
	"database/sql"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Repository is the storage boundary for entries. It maps sqlc rows to domain
// types and centralises the few stable query shapes so prepared statements can
// be reused. It does NOT expose *sql.DB or sqlc models to callers outside storage.
type Repository struct {
	queries *db.Queries
}

// NewRepository creates a repository.
func NewRepository(queries *db.Queries) *Repository { return &Repository{queries: queries} }

// Get returns a single entry by ID.
func (r *Repository) Get(ctx context.Context, id string) (Entry, error) {
	row, err := r.queries.GetEntry(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		ID: row.ID, ContentTypeID: ContentTypeID(row.ContentTypeID), Slug: row.Slug,
		Status: row.Status, AuthorID: row.AuthorID, PublishedRevisionID: row.PublishedRevisionID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, PublishedAt: row.PublishedAt,
		FirstPublishedAt: row.FirstPublishedAt,
	}, nil
}

// QueryPublished executes a normalized EntryQuery against stable prepared shapes.
// If TermID is set, it uses the term-filtered variant that JOINs via published_revision_id.
func (r *Repository) QueryPublished(ctx context.Context, q EntryQuery) ([]PublishedEntry, error) {
	q = q.Normalized()
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if q.TermID != "" {
		if q.Order == "published_asc" {
			rows, err := r.queries.ListPublishedEntriesByTermAsc(ctx, db.ListPublishedEntriesByTermAscParams{
				TermID:        q.TermID,
				ContentTypeID: string(q.ContentType),
				Limit:         int64(q.Limit + len(q.ExcludeIDs) + 5),
				Offset:        int64(q.Offset),
			})
			if err != nil {
				return nil, err
			}
			out := make([]PublishedEntry, 0, len(rows))
			for _, row := range rows {
				if contains(q.ExcludeIDs, row.ID) {
					continue
				}
				out = append(out, PublishedEntry{
					ID: row.ID, Slug: row.Slug, ContentTypeID: ContentTypeID(string(q.ContentType)),
					Title: row.Title, Excerpt: sqlNullToString(row.Excerpt),
					FeaturedMediaID: row.FeaturedMediaID, RoutePath: row.RoutePath,
					PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID, FieldsJSON: row.FieldsJson,
				})
				if len(out) >= q.Limit {
					break
				}
			}
			return out, nil
		}
		rows, err := r.queries.ListPublishedEntriesByTerm(ctx, db.ListPublishedEntriesByTermParams{
			TermID:        q.TermID,
			ContentTypeID: string(q.ContentType),
			Limit:         int64(q.Limit + len(q.ExcludeIDs) + 5),
			Offset:        int64(q.Offset),
		})
		if err != nil {
			return nil, err
		}
		out := make([]PublishedEntry, 0, len(rows))
		for _, row := range rows {
			if contains(q.ExcludeIDs, row.ID) {
				continue
			}
			out = append(out, PublishedEntry{
				ID: row.ID, Slug: row.Slug, ContentTypeID: ContentTypeID(string(q.ContentType)),
				Title: row.Title, Excerpt: sqlNullToString(row.Excerpt),
				FeaturedMediaID: row.FeaturedMediaID, RoutePath: row.RoutePath,
				PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID, FieldsJSON: row.FieldsJson,
			})
			if len(out) >= q.Limit {
				break
			}
		}
		return out, nil
	}
	if q.Order == "published_asc" {
		rows, err := r.queries.ListPublishedEntriesByContentTypeAsc(ctx, db.ListPublishedEntriesByContentTypeAscParams{
			ContentTypeID: string(q.ContentType),
			Limit:         int64(q.Limit + len(q.ExcludeIDs) + 5),
			Offset:        int64(q.Offset),
		})
		if err != nil {
			return nil, err
		}
		out := make([]PublishedEntry, 0, len(rows))
		for _, row := range rows {
			if contains(q.ExcludeIDs, row.ID) {
				continue
			}
			out = append(out, PublishedEntry{
				ID: row.ID, Slug: row.Slug, ContentTypeID: ContentTypeID(string(q.ContentType)),
				Title: row.Title, Excerpt: sqlNullToString(row.Excerpt),
				FeaturedMediaID: row.FeaturedMediaID, RoutePath: row.RoutePath,
				PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID, FieldsJSON: row.FieldsJson,
			})
			if len(out) >= q.Limit {
				break
			}
		}
		return out, nil
	}
	rows, err := r.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{
		ContentTypeID: string(q.ContentType),
		Limit:         int64(q.Limit + len(q.ExcludeIDs) + 5),
		Offset:        int64(q.Offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]PublishedEntry, 0, len(rows))
	for _, row := range rows {
		if contains(q.ExcludeIDs, row.ID) {
			continue
		}
		out = append(out, PublishedEntry{
			ID: row.ID, Slug: row.Slug, ContentTypeID: ContentTypeID(string(q.ContentType)),
			Title: row.Title, Excerpt: sqlNullToString(row.Excerpt),
			FeaturedMediaID: row.FeaturedMediaID, RoutePath: row.RoutePath,
			PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID, FieldsJSON: row.FieldsJson,
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

// CountPublished returns the count of published entries for a type.
func (r *Repository) CountPublished(ctx context.Context, ct ContentTypeID) (int64, error) {
	return r.queries.CountPublishedEntriesByContentType(ctx, string(ct))
}

// CountPublishedByTerm returns count for term-filtered published entries.
func (r *Repository) CountPublishedByTerm(ctx context.Context, ct ContentTypeID, termID string) (int64, error) {
	return r.queries.ListPublishedEntriesByTermCount(ctx, db.ListPublishedEntriesByTermCountParams{TermID: termID, ContentTypeID: string(ct)})
}

func contains(ids []string, needle string) bool {
	for _, id := range ids {
		if id == needle {
			return true
		}
	}
	return false
}

func sqlNullToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
