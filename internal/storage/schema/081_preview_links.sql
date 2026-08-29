CREATE TABLE preview_links (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    entry_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    created_by TEXT,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE,
    FOREIGN KEY (revision_id) REFERENCES entry_revisions(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_preview_links_token_hash ON preview_links(token_hash);
CREATE INDEX idx_preview_links_entry_id ON preview_links(entry_id);
CREATE INDEX idx_preview_links_revision_id ON preview_links(revision_id);
CREATE INDEX idx_preview_links_expires_at ON preview_links(expires_at);
