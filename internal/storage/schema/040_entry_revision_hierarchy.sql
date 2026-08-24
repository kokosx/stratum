-- Hierarchy is revisioned: a draft re-parent must not change public URLs before
-- that revision is published. parent_entry_id intentionally has no FK because
-- historical revisions may retain the stable ID of a permanently deleted entry.
ALTER TABLE entry_revisions ADD COLUMN parent_entry_id TEXT;
ALTER TABLE entry_revisions ADD COLUMN menu_order INTEGER NOT NULL DEFAULT 0 CHECK (menu_order >= 0);

CREATE INDEX IF NOT EXISTS idx_entry_revisions_parent_entry_id ON entry_revisions(parent_entry_id);
CREATE INDEX IF NOT EXISTS idx_entry_revisions_entry_revision_number ON entry_revisions(entry_id, revision_number DESC);
