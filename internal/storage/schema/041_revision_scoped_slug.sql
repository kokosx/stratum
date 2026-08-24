-- A slug belongs to a revision. Draft renames must not change the public URL
-- until that revision is published.
ALTER TABLE entry_revisions ADD COLUMN slug TEXT NOT NULL DEFAULT '';
UPDATE entry_revisions
SET slug = (SELECT slug FROM entries WHERE entries.id = entry_revisions.entry_id)
WHERE slug = '';

-- SQLite cannot drop the implicit UNIQUE(content_type_id, slug) index, so
-- rebuild entries without it. The legacy slug is retained only for old data
-- compatibility; all reads use entry_revisions.slug.
PRAGMA defer_foreign_keys = ON;
CREATE TABLE entries_new (
    id TEXT PRIMARY KEY,
    content_type_id TEXT NOT NULL,
    slug TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'private', 'trash')),
    author_id TEXT,
    published_revision_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    published_at INTEGER,
    featured_media_id TEXT,
    first_published_at INTEGER,
    status_before_trash TEXT CHECK (status_before_trash IN ('active', 'private')),
    trashed_at INTEGER,
    FOREIGN KEY(content_type_id) REFERENCES content_types(id) ON DELETE RESTRICT,
    FOREIGN KEY(published_revision_id) REFERENCES entry_revisions(id) ON DELETE SET NULL
);
INSERT INTO entries_new SELECT id, content_type_id, slug, status, author_id, published_revision_id, created_at, updated_at, published_at, featured_media_id, first_published_at, status_before_trash, trashed_at FROM entries;
DROP TABLE entries;
ALTER TABLE entries_new RENAME TO entries;
CREATE INDEX idx_entries_content_type ON entries(content_type_id);
CREATE INDEX idx_entries_status ON entries(status);
CREATE INDEX idx_entries_slug ON entries(slug);
CREATE INDEX idx_entries_featured_media ON entries(featured_media_id);
CREATE INDEX idx_entries_trash ON entries(status, trashed_at);
CREATE INDEX idx_entry_revisions_slug ON entry_revisions(entry_id, slug);
