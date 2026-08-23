-- 036_index_cleanup.sql
-- Corrective forward migration: unify schema after immutable 032/034 were restored.
-- Once a migration has landed, its contents are immutable. Any schema/index
-- correction must be a new forward migration. This migration brings both:
--   - DB upgraded from 3a046a7 (had original 032/034)
--   - fresh DB (runs restored 032/034)
-- to the same deduplicated final schema.
--
-- EXPLAIN QUERY PLAN verified: routes.path UNIQUE covers GetRouteByPath;
-- the archive partial index from 032 covers archive lookups; the UNIQUE on
-- entry_revisions(entry_id, revision_number) covers latest revision ordering.
-- Only non-redundant indexes remain.

-- Drop redundant indexes that were added in the original 032/034 but are
-- covered by UNIQUE constraints or more specific partial indexes.
DROP INDEX IF EXISTS idx_routes_content_type;
DROP INDEX IF EXISTS idx_routes_path_type;
DROP INDEX IF EXISTS idx_routes_archive_content;
DROP INDEX IF EXISTS idx_entries_published_content;
DROP INDEX IF EXISTS idx_entry_revisions_entry_number;
DROP INDEX IF EXISTS idx_layout_template_revisions_published;

-- Ensure the deduplicated, actually useful indexes exist.
CREATE INDEX IF NOT EXISTS idx_routes_archive_content_type ON routes(route_type, content_type_id) WHERE route_type = 'archive';
CREATE INDEX IF NOT EXISTS idx_routes_redirect_to ON routes(redirect_to) WHERE route_type = 'redirect';
CREATE INDEX IF NOT EXISTS idx_routes_entry_type_path ON routes(entry_id, route_type, path);
