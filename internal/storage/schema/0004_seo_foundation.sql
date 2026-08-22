-- ===== 0004_seo_foundation.sql =====
-- SEO foundation: move featured image to the revision workflow and add
-- per-revision social image + robots overrides.
--
-- featured_media_id was on entries, so changing the featured image leaked to
-- the public site before Publish. Moving it to entry_revisions keeps drafts
-- isolated: the public renderer reads only the published revision.

-- Per-revision SEO image + robots overrides.
-- Nullable INTEGER (0/1) with inherit semantics: NULL = inherit, 0 = false, 1 = true.
ALTER TABLE entry_revisions ADD COLUMN featured_media_id TEXT;
ALTER TABLE entry_revisions ADD COLUMN social_media_id TEXT;
ALTER TABLE entry_revisions ADD COLUMN seo_robots_index INTEGER CHECK (seo_robots_index IN (0, 1));
ALTER TABLE entry_revisions ADD COLUMN seo_robots_follow INTEGER CHECK (seo_robots_follow IN (0, 1));

CREATE INDEX idx_entry_revisions_featured_media ON entry_revisions (featured_media_id);
CREATE INDEX idx_entry_revisions_social_media ON entry_revisions (social_media_id);

-- Migrate existing featured_media_id from entries to the published revision
-- (or latest revision when not yet published). Existing data is preserved;
-- the old column is kept for rollback but no longer read by the public renderer.
UPDATE entry_revisions
SET featured_media_id = (
    SELECT e.featured_media_id
    FROM entries e
    WHERE e.id = entry_revisions.entry_id
      AND e.featured_media_id IS NOT NULL
      AND e.published_revision_id = entry_revisions.id
)
WHERE EXISTS (
    SELECT 1 FROM entries e
    WHERE e.id = entry_revisions.entry_id
      AND e.featured_media_id IS NOT NULL
      AND e.published_revision_id = entry_revisions.id
);

-- For entries that have a featured image but no published revision yet
-- (pure drafts), copy it to their latest revision so the editor preview
-- retains the image.
UPDATE entry_revisions
SET featured_media_id = (
    SELECT e.featured_media_id
    FROM entries e
    WHERE e.id = entry_revisions.entry_id
      AND e.featured_media_id IS NOT NULL
      AND e.published_revision_id IS NULL
      AND entry_revisions.revision_number = (
          SELECT MAX(revision_number) FROM entry_revisions er2 WHERE er2.entry_id = e.id
      )
)
WHERE featured_media_id IS NULL
  AND EXISTS (
    SELECT 1 FROM entries e
    WHERE e.id = entry_revisions.entry_id
      AND e.featured_media_id IS NOT NULL
      AND e.published_revision_id IS NULL
      AND entry_revisions.revision_number = (
          SELECT MAX(revision_number) FROM entry_revisions er2 WHERE er2.entry_id = e.id
      )
);
