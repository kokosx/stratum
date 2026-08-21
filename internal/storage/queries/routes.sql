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
