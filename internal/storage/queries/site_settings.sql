-- name: GetSiteSettings :one
SELECT id, site_title, site_tagline, homepage_mode, homepage_entry_id, posts_page_entry_id, posts_per_page, language, timezone, active_theme, indexing_enabled, site_url, sitemap_enabled, robots_mode, robots_custom, speculation_mode, speculation_eagerness, title_separator, site_logo_media_id, social_links, site_social_media_id, twitter_site, site_represents, created_at, updated_at
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
    site_url = ?,
    sitemap_enabled = ?,
    robots_mode = ?,
    robots_custom = ?,
    speculation_mode = ?,
    speculation_eagerness = ?,
    title_separator = ?,
    site_social_media_id = ?,
    twitter_site = ?,
    site_represents = ?,
    updated_at = ?
WHERE id = 1;
