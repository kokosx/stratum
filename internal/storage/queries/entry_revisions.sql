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

-- name: ListLatestHierarchyForContentType :many
SELECT e.id AS entry_id, e.content_type_id, COALESCE(NULLIF(r.slug, ''), e.slug) AS slug, e.status, r.title,
       r.parent_entry_id, r.menu_order
FROM entries e
JOIN entry_revisions r ON r.id = (
    SELECT latest.id FROM entry_revisions latest
    WHERE latest.entry_id = e.id
    ORDER BY latest.revision_number DESC
    LIMIT 1
)
WHERE e.content_type_id = ?
ORDER BY r.menu_order, r.title, e.id;

-- name: ListPublishedHierarchyForContentType :many
SELECT e.id AS entry_id, e.content_type_id, COALESCE(NULLIF(r.slug, ''), e.slug) AS slug, e.status, r.title,
       r.parent_entry_id, r.menu_order
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
WHERE e.content_type_id = ? AND e.status = 'active' AND e.published_revision_id IS NOT NULL
ORDER BY r.menu_order, r.title, e.id;

-- name: CreateEntryRevision :exec
INSERT INTO entry_revisions (
    id, entry_id, revision_number, slug, title, excerpt, document_json,
    seo_title, seo_description, canonical_url, featured_media_id, social_media_id,
    seo_robots_index, seo_robots_follow, schema_mode, layout_template_id, parent_entry_id, menu_order, fields_json, created_by, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(sqlc.arg(fields_json), ''), '{}'), sqlc.arg(created_by), sqlc.arg(created_at));
