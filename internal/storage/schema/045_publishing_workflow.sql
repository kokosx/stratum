-- Publishing workflow: revision-scoped visibility/sticky/review_state and durable scheduled publication jobs.

-- Extend entry_revisions with immutable revision metadata.
-- Visibility describes how the published revision is exposed: public/private/password.
-- Review state describes editorial workflow for the latest draft: draft/pending.
-- Sticky is revision-scoped; only the published revision's sticky affects ordering.
-- password_hash stores bcrypt hash when visibility=password, else NULL.

ALTER TABLE entry_revisions ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public'
    CHECK (visibility IN ('public', 'private', 'password'));
ALTER TABLE entry_revisions ADD COLUMN password_hash TEXT
    CHECK (visibility != 'password' OR password_hash IS NOT NULL);
ALTER TABLE entry_revisions ADD COLUMN sticky INTEGER NOT NULL DEFAULT 0
    CHECK (sticky IN (0, 1));
ALTER TABLE entry_revisions ADD COLUMN review_state TEXT NOT NULL DEFAULT 'draft'
    CHECK (review_state IN ('draft', 'pending'));

CREATE INDEX IF NOT EXISTS idx_entry_revisions_visibility ON entry_revisions(entry_id, visibility);
CREATE INDEX IF NOT EXISTS idx_entry_revisions_sticky ON entry_revisions(entry_id, sticky);

-- Durable scheduled publication jobs: one exact revision per entry to publish at scheduled_at.
CREATE TABLE publication_jobs (
    id TEXT PRIMARY KEY,
    entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    revision_id TEXT NOT NULL REFERENCES entry_revisions(id) ON DELETE CASCADE,
    scheduled_at INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'completed', 'cancelled', 'failed')),
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_error TEXT,
    CHECK (scheduled_at > 0)
);

-- One active schedule per entry (completed/cancelled/failed are historic).
CREATE UNIQUE INDEX idx_publication_jobs_active_entry
    ON publication_jobs(entry_id)
    WHERE status = 'scheduled';

-- Efficient due-job scan: status + scheduled_at.
CREATE INDEX idx_publication_jobs_due
    ON publication_jobs(status, scheduled_at);

CREATE INDEX idx_publication_jobs_entry
    ON publication_jobs(entry_id);

CREATE INDEX idx_publication_jobs_revision
    ON publication_jobs(revision_id);
