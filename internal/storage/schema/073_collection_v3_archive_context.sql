-- EPIC 7: Collection v3 must be valid in archive templates (Creator generates archive Collections).
-- Version 2 is immutable and remains untouched.
PRAGMA foreign_keys = ON;
UPDATE block_definitions
SET schema_json = replace(schema_json, '"contexts":["entry","layout-template"]', '"contexts":["entry","layout-template","archive-template","site-part"]'),
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'collection' AND version = 3 AND instr(schema_json, '"contexts":["entry","layout-template"]') > 0;
