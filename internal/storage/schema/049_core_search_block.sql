INSERT INTO block_definitions (id, namespace, name, version, display_name, description, schema_json, renderer_type, template, source, enabled, created_at, updated_at)
SELECT 'core-search-v1', 'core', 'search', 1, 'Search', 'A public site search form.',
  '{"schemaVersion":1,"props":{"type":"object","properties":{"placeholder":{"type":"string","default":"Search"},"buttonLabel":{"type":"string","default":"Search"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"none"},"editor":{"category":"widgets","icon":"search","summaryFields":[]}}',
  'template', '<form method="get" action="/search" class="stratum-search"><label><span class="screen-reader-text">Search</span><input type="search" name="q" placeholder="{{ .Props.placeholder }}"></label><button type="submit">{{ .Props.buttonLabel }}</button></form>', 'core', 1, unixepoch(), unixepoch()
WHERE NOT EXISTS (SELECT 1 FROM block_definitions WHERE namespace = 'core' AND name = 'search' AND version = 1);
