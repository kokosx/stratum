-- name: GetEntryRevision :one
SELECT *
FROM entry_revisions
WHERE id = ?
LIMIT 1;

-- name: GetLatestEntryRevision :one
SELECT *
FROM entry_revisions
WHERE entry_id = ?
ORDER BY revision_number DESC
LIMIT 1;

-- name: ListEntryRevisions :many
SELECT *
FROM entry_revisions
WHERE entry_id = ?
ORDER BY revision_number DESC;

-- name: CreateEntryRevision :exec
INSERT INTO entry_revisions (
    id, entry_id, revision_number, title, excerpt, document_json,
    seo_title, seo_description, created_by, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
