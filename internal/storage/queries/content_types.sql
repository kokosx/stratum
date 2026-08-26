-- name: GetContentType :one
SELECT *
FROM content_types
WHERE id = ?
LIMIT 1;

-- name: ListContentTypes :many
SELECT *
FROM content_types
ORDER BY display_name;

-- name: CreateContentType :exec
INSERT INTO content_types (
    id, display_name, plural_name, hierarchical, public, config_json, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateContentType :exec
UPDATE content_types
SET display_name = ?, plural_name = ?, hierarchical = ?, public = ?, config_json = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteContentType :exec
DELETE FROM content_types
WHERE id = ?;
