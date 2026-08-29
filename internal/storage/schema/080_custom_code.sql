CREATE TABLE custom_code_snippets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) >= 1 AND length(name) <= 100),
    scope TEXT NOT NULL CHECK (scope IN ('global','template')),
    scope_id TEXT CHECK ((scope = 'global' AND scope_id IS NULL) OR (scope = 'template' AND scope_id IS NOT NULL)),
    kind TEXT NOT NULL CHECK (kind IN ('html','css','js')),
    placement TEXT NOT NULL CHECK (placement IN ('head','body_start','body_end')),
    code TEXT NOT NULL CHECK (length(code) <= 200000),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (scope_id) REFERENCES layout_templates(id) ON DELETE CASCADE
);

CREATE INDEX idx_custom_code_scope ON custom_code_snippets(scope, scope_id, placement, sort_order, id);

-- Core custom-code block for SDT
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
) VALUES (
    'core-custom-code-v1', 'core', 'custom-code', 1, 'Custom Code', 'Insert trusted HTML, CSS or JavaScript at this position. Runs only on the published site.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"code":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"kind":{"type":"string","enum":["html","css","js"],"default":"html"}}},"children":{"mode":"none"},"editor":{"contexts":["page","post","layout-template","site-part"]}}',
    'template',
    '{{if .Context.IsPreview}}<div class="block-placeholder stratum-custom-code-placeholder" data-kind="{{ .Settings.kind }}">Custom {{if eq .Settings.kind "css"}}CSS{{else if eq .Settings.kind "js"}}JavaScript{{else}}HTML{{end}} — Runs on the published site.</div>{{else if eq .Settings.kind "css"}}<style data-stratum-custom-code="{{ .ID }}">{{ raw .Props.code }}</style>{{else if eq .Settings.kind "js"}}<script data-stratum-custom-code="{{ .ID }}">{{ raw .Props.code }}</script>{{else}}{{ raw .Props.code }}{{end}}',
    '',
    'core',
    1, unixepoch(), unixepoch()
);
