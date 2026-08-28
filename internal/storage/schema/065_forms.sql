CREATE TABLE forms (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
    definition_json TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    CHECK (length(name) BETWEEN 1 AND 200)
);

CREATE TABLE form_submissions (
    id TEXT PRIMARY KEY,
    form_id TEXT NOT NULL REFERENCES forms(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'read', 'spam', 'archived')),
    values_json TEXT NOT NULL,
    schema_snapshot_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_form_submissions_form_created ON form_submissions(form_id, created_at DESC);
CREATE INDEX idx_form_submissions_form_status_created ON form_submissions(form_id, status, created_at DESC);

