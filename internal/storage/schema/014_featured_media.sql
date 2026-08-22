-- Featured Media / Featured Image on an Entry. The model references a media
-- asset by id; no UI is wired yet, but the column enables the feature later
-- without a schema rebuild. Content revisions already reference media by id.
-- Like site_icon_media_id, integrity is enforced in the application (SQLite
-- cannot add a FOREIGN KEY to an existing column via ALTER TABLE).
ALTER TABLE entries ADD COLUMN featured_media_id TEXT;

CREATE INDEX idx_entries_featured_media ON entries (featured_media_id);
