CREATE TABLE taxonomies (
    id TEXT PRIMARY KEY,
    content_type_id TEXT NOT NULL REFERENCES content_types(id) ON DELETE CASCADE,
    singular_name TEXT NOT NULL,
    plural_name TEXT NOT NULL,
    hierarchical INTEGER NOT NULL DEFAULT 0 CHECK (hierarchical IN (0, 1)),
    public INTEGER NOT NULL DEFAULT 1 CHECK (public IN (0, 1)),
    route_base TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(content_type_id, id)
);
CREATE TABLE terms (
    id TEXT PRIMARY KEY,
    taxonomy_id TEXT NOT NULL REFERENCES taxonomies(id) ON DELETE CASCADE,
    parent_id TEXT REFERENCES terms(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(taxonomy_id, slug)
);
CREATE TABLE entry_revision_terms (
    revision_id TEXT NOT NULL REFERENCES entry_revisions(id) ON DELETE CASCADE,
    term_id TEXT NOT NULL REFERENCES terms(id) ON DELETE CASCADE,
    PRIMARY KEY (revision_id, term_id)
);
ALTER TABLE routes ADD COLUMN taxonomy_id TEXT REFERENCES taxonomies(id) ON DELETE SET NULL;
ALTER TABLE routes ADD COLUMN term_id TEXT REFERENCES terms(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_terms_taxonomy ON terms(taxonomy_id);
CREATE INDEX IF NOT EXISTS idx_terms_parent ON terms(parent_id);
CREATE INDEX IF NOT EXISTS idx_terms_slug ON terms(taxonomy_id, slug);
CREATE INDEX IF NOT EXISTS idx_entry_revision_terms_revision ON entry_revision_terms(revision_id);
CREATE INDEX IF NOT EXISTS idx_entry_revision_terms_term ON entry_revision_terms(term_id);
CREATE INDEX IF NOT EXISTS idx_routes_taxonomy_term ON routes(taxonomy_id, term_id);
CREATE INDEX IF NOT EXISTS idx_routes_taxonomy_archive ON routes(route_type, taxonomy_id, term_id) WHERE route_type = 'archive';
INSERT INTO taxonomies (id, content_type_id, singular_name, plural_name, hierarchical, public, route_base, created_at, updated_at)
VALUES ('category', 'post', 'Category', 'Categories', 1, 1, '/category', unixepoch(), unixepoch())
ON CONFLICT(id) DO NOTHING;
INSERT INTO taxonomies (id, content_type_id, singular_name, plural_name, hierarchical, public, route_base, created_at, updated_at)
VALUES ('tag', 'post', 'Tag', 'Tags', 0, 1, '/tag', unixepoch(), unixepoch())
ON CONFLICT(id) DO NOTHING;
