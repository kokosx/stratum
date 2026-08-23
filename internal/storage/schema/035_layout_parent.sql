-- 035_layout_parent.sql
-- Nested template composition: one optional parent per template.
PRAGMA foreign_keys = ON;

ALTER TABLE layout_templates
ADD COLUMN parent_template_id TEXT
REFERENCES layout_templates(id)
ON DELETE SET NULL;

CREATE INDEX idx_layout_templates_parent
ON layout_templates(parent_template_id);
