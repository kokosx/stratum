-- Normalize legacy private entry lifecycle: status='private' is now represented as
-- active entry with published revision visibility='private'.
-- This preserves public intent while moving source of truth to entry_revisions.visibility.
-- Private entries have no public route; after normalization they remain unpublished at route level.
UPDATE entry_revisions
SET visibility = 'private'
WHERE id IN (
    SELECT published_revision_id FROM entries WHERE status = 'private' AND published_revision_id IS NOT NULL
)
AND visibility != 'private';

UPDATE entries SET status = 'active' WHERE status = 'private';

-- status_before_trash may contain 'private' from old trash flows; normalize to 'active' for consistency.
UPDATE entries SET status_before_trash = 'active' WHERE status_before_trash = 'private';
