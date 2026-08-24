ALTER TABLE entries ADD COLUMN status_before_trash TEXT CHECK (status_before_trash IN ('active', 'private'));
ALTER TABLE entries ADD COLUMN trashed_at INTEGER;
CREATE INDEX IF NOT EXISTS idx_entries_trash ON entries(status, trashed_at);
