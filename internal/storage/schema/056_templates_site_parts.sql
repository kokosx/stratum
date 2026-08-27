-- 056_templates_site_parts.sql
-- Generalize Layout Template model to support archive templates and introduce Site Parts.
PRAGMA foreign_keys = ON;

-- ============================================================
-- Layout Templates: add kind
-- ============================================================

ALTER TABLE layout_templates
ADD COLUMN kind TEXT NOT NULL DEFAULT 'single'
CHECK (kind IN ('single', 'archive'));

CREATE INDEX idx_layout_templates_kind
    ON layout_templates(kind);

CREATE INDEX idx_layout_templates_kind_content_type
    ON layout_templates(kind, content_type_id);

UPDATE layout_templates SET kind = 'single' WHERE kind IS NULL OR kind = '';

-- ============================================================
-- Content Types: default archive template
-- ============================================================

ALTER TABLE content_types
ADD COLUMN default_archive_template_id TEXT
REFERENCES layout_templates(id)
ON DELETE SET NULL;

-- ============================================================
-- Site Parts
-- ============================================================

CREATE TABLE site_parts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    published_revision_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(published_revision_id)
        REFERENCES site_part_revisions(id)
        ON DELETE SET NULL
);

CREATE INDEX idx_site_parts_published
    ON site_parts(published_revision_id);

CREATE TABLE site_part_revisions (
    id TEXT PRIMARY KEY,
    site_part_id TEXT NOT NULL,
    revision_number INTEGER NOT NULL
        CHECK (revision_number > 0),
    document_json TEXT NOT NULL
        CHECK (json_valid(document_json)),
    created_by TEXT,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(site_part_id)
        REFERENCES site_parts(id)
        ON DELETE CASCADE,
    UNIQUE(site_part_id, revision_number)
);

CREATE INDEX idx_site_part_revisions_site_part
    ON site_part_revisions(site_part_id);

CREATE INDEX idx_site_part_revisions_site_part_rev
    ON site_part_revisions(site_part_id, revision_number);

-- ============================================================
-- Site Part Locations (header/footer assignments)
-- ============================================================

CREATE TABLE site_part_locations (
    location TEXT PRIMARY KEY
        CHECK (location IN ('header', 'footer')),
    site_part_id TEXT
        REFERENCES site_parts(id)
        ON DELETE SET NULL,
    updated_at INTEGER NOT NULL
);
