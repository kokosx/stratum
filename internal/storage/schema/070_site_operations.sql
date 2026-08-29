-- EPIC 5 — Site Operations: 404 aggregation
-- Historical migrations remain immutable; this is a forward-only addition.

CREATE TABLE IF NOT EXISTS not_found_paths (
    path TEXT PRIMARY KEY,
    hit_count INTEGER NOT NULL DEFAULT 1 CHECK (hit_count > 0),
    first_seen_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_not_found_last_seen ON not_found_paths(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_not_found_hit_count ON not_found_paths(hit_count DESC);
