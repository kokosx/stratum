-- name: GetBlockDefinition :one
SELECT *
FROM block_definitions
WHERE namespace = ?
  AND name = ?
  AND version = ?
LIMIT 1;

-- name: ListBlockDefinitions :many
SELECT *
FROM block_definitions
ORDER BY namespace, name, version;

-- name: CreateBlockDefinition :exec
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DisableBlockDefinition :exec
UPDATE block_definitions
SET enabled = 0, updated_at = ?
WHERE id = ?;
