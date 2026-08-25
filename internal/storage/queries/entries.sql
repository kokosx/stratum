-- name: GetEntry :one
SELECT *
FROM entries
WHERE id = ?
LIMIT 1;

-- name: GetEntriesByIDs :many
SELECT *
FROM entries
WHERE id IN (sqlc.slice('ids'))
  AND content_type_id = sqlc.arg('content_type_id');

-- name: GetFlatEntryBySlug :one
-- NOTE: MUST NOT be used for hierarchical content types (e.g. page).
-- Public Page lookup must use routes (Route path -> Entry). This helper exists only
-- for flat content-type tests and non-hierarchical lookups; prefer route-based lookup in production.
SELECT *
FROM entries
WHERE content_type_id = ?
  AND slug = ?
LIMIT 1;

-- name: ListEntriesByContentType :many
SELECT
    entries.id,
    COALESCE(NULLIF(latest_revision.slug, ''), entries.slug) AS slug,
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

-- name: UpdateEntryProjection :exec
UPDATE entries
SET slug = ?, status = ?, updated_at = ?, published_at = ?
WHERE id = ?;

-- name: SetPublishedRevision :exec
UPDATE entries
SET published_revision_id = ?, status = 'active', published_at = ?, updated_at = ?
WHERE id = ?;

-- name: ClearPublishedRevision :exec
UPDATE entries
SET published_revision_id = NULL, published_at = NULL, updated_at = ?
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

-- name: MoveEntryToTrash :exec
UPDATE entries
SET status = 'trash',
    status_before_trash = CASE WHEN status IN ('active', 'private') THEN status ELSE 'active' END,
    trashed_at = ?,
    updated_at = ?
WHERE id = ?
  AND status != 'trash';

-- name: RestoreEntryFromTrash :exec
UPDATE entries
SET status = COALESCE(status_before_trash, 'active'),
    status_before_trash = NULL,
    trashed_at = NULL,
    updated_at = ?
WHERE id = ?
  AND status = 'trash';

-- name: DeleteRoutesByEntryID :exec
DELETE FROM routes
WHERE entry_id = ?;

-- name: ListEntriesAdmin :many
SELECT
    entries.id,
    COALESCE(NULLIF(latest_revision.slug, ''), entries.slug) AS slug,
    entries.status,
    entries.updated_at,
    entries.published_revision_id,
    latest_revision.id AS latest_revision_id,
    latest_revision.title,
    latest_revision.review_state AS latest_review_state,
    published_revision.visibility AS published_visibility,
    published_revision.review_state AS published_review_state,
    active_job.scheduled_at AS scheduled_at,
    CASE WHEN active_job.id IS NOT NULL THEN 1 ELSE 0 END AS has_schedule,
    public_route.path AS public_path
FROM entries
LEFT JOIN entry_revisions AS latest_revision
    ON latest_revision.entry_id = entries.id
    AND latest_revision.revision_number = (
        SELECT MAX(revision_number)
        FROM entry_revisions
        WHERE entry_id = entries.id
    )
LEFT JOIN entry_revisions AS published_revision
    ON published_revision.id = entries.published_revision_id
LEFT JOIN publication_jobs AS active_job
    ON active_job.entry_id = entries.id AND active_job.status = 'scheduled'
LEFT JOIN routes AS public_route
    ON public_route.id = (
        SELECT id
        FROM routes
        WHERE entry_id = entries.id
          AND route_type = 'entry'
        ORDER BY path
        LIMIT 1
    )
WHERE entries.content_type_id = sqlc.arg('content_type_id')
	AND (sqlc.narg('author_id') IS NULL OR entries.author_id = sqlc.narg('author_id'))
   AND (
      sqlc.arg('status_filter') = ''
      OR (
          sqlc.arg('status_filter') = 'published' AND entries.status != 'trash' AND entries.published_revision_id IS NOT NULL AND published_revision.visibility IN ('public','password')
      )
      OR (
          sqlc.arg('status_filter') = 'draft' AND entries.status != 'trash' AND entries.published_revision_id IS NULL AND latest_revision.review_state = 'draft' AND active_job.id IS NULL
      )
      OR (
          sqlc.arg('status_filter') = 'pending' AND entries.status != 'trash' AND latest_revision.review_state = 'pending' AND latest_revision.id != COALESCE(entries.published_revision_id,'')
      )
      OR (
          sqlc.arg('status_filter') = 'scheduled' AND entries.status != 'trash' AND active_job.id IS NOT NULL
      )
      OR (
          sqlc.arg('status_filter') = 'private' AND entries.status != 'trash' AND published_revision.visibility = 'private'
      )
      OR (
          sqlc.arg('status_filter') = 'trash' AND entries.status = 'trash'
      )
      OR (
          sqlc.arg('status_filter') = 'all' AND entries.status != 'trash'
      )
  )
  AND (
      sqlc.arg('search') = ''
      OR latest_revision.title LIKE '%' || sqlc.arg('search') || '%'
       OR COALESCE(NULLIF(latest_revision.slug, ''), entries.slug) LIKE '%' || sqlc.arg('search') || '%'
  )
ORDER BY entries.updated_at DESC, entries.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountEntriesAdmin :one
SELECT COUNT(*)
FROM entries
LEFT JOIN entry_revisions AS latest_revision
    ON latest_revision.entry_id = entries.id
    AND latest_revision.revision_number = (
        SELECT MAX(revision_number)
        FROM entry_revisions
        WHERE entry_id = entries.id
    )
LEFT JOIN entry_revisions AS published_revision
    ON published_revision.id = entries.published_revision_id
LEFT JOIN publication_jobs AS active_job
    ON active_job.entry_id = entries.id AND active_job.status = 'scheduled'
WHERE entries.content_type_id = sqlc.arg('content_type_id')
	AND (sqlc.narg('author_id') IS NULL OR entries.author_id = sqlc.narg('author_id'))
   AND (
      sqlc.arg('status_filter') = ''
      OR (
          sqlc.arg('status_filter') = 'published' AND entries.status != 'trash' AND entries.published_revision_id IS NOT NULL AND published_revision.visibility IN ('public','password')
      )
      OR (
          sqlc.arg('status_filter') = 'draft' AND entries.status != 'trash' AND entries.published_revision_id IS NULL AND latest_revision.review_state = 'draft' AND active_job.id IS NULL
      )
      OR (
          sqlc.arg('status_filter') = 'pending' AND entries.status != 'trash' AND latest_revision.review_state = 'pending' AND latest_revision.id != COALESCE(entries.published_revision_id,'')
      )
      OR (
          sqlc.arg('status_filter') = 'scheduled' AND entries.status != 'trash' AND active_job.id IS NOT NULL
      )
      OR (
          sqlc.arg('status_filter') = 'private' AND entries.status != 'trash' AND published_revision.visibility = 'private'
      )
      OR (
          sqlc.arg('status_filter') = 'trash' AND entries.status = 'trash'
      )
      OR (
          sqlc.arg('status_filter') = 'all' AND entries.status != 'trash'
      )
  )
  AND (
      sqlc.arg('search') = ''
      OR latest_revision.title LIKE '%' || sqlc.arg('search') || '%'
       OR COALESCE(NULLIF(latest_revision.slug, ''), entries.slug) LIKE '%' || sqlc.arg('search') || '%'
  );

-- name: CountEntriesByAdminStatus :one
SELECT
    SUM(CASE WHEN entries.status != 'trash' THEN 1 ELSE 0 END) AS all_count,
    SUM(CASE WHEN entries.status != 'trash' AND entries.published_revision_id IS NOT NULL AND published_revision.visibility IN ('public','password') THEN 1 ELSE 0 END) AS published_count,
    SUM(CASE WHEN entries.status != 'trash' AND entries.published_revision_id IS NULL AND latest_revision.review_state = 'draft' AND active_job.id IS NULL THEN 1 ELSE 0 END) AS draft_count,
    SUM(CASE WHEN entries.status != 'trash' AND published_revision.visibility = 'private' THEN 1 ELSE 0 END) AS private_count,
    SUM(CASE WHEN latest_revision.review_state = 'pending' AND latest_revision.id != COALESCE(entries.published_revision_id,'') AND entries.status != 'trash' THEN 1 ELSE 0 END) AS pending_count,
    SUM(CASE WHEN active_job.id IS NOT NULL AND entries.status != 'trash' THEN 1 ELSE 0 END) AS scheduled_count,
    SUM(CASE WHEN entries.status = 'trash' THEN 1 ELSE 0 END) AS trash_count
FROM entries
LEFT JOIN entry_revisions AS latest_revision
    ON latest_revision.entry_id = entries.id
    AND latest_revision.revision_number = (
        SELECT MAX(revision_number)
        FROM entry_revisions
        WHERE entry_id = entries.id
    )
LEFT JOIN entry_revisions AS published_revision
    ON published_revision.id = entries.published_revision_id
LEFT JOIN publication_jobs AS active_job
    ON active_job.entry_id = entries.id AND active_job.status = 'scheduled'
WHERE entries.content_type_id = sqlc.arg('content_type_id')
  AND (sqlc.narg('author_id') IS NULL OR entries.author_id = sqlc.narg('author_id'))
;

-- name: SetImportedPublishedDates :exec
-- Historical date preservation for imported published entries ONLY.
UPDATE entries SET created_at = ?, updated_at = ?, published_at = ?, first_published_at = ? WHERE id = ?;
