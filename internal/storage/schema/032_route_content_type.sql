-- 032_route_content_type.sql
-- Make archive routes explicit about which content type they serve.
-- Single entry routes already have the type via the joined entries row.
-- Archive routes need a nullable content_type_id so the system can serve
-- multiple archives (post, case-study, ...) without inferring from site settings.

ALTER TABLE routes ADD COLUMN content_type_id TEXT REFERENCES content_types(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_routes_archive_content_type ON routes(route_type, content_type_id) WHERE route_type = 'archive';

-- Backfill existing archive routes: they are all post archives.
UPDATE routes SET content_type_id = 'post' WHERE route_type = 'archive' AND content_type_id IS NULL;
