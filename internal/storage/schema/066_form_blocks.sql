INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
) VALUES (
    'core-form-v1', 'core', 'form', 1, 'Form', 'Displays a reusable first-party form.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","required":["formId"],"properties":{"formId":{"type":"string","default":""}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"form","contexts":["entry","single-template","archive-template","site-part"],"fields":{"settings.formId":{"label":"Form","control":"select","group":"Content","optionsSource":"forms"}}}}',
    'runtime', '<div></div>',
    '.stratum-form{display:grid;gap:var(--st-space-md)}.stratum-form-field{display:grid;gap:var(--st-space-xs)}.stratum-form input:not([type=checkbox]),.stratum-form textarea,.stratum-form select{width:100%;box-sizing:border-box;padding:var(--st-space-sm);color:var(--st-color-text);background:transparent;border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm)}.stratum-form textarea{min-height:8rem;resize:vertical}.stratum-form button{justify-self:start}.stratum-form-honeypot{position:absolute!important;width:1px!important;height:1px!important;overflow:hidden!important;clip:rect(0,0,0,0)!important;white-space:nowrap!important}.stratum-form-success{color:var(--st-color-text)}',
    'core', 1, unixepoch(), unixepoch()
) ON CONFLICT(namespace, name, version) DO NOTHING;

INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
) VALUES (
    'core-search-form-v1', 'core', 'search-form', 1, 'Search Form', 'Displays the site search form.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"placeholder":{"type":"string","maxLength":200,"default":"Search…"},"buttonLabel":{"type":"string","maxLength":100,"default":"Search"},"showLabel":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"search","contexts":["entry","single-template","archive-template","site-part"],"fields":{"settings.placeholder":{"label":"Placeholder","control":"text","group":"Content"},"settings.buttonLabel":{"label":"Button label","control":"text","group":"Content"},"settings.showLabel":{"label":"Show label","control":"checkbox","group":"Content"}}}}',
    'template',
    '<form method="get" action="/search" class="stratum-search-form" role="search">{{ if .Settings.showLabel }}<label for="search-{{ .ID }}">Search</label>{{ else }}<label class="sr-only" for="search-{{ .ID }}">Search</label>{{ end }}<input id="search-{{ .ID }}" name="q" type="search" placeholder="{{ .Settings.placeholder }}"><button type="submit">{{ .Settings.buttonLabel }}</button></form>',
    '.stratum-search-form{display:flex;align-items:end;gap:var(--st-space-sm)}.stratum-search-form input{min-width:0;flex:1;padding:var(--st-space-sm);color:var(--st-color-text);background:transparent;border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm)}',
    'core', 1, unixepoch(), unixepoch()
) ON CONFLICT(namespace, name, version) DO NOTHING;
