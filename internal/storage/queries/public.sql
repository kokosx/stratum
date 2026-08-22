-- name: GetPublishedEntryByPath :one
SELECT
    e.id,
    e.content_type_id,
    e.slug,
    e.status,
    e.published_at,
    e.first_published_at,

    r.id AS revision_id,
    r.title,
    r.excerpt,
    r.document_json,
    r.seo_title,
    r.seo_description,
    r.canonical_url,
    r.featured_media_id,
    r.social_media_id,
    r.seo_robots_index,
    r.seo_robots_follow

FROM routes rt

JOIN entries e
    ON e.id = rt.entry_id

JOIN entry_revisions r
    ON r.id = e.published_revision_id

WHERE rt.path = ?
  AND rt.route_type = 'entry'
  AND e.status = 'active'

LIMIT 1;