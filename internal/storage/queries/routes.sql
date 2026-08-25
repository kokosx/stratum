-- name: GetRouteByPath :one
SELECT *
FROM routes
WHERE path = ?
LIMIT 1;

-- name: ListRoutesForEntry :many
SELECT *
FROM routes
WHERE entry_id = ?
ORDER BY path;

-- name: GetEntryRoute :one
SELECT *
FROM routes
WHERE entry_id = ?
  AND route_type = 'entry'
ORDER BY path
LIMIT 1;

-- name: CreateRoute :exec
INSERT INTO routes (
    id, path, entry_id, route_type, content_type_id, taxonomy_id, term_id, redirect_to, redirect_status, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateRoute :exec
UPDATE routes
SET path = ?, entry_id = ?, route_type = ?, content_type_id = ?, taxonomy_id = ?, term_id = ?, redirect_to = ?, redirect_status = ?, updated_at = ?
WHERE id = ?;

-- name: GetArchiveRouteByContentType :one
SELECT *
FROM routes
WHERE route_type = 'archive' AND content_type_id = ?
LIMIT 1;

-- name: ListArchiveRoutes :many
SELECT *
FROM routes
WHERE route_type = 'archive'
ORDER BY path;

-- name: GetRouteByPathAndType :one
SELECT *
FROM routes
WHERE path = ? AND route_type = ?
LIMIT 1;

-- name: DeleteRoute :exec
DELETE FROM routes
WHERE id = ?;

-- name: ListRedirectsToTarget :many
-- Every redirect route whose target is the given path. Slug changes use this to
-- flatten redirect chains: when /a -> /b exists and /b moves to /c, this query
-- finds /a so it can be retargeted straight to /c.
SELECT *
FROM routes
WHERE route_type = 'redirect'
  AND redirect_to = ?;

-- name: ListRoutes :many
SELECT * FROM routes ORDER BY path;

-- name: GetTermArchiveRoute :one
SELECT * FROM routes WHERE taxonomy_id = ? AND term_id = ? AND route_type = 'archive' LIMIT 1;

-- name: GetRouteByTaxonomyTerm :one
SELECT * FROM routes WHERE taxonomy_id = ? AND term_id = ? LIMIT 1;

-- name: ListTaxonomyArchiveRoutes :many
SELECT * FROM routes WHERE route_type = 'archive' AND taxonomy_id IS NOT NULL ORDER BY path;

-- name: ListEntryRouteVisibilities :many
SELECT entries.id AS entry_id, entry_revisions.visibility AS visibility, entries.published_revision_id AS published_revision_id
FROM entries
JOIN entry_revisions ON entry_revisions.id = entries.published_revision_id
WHERE entries.published_revision_id IS NOT NULL;
