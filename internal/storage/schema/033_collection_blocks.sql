-- 033_collection_blocks.sql
-- Generic collection block replaces core/posts. It fetches entries via EntryQuery
-- and renders its children once per entry with a scoped Entry context.
PRAGMA foreign_keys = ON;

INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-collection-v1', 'core', 'collection', 1, 'Collection', 'Display entries from any content type. Children are rendered once per entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"contentType":{"type":"string","enum":["post","page"],"default":"post"},"limit":{"type":"integer","default":3,"minimum":1,"maximum":20},"offset":{"type":"integer","default":0},"order":{"type":"string","enum":["published_desc","published_asc"],"default":"published_desc"},"source":{"type":"string","enum":["query","context"],"default":"query"},"excludeCurrent":{"type":"boolean","default":false}}},"children":{"mode":"any"},"editor":{"category":"query","icon":"collection","contexts":["entry","layout-template"],"fields":{"settings.contentType":{"label":"Content type","control":"select","group":"Content"},"settings.limit":{"label":"Number of items","control":"number","group":"Content"},"settings.order":{"label":"Order","control":"select","group":"Content"},"settings.source":{"label":"Source","control":"select","group":"Content"},"settings.excludeCurrent":{"label":"Exclude current entry","control":"checkbox","group":"Content"}}}}',
    'template',
    '<div class="stratum-collection">{{ .Children }}</div>',
    '.stratum-collection{display:grid;gap:var(--st-space-lg)}.stratum-collection .stratum-entry-title{margin:0}',
    'core', 1, unixepoch(), unixepoch()
)
ON CONFLICT(namespace, name, version) DO UPDATE SET
    display_name = excluded.display_name,
    description = excluded.description,
    schema_json = excluded.schema_json,
    renderer_type = excluded.renderer_type,
    template = excluded.template,
    styles = excluded.styles,
    source = excluded.source,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at;

-- Entry Link: renders a link to the current entry's permalink. Useful inside a Collection.
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-entry-link-v1', 'core', 'entry-link', 1, 'Entry Link', 'A link to the current entry. Must be used inside a Collection or on a single entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"text":{"type":"string","default":"View entry"}}},"settings":{"type":"object","properties":{"openInNewTab":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"link","fields":{"props.text":{"label":"Text","control":"text","group":"Content"},"settings.openInNewTab":{"label":"Open in new tab","control":"checkbox","group":"Style"}}}}',
    'template',
    '{{ if .Context.Entry.Permalink }}<a class="stratum-entry-link" href="{{ .Context.Entry.Permalink }}"{{ if .Settings.openInNewTab }} target="_blank" rel="noopener"{{ end }}>{{ if .Props.text }}{{ .Props.text }}{{ else }}{{ .Context.Entry.Title }}{{ end }}</a>{{ else }}<span class="stratum-placeholder">Entry link</span>{{ end }}',
    '.stratum-entry-link{color:var(--st-color-primary);text-decoration:none}.stratum-entry-link:hover{text-decoration:underline}',
    'core', 1, unixepoch(), unixepoch()
)
ON CONFLICT(namespace, name, version) DO UPDATE SET
    display_name = excluded.display_name,
    description = excluded.description,
    schema_json = excluded.schema_json,
    renderer_type = excluded.renderer_type,
    template = excluded.template,
    styles = excluded.styles,
    source = excluded.source,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at;

-- Deprecate core/posts: keep it enabled for historical documents but hide from inserter.
-- We mark it disabled in the editor catalog via a flag; the runtime keeps it for old docs.
-- For now we keep enabled=1 for backward compat but the registry will filter it via deprecation list.
-- To hide it, we set a marker in schema_json editor.hidden.
UPDATE block_definitions SET schema_json = replace(schema_json, '"category":"query"', '"category":"query","hidden":true') WHERE namespace='core' AND name='posts' AND version=1 AND instr(schema_json, '"hidden"')=0;
