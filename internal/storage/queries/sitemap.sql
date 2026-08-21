-- name: ListSitemapEntries :many
-- Returns every publicly published entry that owns an entry-type route. Drafts,
-- private/trash entries, redirect routes, admin/preview URLs and unpublished
-- entries are excluded by the joins and filters below. The published revision
-- timestamp drives <lastmod> so a newer draft does not change the sitemap.
SELECT
    rt.path AS route_path,
    e.content_type_id AS content_type,
    r.created_at AS lastmod
FROM routes rt
JOIN entries e
    ON e.id = rt.entry_id
JOIN entry_revisions r
    ON r.id = e.published_revision_id
WHERE rt.route_type = 'entry'
  AND e.status = 'active'
  AND e.published_revision_id IS NOT NULL
ORDER BY rt.path;
