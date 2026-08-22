-- SEO layer: per-revision canonical URL and the site's canonical public origin.
ALTER TABLE entry_revisions ADD COLUMN canonical_url TEXT;
ALTER TABLE site_settings ADD COLUMN site_url TEXT NOT NULL DEFAULT '';
