-- 034_performance_indexes.sql
-- Add missing indexes for hot public paths after EntryQuery and routing unification.
-- EXPLAIN QUERY PLAN was used to verify each index is used by the hot queries.

-- Route lookup by path is the hottest public path (page cache miss).
CREATE INDEX IF NOT EXISTS idx_routes_path_type ON routes(path, route_type);
CREATE INDEX IF NOT EXISTS idx_routes_archive_content ON routes(route_type, content_type_id) WHERE route_type = 'archive';

-- Published entries by content type ordered by publication date (Collection hot path).
-- The ListPublishedEntriesByContentType query joins entries + routes + revisions.
CREATE INDEX IF NOT EXISTS idx_entries_published_content ON entries(content_type_id, status, published_revision_id, first_published_at, published_at);
CREATE INDEX IF NOT EXISTS idx_routes_entry_type_path ON routes(entry_id, route_type, path);

-- Revisions latest lookup per entry.
CREATE INDEX IF NOT EXISTS idx_entry_revisions_entry_number ON entry_revisions(entry_id, revision_number DESC);

-- Layout template published revision lookup.
CREATE INDEX IF NOT EXISTS idx_layout_template_revisions_published ON layout_template_revisions(template_id, revision_number DESC);

-- Navigation public load and site settings are already indexed.
