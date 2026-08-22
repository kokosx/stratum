-- ===== 026_schema_mode.sql =====
-- Per-revision structured-data mode override.
-- Values: ''/NULL = automatic, 'disabled', 'webpage', 'aboutpage', 'contactpage'.
ALTER TABLE entry_revisions ADD COLUMN schema_mode TEXT NOT NULL DEFAULT '';
