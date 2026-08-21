-- name: SeedEntry :exec
INSERT INTO entries (
    id, content_type_id, slug, status, created_at, updated_at, published_at
)
VALUES (?, ?, ?, 'active', ?, ?, ?)
ON CONFLICT(id) DO NOTHING;

-- name: SeedEntryRevision :exec
INSERT INTO entry_revisions (
    id, entry_id, revision_number, title, excerpt, document_json,
    seo_title, seo_description, created_at
)
VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING;

-- name: SeedPublishedRevision :exec
UPDATE entries
SET published_revision_id = ?, published_at = ?, updated_at = ?
WHERE id = ?
  AND published_revision_id IS NULL;

-- name: SeedRoute :exec
INSERT INTO routes (
    id, path, entry_id, route_type, created_at, updated_at
)
VALUES (?, ?, ?, 'entry', ?, ?)
ON CONFLICT(path) DO NOTHING;

-- name: SeedSiteSettings :exec
UPDATE site_settings
SET
    site_title = 'StratumCMS',
    site_tagline = 'A modern self-hosted CMS',
    homepage_mode = 'page',
    homepage_entry_id = ?,
    updated_at = ?
WHERE id = 1
  AND homepage_entry_id IS NULL;
