CREATE TABLE import_runs (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    UNIQUE(id, source)
);

CREATE TABLE import_mappings (
    source TEXT NOT NULL,
    object_type TEXT NOT NULL,
    external_id TEXT NOT NULL,
    internal_id TEXT NOT NULL,
    run_id TEXT NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY(source, object_type, external_id)
);

CREATE INDEX idx_import_mappings_run ON import_mappings(run_id);
