package db

import (
	"context"
	"database/sql"
	"strings"
)

const listRoutes = `-- name: ListRoutes :many
SELECT id, path, entry_id, route_type, redirect_to, redirect_status, created_at, updated_at, content_type_id FROM routes ORDER BY path`

// ListRoutes returns all routes ordered by path.
func (q *Queries) ListRoutes(ctx context.Context) ([]Route, error) {
	rows, err := q.db.QueryContext(ctx, listRoutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Route
	for rows.Next() {
		var i Route
		if err := rows.Scan(
			&i.ID,
			&i.Path,
			&i.EntryID,
			&i.RouteType,
			&i.RedirectTo,
			&i.RedirectStatus,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.ContentTypeID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listMediaByIDsPrefix = `SELECT id, original_filename, storage_key, mime_type, asset_type, file_size, width, height, alt_text, title, caption, description, author_id, created_at, updated_at FROM media WHERE id IN (`
const listMediaVariantsByMediaIDsPrefix = `SELECT id, media_id, kind, storage_key, mime_type, width, height, file_size, created_at, content_hash FROM media_variants WHERE media_id IN (`

// ListMediaByIDs returns media rows for the given ids.
func (q *Queries) ListMediaByIDs(ctx context.Context, ids []string) ([]Medium, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Deduplicate to keep query small.
	seen := make(map[string]struct{}, len(ids))
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	placeholders := strings.Repeat("?,", len(uniq))
	placeholders = placeholders[:len(placeholders)-1]
	query := listMediaByIDsPrefix + placeholders + `) ORDER BY id`
	args := make([]interface{}, len(uniq))
	for i, id := range uniq {
		args[i] = id
	}
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Medium
	for rows.Next() {
		var i Medium
		if err := rows.Scan(
			&i.ID,
			&i.OriginalFilename,
			&i.StorageKey,
			&i.MimeType,
			&i.AssetType,
			&i.FileSize,
			&i.Width,
			&i.Height,
			&i.AltText,
			&i.Title,
			&i.Caption,
			&i.Description,
			&i.AuthorID,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListMediaVariantsByMediaIDs returns variants for the given media ids.
func (q *Queries) ListMediaVariantsByMediaIDs(ctx context.Context, mediaIDs []string) ([]MediaVariant, error) {
	if len(mediaIDs) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(mediaIDs))
	uniq := make([]string, 0, len(mediaIDs))
	for _, id := range mediaIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	placeholders := strings.Repeat("?,", len(uniq))
	placeholders = placeholders[:len(placeholders)-1]
	query := listMediaVariantsByMediaIDsPrefix + placeholders + `) ORDER BY media_id, kind`
	args := make([]interface{}, len(uniq))
	for i, id := range uniq {
		args[i] = id
	}
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []MediaVariant
	for rows.Next() {
		var i MediaVariant
		if err := rows.Scan(
			&i.ID,
			&i.MediaID,
			&i.Kind,
			&i.StorageKey,
			&i.MimeType,
			&i.Width,
			&i.Height,
			&i.FileSize,
			&i.CreatedAt,
			&i.ContentHash,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListAllMedia returns all media rows (for serve index rebuild).
func (q *Queries) ListAllMedia(ctx context.Context) ([]Medium, error) {
	const qstr = `SELECT id, original_filename, storage_key, mime_type, asset_type, file_size, width, height, alt_text, title, caption, description, author_id, created_at, updated_at FROM media ORDER BY id`
	rows, err := q.db.QueryContext(ctx, qstr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Medium
	for rows.Next() {
		var i Medium
		if err := rows.Scan(&i.ID, &i.OriginalFilename, &i.StorageKey, &i.MimeType, &i.AssetType, &i.FileSize, &i.Width, &i.Height, &i.AltText, &i.Title, &i.Caption, &i.Description, &i.AuthorID, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListAllMediaVariants returns all variants (for serve index rebuild).
func (q *Queries) ListAllMediaVariants(ctx context.Context) ([]MediaVariant, error) {
	const qstr = `SELECT id, media_id, kind, storage_key, mime_type, width, height, file_size, created_at, content_hash FROM media_variants ORDER BY media_id, kind`
	rows, err := q.db.QueryContext(ctx, qstr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []MediaVariant
	for rows.Next() {
		var i MediaVariant
		if err := rows.Scan(&i.ID, &i.MediaID, &i.Kind, &i.StorageKey, &i.MimeType, &i.Width, &i.Height, &i.FileSize, &i.CreatedAt, &i.ContentHash); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// Ensure sql import is used.
var _ = sql.ErrNoRows
