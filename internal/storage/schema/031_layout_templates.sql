-- 031_layout_templates.sql
-- Layout Template system: reusable SDT surrounding Entry content.

PRAGMA foreign_keys = ON;

-- ============================================================
-- Layout Templates
-- ============================================================

CREATE TABLE layout_templates (
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

CREATE INDEX idx_layout_templates_content_type
    ON layout_templates(content_type_id);

-- ============================================================
-- Layout Template Revisions
-- ============================================================

CREATE TABLE layout_template_revisions (
    id TEXT PRIMARY KEY,

    template_id TEXT NOT NULL,

    revision_number INTEGER NOT NULL
        CHECK (revision_number > 0),

    document_json TEXT NOT NULL
        CHECK (json_valid(document_json)),

    created_by TEXT,

    created_at INTEGER NOT NULL,

    FOREIGN KEY(template_id)
        REFERENCES layout_templates(id)
        ON DELETE CASCADE,

    UNIQUE(template_id, revision_number)
);

CREATE INDEX idx_layout_template_revisions_template
    ON layout_template_revisions(template_id);

CREATE INDEX idx_layout_template_revisions_template_rev
    ON layout_template_revisions(template_id, revision_number);

-- ============================================================
-- Entry Revisions: layout template assignment (revision-aware)
-- ============================================================

ALTER TABLE entry_revisions
ADD COLUMN layout_template_id TEXT
REFERENCES layout_templates(id)
ON DELETE RESTRICT;

CREATE INDEX idx_entry_revisions_layout_template
    ON entry_revisions(layout_template_id);

-- ============================================================
-- Content Types: default layout template for new entries
-- ============================================================

ALTER TABLE content_types
ADD COLUMN default_layout_template_id TEXT
REFERENCES layout_templates(id)
ON DELETE SET NULL;

-- ============================================================
-- Core Content Slot block
-- ============================================================

INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-content-slot-v1', 'core', 'content-slot', 1, 'Content', 'The editable content of the current Entry is inserted here.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"layout","icon":"content-slot"}}',
    'template',
    '<div class="stratum-content-slot-placeholder" data-content-slot style="padding:16px;border:1px dashed #c7d2fe;background:#eef2ff;border-radius:8px;text-align:center;color:#4338ca;"><strong>Page Content</strong><br><span>Content added while editing this page will appear here.</span></div>',
    '',
    'core', 1, unixepoch(), unixepoch()
)
ON CONFLICT(namespace, name, version) DO UPDATE SET
    display_name = excluded.display_name,
    description = excluded.description,
    schema_json = excluded.schema_json,
    renderer_type = excluded.renderer_type,
    template = excluded.template,
    styles = excluded.styles,
    source = excluded.source,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at;

-- ============================================================
-- Default layout templates (one content slot each)
-- ============================================================

INSERT INTO layout_templates (id, name, content_type_id, published_revision_id, created_at, updated_at)
VALUES
    ('layout-template-default-page', 'Default Page', 'page', NULL, unixepoch(), unixepoch()),
    ('layout-template-default-post', 'Default Post', 'post', NULL, unixepoch(), unixepoch())
ON CONFLICT(id) DO NOTHING;

INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at)
VALUES
    ('layout-template-default-page-r1', 'layout-template-default-page', 1, '{"version":1,"nodes":[{"id":"slot-page-default","block":"core/content-slot","version":1,"props":{},"settings":{}}]}', NULL, unixepoch()),
    ('layout-template-default-post-r1', 'layout-template-default-post', 1, '{"version":1,"nodes":[{"id":"slot-post-default","block":"core/content-slot","version":1,"props":{},"settings":{}}]}', NULL, unixepoch())
ON CONFLICT(id) DO NOTHING;

UPDATE layout_templates SET published_revision_id = 'layout-template-default-page-r1' WHERE id = 'layout-template-default-page' AND published_revision_id IS NULL;
UPDATE layout_templates SET published_revision_id = 'layout-template-default-post-r1' WHERE id = 'layout-template-default-post' AND published_revision_id IS NULL;

UPDATE content_types SET default_layout_template_id = 'layout-template-default-page' WHERE id = 'page' AND (default_layout_template_id IS NULL OR default_layout_template_id = '');
UPDATE content_types SET default_layout_template_id = 'layout-template-default-post' WHERE id = 'post' AND (default_layout_template_id IS NULL OR default_layout_template_id = '');
