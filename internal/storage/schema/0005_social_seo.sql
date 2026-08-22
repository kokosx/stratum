-- ===== 0005_social_seo.sql =====
-- Social SEO foundation: global default social image, optional Twitter site handle,
-- and support for the dedicated 1200x630 social preview variant (stored as a
-- media_variants row with kind='social').

ALTER TABLE site_settings ADD COLUMN site_social_media_id TEXT;
CREATE INDEX idx_site_settings_social_media ON site_settings (site_social_media_id);

-- Optional Twitter/X handle for twitter:site (e.g. "@stratum"). Empty means no tag.
ALTER TABLE site_settings ADD COLUMN twitter_site TEXT NOT NULL DEFAULT '';
