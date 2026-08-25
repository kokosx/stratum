-- Search is a rebuildable projection of public published revisions.
CREATE TABLE search_documents (
    entry_id TEXT PRIMARY KEY,
    content_type_id TEXT NOT NULL,
    title TEXT NOT NULL,
    excerpt TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    fields TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL,
    first_published_at INTEGER,
    FOREIGN KEY(entry_id) REFERENCES entries(id) ON DELETE CASCADE
);

CREATE VIRTUAL TABLE search_documents_fts USING fts5(
    entry_id UNINDEXED,
    title,
    excerpt,
    body,
    fields,
    tokenize='unicode61'
);
