-- name: SeedBlockDefinition :exec
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, source, enabled, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, 'template', ?, 'core', 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    display_name = excluded.display_name,
    schema_json = excluded.schema_json,
    renderer_type = excluded.renderer_type,
    template = excluded.template,
    updated_at = excluded.updated_at;

-- name: SeedEntry :exec
INSERT INTO entries (
    id, content_type_id, slug, status, created_at, updated_at, published_at
)
VALUES (?, 'page', ?, 'active', ?, ?, ?)
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
