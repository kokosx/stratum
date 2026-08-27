package content

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

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
	if len(q.Filters) > 0 || q.OrderBy != "" {
		// Validate field existence and operator compatibility against the effective definition.
		if def, err := NewCatalog(r.queries).GetDefinition(ctx, string(q.ContentType)); err == nil {
			if err := q.ValidateForDefinition(def); err != nil {
				return nil, err
			}
		} else {
			// Fallback to builtin definitions for cases where DB has no row (pre-catalog).
			def := DefinitionFor(string(q.ContentType))
			if err := q.ValidateForDefinition(def); err != nil {
				return nil, err
			}
		}
		return r.queryPublishedAdvanced(ctx, q)
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

func (r *Repository) queryPublishedAdvanced(ctx context.Context, q EntryQuery) ([]PublishedEntry, error) {
	// SQLite JSON snapshots are acceptable for the current CMS scale. Fetch a
	// bounded published projection and evaluate only host-validated refs/operators.
	// The join remains pinned to entries.published_revision_id, so drafts cannot leak.
	// We use bounded batches and fetch the entire matching set so sorting/filtering
	// is globally correct (no LIMIT 1000 truncation).
	const batchSize = 500
	entries := make([]PublishedEntry, 0)
	if q.TermID != "" {
		for offset := int64(0); ; offset += batchSize {
			rows, err := r.queries.ListPublishedEntriesByTerm(ctx, db.ListPublishedEntriesByTermParams{TermID: q.TermID, ContentTypeID: string(q.ContentType), Limit: batchSize, Offset: offset})
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				if contains(q.ExcludeIDs, row.ID) {
					continue
				}
				entries = append(entries, PublishedEntry{ID: row.ID, Slug: row.Slug, ContentTypeID: q.ContentType, Title: row.Title, Excerpt: sqlNullToString(row.Excerpt), FeaturedMediaID: row.FeaturedMediaID, RoutePath: row.RoutePath, PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID, FieldsJSON: row.FieldsJson})
			}
			if int64(len(rows)) < batchSize {
				break
			}
		}
	} else {
		for offset := int64(0); ; offset += batchSize {
			rows, err := r.queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: string(q.ContentType), Limit: batchSize, Offset: offset})
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				if contains(q.ExcludeIDs, row.ID) {
					continue
				}
				entries = append(entries, PublishedEntry{ID: row.ID, Slug: row.Slug, ContentTypeID: q.ContentType, Title: row.Title, Excerpt: sqlNullToString(row.Excerpt), FeaturedMediaID: row.FeaturedMediaID, RoutePath: row.RoutePath, PublishedAt: row.PublishedAt, FirstPublishedAt: row.FirstPublishedAt, RevisionID: row.RevisionID, FieldsJSON: row.FieldsJson})
			}
			if int64(len(rows)) < batchSize {
				break
			}
		}
	}
	// TODO: future SQLite json_extract indexing could push filtering/sorting into SQL.
	// For now we scan revision snapshots in Go with bounded batches, guaranteeing correctness.
	filtered := entries[:0]
	for _, entry := range entries {
		ok, err := matchesFilters(entry, q.Filters)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, entry)
		}
	}
	entries = filtered
	if q.OrderBy != "" {
		sort.SliceStable(entries, func(i, j int) bool { return lessPublished(entries[i], entries[j], q.OrderBy, q.Direction) })
	}
	start := q.Offset
	if start > len(entries) {
		start = len(entries)
	}
	end := start + q.Limit
	if end > len(entries) {
		end = len(entries)
	}
	return entries[start:end], nil
}

func publishedValue(entry PublishedEntry, field string) (any, bool, error) {
	ref, err := ParseFieldRef(field)
	if err != nil {
		return nil, false, err
	}
	fields, err := DecodeFieldSnapshot(entry.FieldsJSON)
	if err != nil {
		return nil, false, err
	}
	published := ""
	if entry.FirstPublishedAt.Valid {
		published = fmt.Sprint(entry.FirstPublishedAt.Int64)
	} else if entry.PublishedAt.Valid {
		published = fmt.Sprint(entry.PublishedAt.Int64)
	}
	value, ok := ResolveEntryField(ref, entry.Title, entry.Excerpt, entry.RoutePath, published, entry.FeaturedMediaID.String, fields)
	return value, ok, nil
}
func matchesFilters(entry PublishedEntry, filters []EntryFilter) (bool, error) {
	for _, filter := range filters {
		value, exists, err := publishedValue(entry, filter.Field)
		if err != nil {
			return false, err
		}
		switch filter.Operator {
		case OpExists:
			if !exists || emptyQueryValue(value) {
				return false, nil
			}
			continue
		case OpNotExists:
			if exists && !emptyQueryValue(value) {
				return false, nil
			}
			continue
		case OpIsTrue:
			b, ok := value.(bool)
			if !exists || !ok || !b {
				return false, nil
			}
			continue
		case OpIsFalse:
			b, ok := value.(bool)
			if !exists || !ok || b {
				return false, nil
			}
			continue
		}
		if !exists {
			return false, nil
		}
		cmp := compareQueryValues(value, filter.Value)
		switch filter.Operator {
		case OpEquals:
			if cmp != 0 {
				return false, nil
			}
		case OpNotEquals:
			if cmp == 0 {
				return false, nil
			}
		case OpContains:
			if !strings.Contains(strings.ToLower(fmt.Sprint(value)), strings.ToLower(fmt.Sprint(filter.Value))) {
				return false, nil
			}
		case OpGreater, OpAfter:
			if cmp <= 0 {
				return false, nil
			}
		case OpGreaterEqual:
			if cmp < 0 {
				return false, nil
			}
		case OpLess, OpBefore:
			if cmp >= 0 {
				return false, nil
			}
		case OpLessEqual:
			if cmp > 0 {
				return false, nil
			}
		}
	}
	return true, nil
}
func emptyQueryValue(v any) bool {
	s, ok := v.(string)
	return v == nil || (ok && strings.TrimSpace(s) == "")
}
func compareQueryValues(a, b any) int {
	an, ae := numberValue(a)
	bn, be := numberValue(b)
	if ae == nil && be == nil {
		if an < bn {
			return -1
		}
		if an > bn {
			return 1
		}
		return 0
	}
	as, bs := fmt.Sprint(a), fmt.Sprint(b)
	if as < bs {
		return -1
	}
	if as > bs {
		return 1
	}
	return 0
}
func lessPublished(a, b PublishedEntry, field, direction string) bool {
	av, aok, _ := publishedValue(a, field)
	bv, bok, _ := publishedValue(b, field)
	if aok != bok {
		return aok
	}
	cmp := compareQueryValues(av, bv)
	if cmp == 0 {
		return a.ID < b.ID
	}
	if direction == "asc" {
		return cmp < 0
	}
	return cmp > 0
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
