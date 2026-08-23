-- name: ListSitemapEntries :many
-- Returns every URL that belongs in the sitemap: published and active entries
-- of public content types that own an entry-type route and resolve as
-- indexable. Drafts (no published revision), private/trash entries,
-- non-public content types, redirect/system routes, admin/preview URLs and
-- noindex revisions are all excluded by the joins and filters below.
-- <lastmod> comes from the published revision timestamp, so a newer draft
-- never changes the sitemap.
SELECT
    rt.path AS route_path,
    e.content_type_id AS content_type,
    r.created_at AS lastmod
FROM routes rt
JOIN entries e
    ON e.id = rt.entry_id
JOIN entry_revisions r
    ON r.id = e.published_revision_id
JOIN content_types ct
    ON ct.id = e.content_type_id
WHERE rt.route_type = 'entry'
  AND e.status = 'active'
  AND e.published_revision_id IS NOT NULL
  AND ct.public = 1
  AND (r.seo_robots_index IS NULL OR r.seo_robots_index != 0)
ORDER BY rt.path;

-- name: ListSitemapArchiveRoutes :many
SELECT path FROM routes WHERE route_type = 'archive' ORDER BY path;
