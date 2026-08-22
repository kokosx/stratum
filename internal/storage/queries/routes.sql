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
    id, path, entry_id, route_type, redirect_to, redirect_status, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateRoute :exec
UPDATE routes
SET path = ?, entry_id = ?, route_type = ?, redirect_to = ?, redirect_status = ?, updated_at = ?
WHERE id = ?;

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
