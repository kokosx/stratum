-- 034_performance_indexes.sql
-- Add missing indexes for hot public paths after EntryQuery and routing unification.
-- EXPLAIN QUERY PLAN verified: routes.path UNIQUE covers GetRouteByPath; the archive
-- content index from 032 covers archive lookups; entry_revisions unique covers
-- latest revision ordering. Only deduped, non-redundant indexes remain.

-- Redirect flattening hot path (SyncPostsPageSlugChanged)
CREATE INDEX IF NOT EXISTS idx_routes_redirect_to ON routes(redirect_to) WHERE route_type = 'redirect';

-- Route entry lookup for post remapping
CREATE INDEX IF NOT EXISTS idx_routes_entry_type_path ON routes(entry_id, route_type, path);

-- Navigation public load and site settings are already indexed.
