-- 076_entry_media_v2.sql
-- Introduce Entry Media v2 with explicit aspect/fit presentation.
-- v1 remains renderable (historical docs use natural rendering).
PRAGMA foreign_keys = ON;

INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-entry-media-v2', 'core', 'entry-media', 2, 'Entry Media', 'Display a media field from the current entry with explicit presentation.',
    '{"schemaVersion":1,"props":{"type":"object","required":["source"],"properties":{"source":{"type":"string","default":"entry.featured_media"}}},"settings":{"type":"object","properties":{"alt":{"type":"string","default":""},"sizes":{"type":"string","default":"100vw"},"aspect":{"type":"string","enum":["natural","square","landscape","standard"],"default":"natural"},"fit":{"type":"string","enum":["cover","contain"],"default":"cover"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"image","contexts":["entry","layout-template"],"fields":{"props.source":{"label":"Media field","control":"select","group":"Content","optionsSource":"entry-fields"},"settings.alt":{"label":"Alt override","control":"text","group":"Content"},"settings.sizes":{"label":"Sizes","control":"text","group":"Style"},"settings.aspect":{"label":"Aspect","control":"select","group":"Style"},"settings.fit":{"label":"Fit","control":"select","group":"Style"}}}}',
    'runtime',
    '<figure class="stratum-entry-media"></figure>',
    '.stratum-entry-media{margin:0}.stratum-entry-media img{display:block;max-width:100%;height:auto}.stratum-entry-media-aspect-square img{aspect-ratio:1/1;object-fit:cover}.stratum-entry-media-aspect-landscape img{aspect-ratio:3/2;object-fit:cover}.stratum-entry-media-aspect-standard img{aspect-ratio:4/3;object-fit:cover}.stratum-entry-media-fit-contain img{object-fit:contain}',
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
