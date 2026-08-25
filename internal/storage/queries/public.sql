-- name: GetPublishedEntryByPath :one
SELECT
    e.id,
    e.content_type_id,
    r.slug,
    e.status,
    e.published_at,
    e.first_published_at,

    r.id AS revision_id,
    r.title,
    r.excerpt,
    r.document_json,
    r.fields_json,
    r.seo_title,
    r.seo_description,
    r.canonical_url,
    r.featured_media_id,
    r.social_media_id,
    r.seo_robots_index,
    r.seo_robots_follow,
    r.schema_mode,
    r.layout_template_id,
    r.visibility,
    r.password_hash,
    r.sticky,
    r.review_state

FROM routes rt

JOIN entries e
    ON e.id = rt.entry_id

JOIN entry_revisions r
    ON r.id = e.published_revision_id

WHERE rt.path = ?
  AND rt.route_type = 'entry'
  AND e.status = 'active'

LIMIT 1;

-- name: GetPublishedEntryByID :one
SELECT
    e.id,
    e.content_type_id,
    r.slug,
    e.status,
    e.published_at,
    e.first_published_at,
    r.id AS revision_id,
    r.title,
    r.excerpt,
    r.document_json,
    r.fields_json,
    r.seo_title,
    r.seo_description,
    r.canonical_url,
    r.featured_media_id,
    r.social_media_id,
    r.seo_robots_index,
    r.seo_robots_follow,
    r.schema_mode,
    r.layout_template_id,
    r.visibility,
    r.password_hash,
    r.sticky,
    r.review_state
FROM entries e
JOIN entry_revisions r
    ON r.id = e.published_revision_id
WHERE e.id = ?
  AND e.status = 'active'
  AND e.published_revision_id IS NOT NULL
LIMIT 1;

-- name: CountPublishedEntriesByContentType :one
SELECT COUNT(*)
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
JOIN routes rt ON rt.entry_id = e.id
WHERE e.content_type_id = ?
  AND e.status = 'active'
  AND e.published_revision_id IS NOT NULL
  AND r.visibility = 'public'
  AND rt.route_type = 'entry';

-- name: ListPublishedEntriesByContentType :many
SELECT
    e.id,
    r.slug,
    e.first_published_at,
    e.published_at,
    r.id AS revision_id,
    r.title,
    r.excerpt,
    r.featured_media_id,
    r.fields_json,
    rt.path AS route_path,
    r.sticky
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
JOIN routes rt ON rt.entry_id = e.id AND rt.route_type = 'entry'
WHERE e.content_type_id = ?
  AND e.status = 'active'
  AND e.published_revision_id IS NOT NULL
  AND r.visibility = 'public'
ORDER BY
    r.sticky DESC,
    COALESCE(e.first_published_at, e.published_at) DESC,
    e.published_at DESC,
    e.id DESC
LIMIT ? OFFSET ?;

-- name: ListPublishedEntriesByContentTypeAsc :many
SELECT
    e.id,
    r.slug,
    e.first_published_at,
    e.published_at,
    r.id AS revision_id,
    r.title,
    r.excerpt,
    r.featured_media_id,
    r.fields_json,
    rt.path AS route_path,
    r.sticky
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
JOIN routes rt ON rt.entry_id = e.id AND rt.route_type = 'entry'
WHERE e.content_type_id = ?
  AND e.status = 'active'
  AND e.published_revision_id IS NOT NULL
  AND r.visibility = 'public'
ORDER BY
    r.sticky DESC,
    COALESCE(e.first_published_at, e.published_at) ASC,
    e.published_at ASC,
    e.id ASC
LIMIT ? OFFSET ?;
