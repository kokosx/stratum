-- Site Icon (favicon) references a single media asset. The CMS generates the
-- needed favicon variants from that asset; the user never manages the files.
-- SQLite cannot add a FOREIGN KEY to an existing column via ALTER TABLE, so
-- referential integrity is enforced in the application: saving validates that
-- the media asset exists, and deletion is guarded by CountMediaUsage.
ALTER TABLE site_settings ADD COLUMN site_icon_media_id TEXT;

CREATE INDEX idx_site_settings_site_icon ON site_settings (site_icon_media_id);
