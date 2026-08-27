-- 059_fix_site_part_template.sql
-- Fix core/site-part template to include Children so nested site parts render correctly.
PRAGMA foreign_keys = ON;
UPDATE block_definitions
SET template = '<div class="stratum-site-part">{{ .Children }}</div>',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'site-part' AND version = 1;
