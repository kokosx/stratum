-- name: ListSitemapEntries :many
-- Returns every URL that belongs in the sitemap: published and active entries
-- of public content types that own an entry-type route and resolve as
-- indexable. Drafts (no published revision), private/trash entries,
-- private/password visibility, non-public content types, redirect/system routes,
-- admin/preview URLs and noindex revisions are all excluded by the joins and filters below.
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
  AND r.visibility = 'public'
  -- LEGACY STORAGE COMPATIBILITY ONLY: ct.public mirrors Routing.Single for custom types (public = single).
  -- Routes are source of truth for public URLs; this filter is kept for backward compat and to exclude legacy private types.
  AND ct.public = 1
  AND (r.seo_robots_index IS NULL OR r.seo_robots_index != 0)
ORDER BY rt.path;

-- name: ListSitemapArchiveRoutes :many
SELECT routes.path
FROM routes
WHERE routes.route_type = 'archive'
  AND (
      routes.term_id IS NULL
      OR EXISTS (
          SELECT 1
          FROM entries
          JOIN entry_revisions pr ON pr.id = entries.published_revision_id
          JOIN entry_revision_terms ON entry_revision_terms.revision_id = entries.published_revision_id
          WHERE entry_revision_terms.term_id = routes.term_id
            AND entries.status = 'active'
            AND entries.published_revision_id IS NOT NULL
            AND pr.visibility = 'public'
      )
  )
ORDER BY routes.path;
