-- 030_route_invariants.sql
-- Enforce invariants that the application already maintains: one canonical owned public route per entry.

-- Each Entry can own at most one public route of type entry or archive.
-- Redirects keep history with entry_id NULL and are not constrained.
CREATE UNIQUE INDEX IF NOT EXISTS routes_entry_canonical_unique
ON routes(entry_id)
WHERE entry_id IS NOT NULL AND route_type IN ('entry', 'archive');
