-- 088_placement_parents.sql
-- Add generic child→parent placement contract (M6). Accordion Item may only live inside Accordion.
PRAGMA foreign_keys = ON;

UPDATE block_definitions
SET schema_json = '{"schemaVersion":1,"placement":{"parents":["core/accordion"]},"props":{"type":"object","required":["title"],"properties":{"title":{"type":"string","default":"Item"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"accordion-item","fields":{"props.title":{"label":"Title","control":"text"}}}}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'accordion-item' AND version = 1;
