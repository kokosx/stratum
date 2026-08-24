-- name: CreateLayoutTemplate :exec
INSERT INTO layout_templates (id, name, content_type_id, published_revision_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetLayoutTemplate :one
SELECT *
FROM layout_templates
WHERE id = ?
LIMIT 1;

-- name: ListLayoutTemplates :many
SELECT *
FROM layout_templates
ORDER BY
    CASE WHEN published_revision_id IS NULL THEN 1 ELSE 0 END,
    name,
    id;

-- name: ListLayoutTemplatesByContentType :many
SELECT *
FROM layout_templates
WHERE content_type_id = ?
ORDER BY name, id;

-- name: ListPublishedLayoutTemplatesByContentType :many
SELECT *
FROM layout_templates
WHERE content_type_id = ?
  AND published_revision_id IS NOT NULL
ORDER BY name, id;

-- name: UpdateLayoutTemplate :exec
UPDATE layout_templates
SET name = ?, updated_at = ?
WHERE id = ?;

-- name: SetLayoutTemplatePublishedRevision :exec
UPDATE layout_templates
SET published_revision_id = ?, updated_at = ?
WHERE id = ?;

-- name: ClearLayoutTemplatePublishedRevision :exec
UPDATE layout_templates
SET published_revision_id = NULL, updated_at = ?
WHERE id = ?;

-- name: GetLayoutTemplateRevision :one
SELECT *
FROM layout_template_revisions
WHERE id = ?
LIMIT 1;

-- name: GetLatestLayoutTemplateRevision :one
SELECT *
FROM layout_template_revisions
WHERE template_id = ?
ORDER BY revision_number DESC
LIMIT 1;

-- name: ListLayoutTemplateRevisions :many
SELECT *
FROM layout_template_revisions
WHERE template_id = ?
ORDER BY revision_number DESC;

-- name: GetPublishedLayoutTemplateRevision :one
SELECT r.*
FROM layout_templates t
JOIN layout_template_revisions r ON r.id = t.published_revision_id
WHERE t.id = ?
LIMIT 1;

-- name: CreateLayoutTemplateRevision :exec
INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetContentTypeWithDefault :one
SELECT *
FROM content_types
WHERE id = ?
LIMIT 1;

-- name: SetContentTypeDefaultLayoutTemplate :exec
UPDATE content_types
SET default_layout_template_id = ?, updated_at = ?
WHERE id = ?;

-- name: ClearContentTypeDefaultLayoutTemplate :exec
UPDATE content_types
SET default_layout_template_id = NULL, updated_at = ?
WHERE id = ?;

-- name: GetLayoutTemplateWithPublishedRevision :one
SELECT
    t.id,
    t.name,
    t.content_type_id,
    t.published_revision_id,
    t.created_at,
    t.updated_at,
    r.id AS revision_id,
    r.document_json
FROM layout_templates t
JOIN layout_template_revisions r ON r.id = t.published_revision_id
WHERE t.id = ?
LIMIT 1;

-- name: ListLatestLayoutRevisions :many
SELECT *
FROM layout_template_revisions
WHERE id IN (
    SELECT id FROM layout_template_revisions
    WHERE (template_id, revision_number) IN (
        SELECT template_id, MAX(revision_number) FROM layout_template_revisions GROUP BY template_id
    )
);
