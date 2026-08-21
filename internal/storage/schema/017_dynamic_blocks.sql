-- Stage 2 dynamic blocks: bind to the current Entry / Site via RenderContext.
-- When rendered without a context (editor preview) they show a placeholder.

-- ============================================================
-- Entry Title
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-entry-title-v1', 'core', 'entry-title', 1, 'Entry Title', 'The title of the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":1},"align":{"type":"string","enum":["left","center","right"],"default":"left"},"visualSize":{"type":"string","enum":["auto","sm","md","lg","xl"],"default":"auto"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"maxWidth":{"type":"string","enum":["normal","wide","none"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"heading","fields":{"settings.level":{"label":"Level","control":"select","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.visualSize":{"label":"Visual size","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.maxWidth":{"label":"Max width","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ $tag := tagFor .Settings.level }}{{ $o := tagOpen $tag }}{{ $c := tagClose $tag }}{{ $o }} class="stratum-entry-title stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ if .Context.Entry.Title }}{{ .Context.Entry.Title }}{{ else }}<span class="stratum-placeholder">Entry title</span>{{ end }}{{ $c }}',
    '.stratum-entry-title{margin:0}.stratum-heading-size-sm{font-size:var(--st-h3-size,1.5rem)}.stratum-heading-size-md{font-size:var(--st-h2-size,2rem)}.stratum-heading-size-lg{font-size:var(--st-h1-size,2.5rem)}.stratum-heading-size-xl{font-size:clamp(2.5rem,5vw,4rem)}.stratum-maxw-normal{max-width:var(--st-content-width)}.stratum-maxw-wide{max-width:var(--st-wide-width)}.stratum-maxw-none{max-width:none}.stratum-tone-muted{color:var(--st-color-text-muted)}.stratum-tone-primary{color:var(--st-color-primary)}.stratum-placeholder{opacity:.5;font-style:italic}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Entry Excerpt
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-entry-excerpt-v1', 'core', 'entry-excerpt', 1, 'Entry Excerpt', 'The summary of the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"text","fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"}}}}',
    'template',
    '<p class="stratum-entry-excerpt stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }} stratum-text-size-{{ .Settings.size }}">{{ if .Context.Entry.Excerpt }}{{ .Context.Entry.Excerpt }}{{ else }}<span class="stratum-placeholder">Entry excerpt</span>{{ end }}</p>',
    '.stratum-entry-excerpt{margin:0}.stratum-text-size-sm{font-size:var(--st-small-size,.875rem)}.stratum-text-size-md{font-size:inherit}.stratum-text-size-lg{font-size:1.15rem;line-height:1.6}.stratum-tone-muted{color:var(--st-color-text-muted)}.stratum-tone-primary{color:var(--st-color-primary)}.stratum-placeholder{opacity:.5;font-style:italic}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Entry Publish Date
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-entry-publish-date-v1', 'core', 'entry-publish-date', 1, 'Publish Date', 'The publication date of the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"format":{"type":"string","enum":["long","iso"],"default":"long"},"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"calendar","fields":{"settings.format":{"label":"Format","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ $d := .Context.Entry.PublishDate }}{{ if eq .Settings.format "iso" }}{{ $d = .Context.Entry.PublishISO }}{{ end }}<time class="stratum-entry-date stratum-align-{{ .Settings.align }}"{{ if .Context.Entry.PublishISO }} datetime="{{ .Context.Entry.PublishISO }}"{{ end }}>{{ if $d }}{{ $d }}{{ else }}<span class="stratum-placeholder">Publish date</span>{{ end }}</time>',
    '.stratum-entry-date{color:var(--st-color-text-muted);font-size:var(--st-small-size,.875rem)}.stratum-align-left{text-align:left}.stratum-align-center{text-align:center}.stratum-align-right{text-align:right}.stratum-placeholder{opacity:.5;font-style:italic}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Site Name
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-site-name-v1', 'core', 'site-name', 1, 'Site Name', 'The site title from Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"link":{"type":"boolean","default":true},"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"site","fields":{"settings.link":{"label":"Link to home","control":"checkbox","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ if .Settings.link }}<a class="stratum-site-name-link" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}<span class="stratum-site-name stratum-align-{{ .Settings.align }}">{{ if .Context.Site.Name }}{{ .Context.Site.Name }}{{ else }}<span class="stratum-placeholder">Site name</span>{{ end }}</span>{{ if .Settings.link }}</a>{{ end }}',
    '.stratum-site-name{font-weight:600}.stratum-align-left{text-align:left}.stratum-align-center{text-align:center}.stratum-align-right{text-align:right}.stratum-placeholder{opacity:.5;font-style:italic}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Site Tagline
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-site-tagline-v1', 'core', 'site-tagline', 1, 'Site Tagline', 'The site tagline from Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"text","fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '<p class="stratum-site-tagline stratum-align-{{ .Settings.align }}">{{ if .Context.Site.Tagline }}{{ .Context.Site.Tagline }}{{ else }}<span class="stratum-placeholder">Site tagline</span>{{ end }}</p>',
    '.stratum-site-tagline{margin:0;color:var(--st-color-text-muted)}.stratum-align-left{text-align:left}.stratum-align-center{text-align:center}.stratum-align-right{text-align:right}.stratum-placeholder{opacity:.5;font-style:italic}',
    'core', 1, unixepoch(), unixepoch()
);
