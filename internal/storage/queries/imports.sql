-- name: CreateImportRun :exec
INSERT INTO import_runs (id, source, created_at) VALUES (?, ?, ?);

-- name: CompleteImportRun :exec
UPDATE import_runs SET completed_at = ? WHERE id = ?;

-- name: GetImportMapping :one
SELECT internal_id FROM import_mappings WHERE source = ? AND object_type = ? AND external_id = ?;

-- name: CreateImportMapping :exec
INSERT INTO import_mappings (source, object_type, external_id, internal_id, run_id, created_at)
VALUES (?, ?, ?, ?, ?, ?);
