-- Stage 1 completion: Row, List, Button Group, Icon.
-- (The Image block lives in 015_core_image_block.sql.)
-- Each block is a self-contained block_definition (schema + Go template + scoped CSS).

-- ============================================================
-- Row: horizontal flex layout
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-row-v1', 'core', 'row', 1, 'Row', 'A horizontal flexbox layout container.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"md"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"stretch"},"justify":{"type":"string","enum":["start","center","end","between"],"default":"start"},"wrap":{"type":"boolean","default":false},"reverse":{"type":"boolean","default":false}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"row","fields":{"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Cross axis","control":"segmented","group":"Layout"},"settings.justify":{"label":"Main axis","control":"segmented","group":"Layout"},"settings.wrap":{"label":"Wrap","control":"checkbox","group":"Layout"},"settings.reverse":{"label":"Reverse","control":"checkbox","group":"Layout"}}}}',
    'template',
    '<div class="stratum-row stratum-row-gap-{{ .Settings.gap }} stratum-row-align-{{ .Settings.align }} stratum-row-justify-{{ .Settings.justify }}{{ if .Settings.wrap }} stratum-row-wrap{{ end }}{{ if .Settings.reverse }} stratum-row-reverse{{ end }}">{{ .Children }}</div>',
    '.stratum-row{display:flex;flex-direction:row}.stratum-row-gap-none{gap:0}.stratum-row-gap-xs{gap:var(--st-space-xs)}.stratum-row-gap-sm{gap:var(--st-space-sm)}.stratum-row-gap-md{gap:var(--st-space-md)}.stratum-row-gap-lg{gap:var(--st-space-lg)}.stratum-row-gap-xl{gap:var(--st-space-xl)}.stratum-row-align-start{align-items:flex-start}.stratum-row-align-center{align-items:center}.stratum-row-align-end{align-items:flex-end}.stratum-row-align-stretch{align-items:stretch}.stratum-row-justify-start{justify-content:flex-start}.stratum-row-justify-center{justify-content:center}.stratum-row-justify-end{justify-content:flex-end}.stratum-row-justify-between{justify-content:space-between}.stratum-row-wrap{flex-wrap:wrap}.stratum-row-reverse{flex-direction:row-reverse}@media(max-width:640px){.stratum-row{flex-direction:column}.stratum-row-reverse{flex-direction:column-reverse}}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- List: ordered/unordered list authored one item per line
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-list-v1', 'core', 'list', 1, 'List', 'An ordered or unordered list of items.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"items":{"type":"string","default":"First item\nSecond item"}}},"settings":{"type":"object","properties":{"ordered":{"type":"boolean","default":false},"marker":{"type":"string","enum":["disc","circle","square","check","none"],"default":"disc"},"start":{"type":"integer","default":1}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"list","fields":{"props.items":{"label":"Items (one per line)","control":"textarea","group":"Content"},"settings.ordered":{"label":"Ordered","control":"checkbox","group":"Style"},"settings.marker":{"label":"Marker","control":"select","group":"Style"},"settings.start":{"label":"Start number","control":"number","group":"Style"}}}}',
    'template',
    '{{ $items := lines .Props.items }}{{ if .Settings.ordered }}<ol class="stratum-list stratum-list-ordered stratum-list-marker-{{ .Settings.marker }}"{{ if ne .Settings.start 1.0 }} start="{{ .Settings.start }}"{{ end }}>{{ else }}<ul class="stratum-list stratum-list-marker-{{ .Settings.marker }}">{{ end }}{{ range $items }}<li>{{ . }}</li>{{ end }}{{ if .Settings.ordered }}</ol>{{ else }}</ul>{{ end }}',
    '.stratum-list{margin:0;padding-left:1.4em}.stratum-list-ordered{padding-left:1.6em}.stratum-list-marker-disc{list-style:disc}.stratum-list-marker-circle{list-style:circle}.stratum-list-marker-square{list-style:square}.stratum-list-marker-none{list-style:none;padding-left:0}.stratum-list-marker-check{list-style:none;padding-left:0}.stratum-list-marker-check li{position:relative;padding-left:1.6em}.stratum-list-marker-check li::before{content:"";position:absolute;left:0;top:.4em;width:.5em;height:.28em;border-left:.16em solid var(--st-color-primary);border-bottom:.16em solid var(--st-color-primary);transform:rotate(-45deg)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Button Group: container for buttons
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-button-group-v1', 'core', 'button-group', 1, 'Button Group', 'A group of buttons laid out together.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"direction":{"type":"string","enum":["horizontal","vertical"],"default":"horizontal"},"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"sm"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"start"},"wrap":{"type":"boolean","default":true}}},"children":{"mode":"allowed","blocks":["core/button"],"min":1},"editor":{"category":"design","icon":"button-group","fields":{"settings.direction":{"label":"Direction","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.wrap":{"label":"Wrap","control":"checkbox","group":"Layout"}}}}',
    'template',
    '<div class="stratum-btn-group stratum-btn-group-dir-{{ .Settings.direction }} stratum-btn-group-gap-{{ .Settings.gap }} stratum-btn-group-align-{{ .Settings.align }}{{ if .Settings.wrap }} stratum-btn-group-wrap{{ end }}">{{ .Children }}</div>',
    '.stratum-btn-group{display:flex}.stratum-btn-group-dir-horizontal{flex-direction:row}.stratum-btn-group-dir-vertical{flex-direction:column}.stratum-btn-group-gap-none{gap:0}.stratum-btn-group-gap-xs{gap:var(--st-space-xs)}.stratum-btn-group-gap-sm{gap:var(--st-space-sm)}.stratum-btn-group-gap-md{gap:var(--st-space-md)}.stratum-btn-group-gap-lg{gap:var(--st-space-lg)}.stratum-btn-group-gap-xl{gap:var(--st-space-xl)}.stratum-btn-group-align-start{align-items:flex-start}.stratum-btn-group-align-center{align-items:center}.stratum-btn-group-align-end{align-items:flex-end}.stratum-btn-group-align-stretch{align-items:stretch}.stratum-btn-group-wrap{flex-wrap:wrap}.stratum-btn-group-dir-vertical .stratum-btn-wrap{width:100%}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Icon: controlled inline SVG icon
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-icon-v1', 'core', 'icon', 1, 'Icon', 'A controlled inline icon from the Stratum icon set.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"name":{"type":"string","enum":["arrow-right","check","x","info","warning","star","menu","search","plus","chevron-down","phone","mail","location","link","external","heart"],"default":"check"},"label":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"size":{"type":"string","enum":["sm","md","lg","xl"],"default":"md"},"color":{"type":"string","enum":["default","muted","primary","inherit"],"default":"default"}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"icon","fields":{"props.name":{"label":"Icon","control":"select","group":"Content"},"props.label":{"label":"Accessibility label","control":"text","group":"Content"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.color":{"label":"Color","control":"select","group":"Style"}}}}',
    'template',
    '<span class="stratum-icon stratum-icon-size-{{ .Settings.size }} stratum-icon-color-{{ .Settings.color }}"{{ if .Props.label }} role="img" aria-label="{{ .Props.label }}"{{ else }} aria-hidden="true"{{ end }}>{{ icon .Props.name }}</span>',
    '.stratum-icon{display:inline-flex;align-items:center;justify-content:center;vertical-align:middle;line-height:0}.stratum-icon svg{width:1em;height:1em;display:block;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}.stratum-icon-size-sm{font-size:1rem}.stratum-icon-size-md{font-size:1.25rem}.stratum-icon-size-lg{font-size:1.75rem}.stratum-icon-size-xl{font-size:2.5rem}.stratum-icon-color-default{color:var(--st-color-text)}.stratum-icon-color-muted{color:var(--st-color-text-muted)}.stratum-icon-color-primary{color:var(--st-color-primary)}.stratum-icon-color-inherit{color:inherit}',
    'core', 1, unixepoch(), unixepoch()
);
