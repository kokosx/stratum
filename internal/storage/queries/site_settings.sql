-- name: GetSiteSettings :one
SELECT *
FROM site_settings
WHERE id = 1;

-- name: UpdateSiteSettings :exec
UPDATE site_settings
SET
    site_title = ?,
    site_tagline = ?,
    homepage_mode = ?,
    homepage_entry_id = ?,
    posts_page_entry_id = ?,
    posts_per_page = ?,
    language = ?,
    timezone = ?,
    active_theme = ?,
    indexing_enabled = ?,
    updated_at = ?
WHERE id = 1;
