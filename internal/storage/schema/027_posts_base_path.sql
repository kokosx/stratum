-- ===== 027_posts_base_path.sql =====
-- Explicit, validated base path for the Post archive and Post single URLs.
-- Default "/blog". Admin validates on save: must start with /, no trailing / (except root), no query/fragment, no collision with reserved paths.
ALTER TABLE site_settings ADD COLUMN posts_base_path TEXT NOT NULL DEFAULT '/blog';
