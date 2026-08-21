-- name: GetEntry :one
SELECT *
FROM entries
WHERE id = ?
LIMIT 1;

-- name: GetEntryBySlug :one
SELECT *
FROM entries
WHERE content_type_id = ?
  AND slug = ?
LIMIT 1;

-- name: ListEntriesByContentType :many
SELECT *
FROM entries
WHERE content_type_id = ?
  AND status = ?
ORDER BY published_at DESC, created_at DESC
LIMIT ? OFFSET ?;

-- name: CreateEntry :exec
INSERT INTO entries (
    id, content_type_id, slug, status, author_id, created_at, updated_at, published_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateEntry :exec
UPDATE entries
SET slug = ?, status = ?, author_id = ?, updated_at = ?, published_at = ?
WHERE id = ?;

-- name: SetPublishedRevision :exec
UPDATE entries
SET published_revision_id = ?, status = 'active', published_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteEntry :exec
DELETE FROM entries
WHERE id = ?;
