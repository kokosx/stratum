-- Legacy private entries may still have a public route from before visibility
-- became revision-scoped. Private revisions must never be publicly routable.
DELETE FROM routes
WHERE route_type = 'entry'
  AND entry_id IN (
    SELECT e.id
    FROM entries e
    JOIN entry_revisions r ON r.id = e.published_revision_id
    WHERE r.visibility = 'private'
  );
