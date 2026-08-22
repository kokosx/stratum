-- name: CreateMedia :one
INSERT INTO media (
    id, original_filename, storage_key, mime_type, asset_type, file_size,
    width, height, alt_text, title, caption, description, author_id,
    created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = ?;

-- name: ListMedia :many
SELECT * FROM media ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: CountMedia :one
SELECT COUNT(*) FROM media;

-- name: UpdateMediaMetadata :exec
UPDATE media
SET alt_text = ?,
    title = ?,
    caption = ?,
    description = ?,
    updated_at = ?
WHERE id = ?;

-- name: DeleteMedia :exec
DELETE FROM media WHERE id = ?;

-- name: CreateMediaVariant :one
INSERT INTO media_variants (
    id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListMediaVariants :many
SELECT * FROM media_variants WHERE media_id = ? ORDER BY kind;

-- name: GetMediaVariant :one
SELECT * FROM media_variants WHERE media_id = ? AND kind = ?;

-- name: DeleteMediaVariant :exec
DELETE FROM media_variants WHERE id = ?;

-- name: CountMediaUsage :one
SELECT
    (
        SELECT COUNT(*) FROM entry_revisions
        WHERE document_json LIKE '%"mediaId":"' || sqlc.arg('id') || '"%'
    )
    + (
        SELECT COUNT(*) FROM site_settings
        WHERE site_icon_media_id = sqlc.arg('id')
           OR site_logo_media_id = sqlc.arg('id')
           OR site_social_media_id = sqlc.arg('id')
    )
    + (
        SELECT COUNT(*) FROM entry_revisions
        WHERE featured_media_id = sqlc.arg('id')
           OR social_media_id = sqlc.arg('id')
    )
AS usage_count;

-- name: GetSiteIconMediaID :one
SELECT site_icon_media_id FROM site_settings WHERE id = 1;

-- name: UpdateSiteIconMediaID :exec
UPDATE site_settings
SET site_icon_media_id = ?, updated_at = unixepoch()
WHERE id = 1;

-- name: GetSiteSocialMediaID :one
SELECT site_social_media_id FROM site_settings WHERE id = 1;

-- name: UpdateSiteSocialMediaID :exec
UPDATE site_settings
SET site_social_media_id = ?, updated_at = unixepoch()
WHERE id = 1;

-- name: GetTwitterSite :one
SELECT twitter_site FROM site_settings WHERE id = 1;

-- name: UpdateTwitterSite :exec
UPDATE site_settings
SET twitter_site = ?, updated_at = unixepoch()
WHERE id = 1;
