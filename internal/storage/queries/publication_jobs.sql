-- name: CreatePublicationJob :exec
INSERT INTO publication_jobs (id, entry_id, revision_id, scheduled_at, status, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, 'scheduled', ?, ?, ?);

-- name: GetPublicationJob :one
SELECT * FROM publication_jobs WHERE id = ? LIMIT 1;

-- name: GetActivePublicationJobByEntry :one
SELECT * FROM publication_jobs WHERE entry_id = ? AND status = 'scheduled' LIMIT 1;

-- name: ListDuePublicationJobs :many
SELECT * FROM publication_jobs
WHERE status = 'scheduled' AND scheduled_at <= ?
ORDER BY scheduled_at ASC;

-- name: UpdatePublicationJobStatus :exec
UPDATE publication_jobs SET status = sqlc.arg(status), updated_at = sqlc.arg(updated_at), last_error = sqlc.narg(last_error) WHERE id = sqlc.arg(id);

-- name: CancelActivePublicationJobsForEntry :exec
UPDATE publication_jobs SET status = 'cancelled', updated_at = sqlc.arg(updated_at), last_error = sqlc.narg(last_error) WHERE entry_id = sqlc.arg(entry_id) AND status = 'scheduled';

-- name: DeletePublicationJobsForEntry :exec
DELETE FROM publication_jobs WHERE entry_id = ?;

-- name: ListPublicationJobsForEntry :many
SELECT * FROM publication_jobs WHERE entry_id = ? ORDER BY scheduled_at DESC;

-- name: GetPublicationJobByEntryAndRevision :one
SELECT * FROM publication_jobs WHERE entry_id = ? AND revision_id = ? AND status = 'scheduled' LIMIT 1;
