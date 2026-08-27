-- name: CreateSitePart :exec
INSERT INTO site_parts (id, name, published_revision_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSitePart :one
SELECT *
FROM site_parts
WHERE id = ?
LIMIT 1;

-- name: ListSiteParts :many
SELECT *
FROM site_parts
ORDER BY
    CASE WHEN published_revision_id IS NULL THEN 1 ELSE 0 END,
    name,
    id;

-- name: UpdateSitePart :exec
UPDATE site_parts
SET name = ?, updated_at = ?
WHERE id = ?;

-- name: SetSitePartPublishedRevision :exec
UPDATE site_parts
SET published_revision_id = ?, updated_at = ?
WHERE id = ?;

-- name: ClearSitePartPublishedRevision :exec
UPDATE site_parts
SET published_revision_id = NULL, updated_at = ?
WHERE id = ?;

-- name: DeleteSitePart :exec
DELETE FROM site_parts
WHERE id = ?;

-- name: GetSitePartRevision :one
SELECT *
FROM site_part_revisions
WHERE id = ?
LIMIT 1;

-- name: GetLatestSitePartRevision :one
SELECT *
FROM site_part_revisions
WHERE site_part_id = ?
ORDER BY revision_number DESC
LIMIT 1;

-- name: ListSitePartRevisions :many
SELECT *
FROM site_part_revisions
WHERE site_part_id = ?
ORDER BY revision_number DESC;

-- name: GetPublishedSitePartRevision :one
SELECT r.*
FROM site_parts p
JOIN site_part_revisions r ON r.id = p.published_revision_id
WHERE p.id = ?
LIMIT 1;

-- name: CreateSitePartRevision :exec
INSERT INTO site_part_revisions (id, site_part_id, revision_number, document_json, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetSitePartWithPublishedRevision :one
SELECT
    p.id,
    p.name,
    p.published_revision_id,
    p.created_at,
    p.updated_at,
    r.id AS revision_id,
    r.document_json
FROM site_parts p
JOIN site_part_revisions r ON r.id = p.published_revision_id
WHERE p.id = ?
LIMIT 1;

-- name: ListLatestSitePartRevisions :many
SELECT *
FROM site_part_revisions
WHERE id IN (
    SELECT id FROM site_part_revisions
    WHERE (site_part_id, revision_number) IN (
        SELECT site_part_id, MAX(revision_number) FROM site_part_revisions GROUP BY site_part_id
    )
);

-- name: GetSitePartLocation :one
SELECT *
FROM site_part_locations
WHERE location = ?
LIMIT 1;

-- name: ListSitePartLocations :many
SELECT *
FROM site_part_locations;

-- name: SetSitePartLocation :exec
INSERT INTO site_part_locations (location, site_part_id, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(location) DO UPDATE SET site_part_id = excluded.site_part_id, updated_at = excluded.updated_at;

-- name: ClearSitePartLocation :exec
DELETE FROM site_part_locations
WHERE location = ?;

-- name: GetSitePartsByPublishedID :many
SELECT *
FROM site_parts
WHERE published_revision_id IS NOT NULL;
