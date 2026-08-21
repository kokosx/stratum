CREATE TABLE theme_customizations (
    theme_id TEXT PRIMARY KEY CHECK (length(trim(theme_id)) > 0),
    theme_version INTEGER NOT NULL CHECK (theme_version > 0),
    settings_json TEXT NOT NULL CHECK (json_valid(settings_json) AND json_type(settings_json) = 'object'),
    custom_css TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

UPDATE site_settings
SET active_theme = 'stratum/default', updated_at = unixepoch()
WHERE active_theme = 'default';

INSERT INTO theme_customizations (theme_id, theme_version, settings_json, custom_css, updated_at)
VALUES ('stratum/default', 1, '{}', '', unixepoch())
ON CONFLICT(theme_id) DO NOTHING;
