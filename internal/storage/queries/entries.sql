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
SELECT
    entries.id,
    entries.slug,
    entries.status,
    entries.updated_at,
    entries.published_revision_id,
    latest_revision.title,
    public_route.path AS public_path
FROM entries
LEFT JOIN entry_revisions AS latest_revision
    ON latest_revision.entry_id = entries.id
    AND latest_revision.revision_number = (
        SELECT MAX(revision_number)
        FROM entry_revisions
        WHERE entry_id = entries.id
    )
LEFT JOIN routes AS public_route
    ON public_route.id = (
        SELECT id
        FROM routes
        WHERE entry_id = entries.id
          AND route_type = 'entry'
        ORDER BY path
        LIMIT 1
    )
WHERE entries.content_type_id = ?
ORDER BY entries.updated_at DESC, entries.created_at DESC;

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

-- name: SetFirstPublishedAtIfNull :exec
-- Records the FIRST publication of an Entry. Later re-publishes must never
-- move it: structured data uses it as the stable datePublished.
UPDATE entries
SET first_published_at = ?
WHERE id = ?
  AND first_published_at IS NULL;

-- name: DeleteEntry :exec
DELETE FROM entries
WHERE id = ?;
