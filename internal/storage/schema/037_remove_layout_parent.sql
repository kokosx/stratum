-- 037_remove_layout_parent.sql
-- Forward corrective migration: remove nested template inheritance.
-- parent_template_id belongs to the logical template, not its revision,
-- so a draft Save could affect public output before Publish. Remove it.
PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_layout_templates_parent;

CREATE TABLE layout_templates_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    content_type_id TEXT NOT NULL,
    published_revision_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(content_type_id)
        REFERENCES content_types(id)
        ON DELETE RESTRICT,
    FOREIGN KEY(published_revision_id)
        REFERENCES layout_template_revisions(id)
        ON DELETE SET NULL
);

INSERT INTO layout_templates_new (id, name, content_type_id, published_revision_id, created_at, updated_at)
  SELECT id, name, content_type_id, published_revision_id, created_at, updated_at FROM layout_templates;

DROP TABLE layout_templates;

ALTER TABLE layout_templates_new RENAME TO layout_templates;

CREATE INDEX IF NOT EXISTS idx_layout_templates_content_type
    ON layout_templates(content_type_id);

PRAGMA foreign_keys = ON;
