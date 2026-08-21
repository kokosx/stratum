-- Stage 2 content blocks: Callout, Code, Tabs, Tab.

-- ============================================================
-- Callout: emphasized informational message
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-callout-v1', 'core', 'callout', 1, 'Callout', 'An emphasized message with an icon and tone.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"title":{"type":"string","default":""},"text":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["info","success","warning","error"],"default":"info"},"icon":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"callout","fields":{"props.title":{"label":"Title","control":"text","group":"Content"},"props.text":{"label":"Message","control":"textarea","group":"Content"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.icon":{"label":"Show icon","control":"checkbox","group":"Style"}}}}',
    'template',
    '<aside class="stratum-callout stratum-callout-{{ .Settings.variant }}">{{ if .Settings.icon }}<span class="stratum-callout-mark">{{ if eq .Settings.variant "info" }}{{ icon "info" }}{{ else if eq .Settings.variant "success" }}{{ icon "check" }}{{ else if eq .Settings.variant "warning" }}{{ icon "warning" }}{{ else }}{{ icon "x" }}{{ end }}</span>{{ end }}<div class="stratum-callout-content">{{ if .Props.title }}<p class="stratum-callout-title">{{ .Props.title }}</p>{{ end }}<div class="stratum-callout-body">{{ .Props.text }}</div></div></aside>',
    '.stratum-callout{display:flex;gap:var(--st-space-sm);padding:var(--st-space-md);border-radius:var(--st-radius-md);border-inline-start:4px solid var(--st-color-border);margin:0;background:var(--st-color-surface)}.stratum-callout-info{border-color:var(--st-color-primary);background:color-mix(in srgb,var(--st-color-primary) 8%,transparent)}.stratum-callout-success{border-color:#16a34a;background:color-mix(in srgb,#16a34a 8%,transparent)}.stratum-callout-warning{border-color:#d97706;background:color-mix(in srgb,#d97706 8%,transparent)}.stratum-callout-error{border-color:#dc2626;background:color-mix(in srgb,#dc2626 8%,transparent)}.stratum-callout-mark{flex:0 0 auto;display:inline-flex;align-items:center}.stratum-callout-title{margin:0 0 .25rem;font-weight:600}.stratum-callout-body{margin:0}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Code: syntax-neutral code block with optional copy button
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-code-v1', 'core', 'code', 1, 'Code', 'A code snippet with optional filename and copy button.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"code":{"type":"string","default":""},"filename":{"type":"string","default":""},"language":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"showLineNumbers":{"type":"boolean","default":false},"copyButton":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"code","fields":{"props.code":{"label":"Code","control":"textarea","group":"Content"},"props.filename":{"label":"Filename","control":"text","group":"Content"},"props.language":{"label":"Language","control":"text","group":"Content"},"settings.showLineNumbers":{"label":"Line numbers","control":"checkbox","group":"Style"},"settings.copyButton":{"label":"Copy button","control":"checkbox","group":"Style"}}}}',
    'template',
    '<figure class="stratum-code{{ if .Settings.showLineNumbers }} stratum-code-lines{{ end }}">{{ if .Props.filename }}<figcaption class="stratum-code-filename">{{ .Props.filename }}</figcaption>{{ end }}<pre class="stratum-code-pre"><code{{ if .Props.language }} data-lang="{{ .Props.language }}"{{ end }}>{{ .Props.code }}</code></pre>{{ if .Settings.copyButton }}<button type="button" class="stratum-code-copy" data-copy-code aria-label="Copy code">Copy</button>{{ end }}</figure>',
    '.stratum-code{position:relative;margin:0;background:var(--st-color-surface-muted,#f4f6f8);border:1px solid var(--st-color-border);border-radius:var(--st-radius-md);overflow:hidden}.stratum-code-filename{padding:.4rem .75rem;font-size:var(--st-small-size,.875rem);background:var(--st-color-surface);border-bottom:1px solid var(--st-color-border)}.stratum-code-pre{margin:0;padding:.75rem 1rem;overflow:auto}.stratum-code pre,.stratum-code code{font-family:var(--st-font-mono,ui-monospace,SFMono-Regular,Menlo,monospace);font-size:.875rem}.stratum-code-copy{position:absolute;top:.4rem;right:.4rem;padding:.25rem .5rem;font-size:.75rem;border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm);background:var(--st-color-surface);cursor:pointer}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Tabs: accessible tab container
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-tabs-v1', 'core', 'tabs', 1, 'Tabs', 'A set of switchable tab panels.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["underline","boxed"],"default":"underline"}}},"children":{"mode":"allowed","blocks":["core/tab"],"min":1},"editor":{"category":"design","icon":"tabs","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}',
    'template',
    '<div class="stratum-tabs stratum-tabs-{{ .Settings.variant }}" data-tabs><div class="stratum-tabs-nav" role="tablist" data-tabs-nav></div>{{ .Children }}</div>',
    '.stratum-tabs-nav{display:flex;gap:.25rem;flex-wrap:wrap;border-bottom:1px solid var(--st-color-border);margin-bottom:var(--st-space-md)}.stratum-tab-btn{padding:.5rem .75rem;border:1px solid transparent;border-bottom:none;background:transparent;cursor:pointer;font:inherit}.stratum-tab-btn[aria-selected="true"]{border-color:var(--st-color-border);background:var(--st-color-surface);border-radius:var(--st-radius-sm) var(--st-radius-sm) 0 0}.stratum-tabs-boxed .stratum-tab-btn{border:1px solid var(--st-color-border);border-radius:var(--st-radius-sm);margin-bottom:.5rem}.stratum-tabs-boxed .stratum-tab-btn[aria-selected="true"]{background:var(--st-color-primary);color:var(--st-color-primary-contrast)}.stratum-tab[hidden]{display:none}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Tab: a single tab panel inside Tabs
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-tab-v1', 'core', 'tab', 1, 'Tab', 'A single panel within a Tabs block.',
    '{"schemaVersion":1,"props":{"type":"object","required":["label"],"properties":{"label":{"type":"string","default":"Tab"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"tab","fields":{"props.label":{"label":"Tab label","control":"text"}}}}',
    'template',
    '<section class="stratum-tab" data-label="{{ .Props.label }}">{{ .Children }}</section>',
    '',
    'core', 1, unixepoch(), unixepoch()
);
