-- name: CreateComment :exec
INSERT INTO comments (id, entry_id, parent_id, status, author_name, author_email, author_url, user_id, body, created_at, updated_at, imported_source, imported_external_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetComment :one
SELECT * FROM comments WHERE id = ? LIMIT 1;

-- name: GetCommentByImportID :one
SELECT * FROM comments WHERE imported_source = ? AND imported_external_id = ? LIMIT 1;

-- name: ListApprovedCommentsByEntry :many
SELECT * FROM comments WHERE entry_id = ? AND status = 'approved' ORDER BY created_at ASC, id ASC LIMIT ? OFFSET ?;

-- name: CountApprovedCommentsByEntry :one
SELECT COUNT(*) FROM comments WHERE entry_id = ? AND status = 'approved';

-- name: ListCommentsByEntry :many
SELECT * FROM comments WHERE entry_id = ? ORDER BY created_at ASC, id ASC;

-- name: ListCommentsFiltered :many
SELECT * FROM comments
WHERE (sqlc.narg(status) IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(search) IS NULL OR author_name LIKE '%' || sqlc.narg(search) || '%' OR author_email LIKE '%' || sqlc.narg(search) || '%' OR body LIKE '%' || sqlc.narg(search) || '%')
  AND (sqlc.narg(entry_id) IS NULL OR entry_id = sqlc.narg(entry_id))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountCommentsFiltered :one
SELECT COUNT(*) FROM comments
WHERE (sqlc.narg(status) IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(search) IS NULL OR author_name LIKE '%' || sqlc.narg(search) || '%' OR author_email LIKE '%' || sqlc.narg(search) || '%' OR body LIKE '%' || sqlc.narg(search) || '%')
  AND (sqlc.narg(entry_id) IS NULL OR entry_id = sqlc.narg(entry_id));

-- name: CountCommentsByStatus :many
SELECT status, COUNT(*) as count FROM comments GROUP BY status;

-- name: UpdateCommentStatus :exec
UPDATE comments SET status = ?, updated_at = ? WHERE id = ?;

-- name: UpdateCommentParent :exec
UPDATE comments SET parent_id = ?, updated_at = ? WHERE id = ?;

-- name: DeleteComment :exec
DELETE FROM comments WHERE id = ?;

-- name: CountAllComments :one
SELECT COUNT(*) FROM comments;

-- name: SetCommentFeaturedMedia :exec
UPDATE entry_revisions SET featured_media_id = ? WHERE id = ?;
