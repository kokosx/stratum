-- name: GetThemeCustomization :one
SELECT theme_id, theme_version, settings_json, custom_css, updated_at
FROM theme_customizations
WHERE theme_id = ?;

-- name: UpsertThemeCustomization :exec
INSERT INTO theme_customizations (theme_id, theme_version, settings_json, custom_css, updated_at)
VALUES (?, ?, ?, ?, unixepoch())
ON CONFLICT(theme_id) DO UPDATE SET
    theme_version = excluded.theme_version,
    settings_json = excluded.settings_json,
    custom_css = excluded.custom_css,
    updated_at = excluded.updated_at;
