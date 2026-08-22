-- Site runtime settings: sitemap, robots, speculation rules, and a title
-- separator. These are first-party, explicitly typed settings stored on the
-- site_settings singleton. They are NOT a generic key/value options store.
ALTER TABLE site_settings ADD COLUMN sitemap_enabled INTEGER NOT NULL DEFAULT 1
    CHECK (sitemap_enabled IN (0, 1));

ALTER TABLE site_settings ADD COLUMN robots_mode TEXT NOT NULL DEFAULT 'managed'
    CHECK (robots_mode IN ('managed', 'custom'));

ALTER TABLE site_settings ADD COLUMN robots_custom TEXT NOT NULL DEFAULT '';

ALTER TABLE site_settings ADD COLUMN speculation_mode TEXT NOT NULL DEFAULT 'off'
    CHECK (speculation_mode IN ('off', 'prefetch', 'prerender'));

ALTER TABLE site_settings ADD COLUMN speculation_eagerness TEXT NOT NULL DEFAULT 'conservative'
    CHECK (speculation_eagerness IN ('conservative', 'moderate', 'eager'));

ALTER TABLE site_settings ADD COLUMN title_separator TEXT NOT NULL DEFAULT '–';
