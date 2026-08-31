-- Accordion starter structure: new Accordion gets 2 Accordion Items as real SDT children (editor-only placeholder for titles/content).
-- Generic factory (mutations.js createValidNode) already supports starterChildren; this migration declares it.
UPDATE block_definitions
SET schema_json = '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["minimal","bordered","cards"],"default":"minimal"}}},"children":{"mode":"allowed","blocks":["core/accordion-item"],"min":1},"editor":{"category":"design","icon":"accordion","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}},"starterChildren":[{"block":"core/accordion-item","version":1},{"block":"core/accordion-item","version":1}]}}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'accordion' AND version = 1;
