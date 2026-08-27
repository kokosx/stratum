-- 057_epic2_blocks.sql
-- EPIC 2 blocks: site-part reference, archive contexts, site logo/navigation
PRAGMA foreign_keys = ON;

-- ============================================================
-- core/site-part : reusable global fragment reference (runtime)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-site-part-v1', 'core', 'site-part', 1, 'Site Part', 'Reusable global section (Header, Footer, or custom part).',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","required":["sitePartId"],"properties":{"sitePartId":{"type":"string","default":""}}},"children":{"mode":"none"},"editor":{"category":"reusable","icon":"site-part","contexts":["entry","layout-template","site-part"],"fields":{"settings.sitePartId":{"label":"Site Part","control":"select","group":"Content","optionsSource":"site-parts"}}}}',
    'runtime',
    '<div class="stratum-site-part"></div>',
    '.stratum-site-part{margin:0}',
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

-- ============================================================
-- core/archive-title : semantic archive title
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-archive-title-v1', 'core', 'archive-title', 1, 'Archive Title', 'The title of the current archive (term name or content type archive).',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":1},"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"heading","contexts":["archive-template"],"fields":{"settings.level":{"label":"Level","control":"select","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}',
    'template',
    '{{ $tag := tagFor .Settings.level }}{{ $o := tagOpen $tag }}{{ $c := tagClose $tag }}{{ $o }} class="stratum-archive-title stratum-align-{{ .Settings.align }}">{{ if .Context.Route.ArchiveTitle }}{{ .Context.Route.ArchiveTitle }}{{ else if .Context.Archive.Title }}{{ .Context.Archive.Title }}{{ else }}<span class="stratum-placeholder">Archive title</span>{{ end }}{{ $c }}',
    '.stratum-archive-title{margin:0}.stratum-placeholder{opacity:.5;font-style:italic}',
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

-- ============================================================
-- core/archive-description : semantic archive description
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-archive-description-v1', 'core', 'archive-description', 1, 'Archive Description', 'The description of the current archive (term description).',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"text","contexts":["archive-template"],"fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}',
    'template',
    '{{ if .Context.Route.ArchiveDescription }}<p class="stratum-archive-description stratum-align-{{ .Settings.align }}">{{ .Context.Route.ArchiveDescription }}</p>{{ else if .Context.Archive.Description }}<p class="stratum-archive-description stratum-align-{{ .Settings.align }}">{{ .Context.Archive.Description }}</p>{{ else }}<p class="stratum-archive-description stratum-placeholder">Archive description</p>{{ end }}',
    '.stratum-archive-description{margin:0;color:var(--st-color-text-muted)}.stratum-placeholder{opacity:.5;font-style:italic}',
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

-- ============================================================
-- core/site-logo : site logo from Site Settings
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-site-logo-v1', 'core', 'site-logo', 1, 'Site Logo', 'The site logo from Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"width":{"type":"integer","default":0},"link":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"image","contexts":["entry","layout-template","site-part","archive-template"],"fields":{"settings.width":{"label":"Max width (px)","control":"number","group":"Style"},"settings.link":{"label":"Link to home","control":"checkbox","group":"Style"}}}}',
    'template',
    '{{ if .Context.Site.LogoURL }}{{ if .Settings.link }}<a href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}" class="stratum-site-logo-link">{{ end }}<img class="stratum-site-logo" src="{{ .Context.Site.LogoURL }}" alt="{{ .Context.Site.Name }}"{{ if .Context.Site.LogoWidth }} width="{{ .Context.Site.LogoWidth }}"{{ end }}{{ if .Context.Site.LogoHeight }} height="{{ .Context.Site.LogoHeight }}"{{ end }}{{ if .Settings.width }} style="max-width:{{ .Settings.width }}px"{{ end }}>{{ if .Settings.link }}</a>{{ end }}{{ else }}<span class="stratum-placeholder">Site logo</span>{{ end }}',
    '.stratum-site-logo{max-width:100%;height:auto;display:block}.stratum-placeholder{opacity:.5;font-style:italic}',
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

-- ============================================================
-- core/navigation : renders assigned navigation menu
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-navigation-v1', 'core', 'navigation', 1, 'Navigation', 'The site navigation menu.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"location":{"type":"string","enum":["primary","footer"],"default":"primary"},"style":{"type":"string","enum":["horizontal","vertical"],"default":"horizontal"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"menu","contexts":["entry","layout-template","site-part","archive-template"],"fields":{"settings.location":{"label":"Menu location","control":"select","group":"Content"},"settings.style":{"label":"Style","control":"select","group":"Style"}}}}',
    'template',
    '{{ $loc := .Settings.location }}{{ if not $loc }}{{ $loc = "primary" }}{{ end }}{{ $menu := index .Context.Navigation $loc }}{{ if $menu }}{{ if $menu.Items }}<nav class="stratum-navigation stratum-navigation--{{ .Settings.style }}" aria-label="{{ $loc }}"><ul>{{ template "menu-items" $menu.Items }}</ul></nav>{{ else }}<span class="stratum-placeholder">Navigation (empty)</span>{{ end }}{{ else }}<span class="stratum-placeholder">Navigation</span>{{ end }}',
    '.stratum-navigation ul{list-style:none;margin:0;padding:0;display:flex;gap:var(--st-space-md)}.stratum-navigation--vertical ul{flex-direction:column}.stratum-placeholder{opacity:.5;font-style:italic}',
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

-- ============================================================
-- Update collection to allow archive-template context
-- ============================================================
UPDATE block_definitions SET schema_json = '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"contentType":{"type":"string","default":"post"},"limit":{"type":"integer","minimum":1,"maximum":20,"default":6},"offset":{"type":"integer","minimum":0,"default":0},"source":{"type":"string","enum":["query","context"],"default":"query"},"excludeCurrent":{"type":"boolean","default":false},"orderBy":{"type":"string","default":"entry.published_at"},"direction":{"type":"string","enum":["asc","desc"],"default":"desc"},"termId":{"type":"string","default":""},"filters":{"type":"array","items":{"type":"object","required":["field","operator"],"properties":{"field":{"type":"string","default":"entry.title"},"operator":{"type":"string","enum":["equals","not_equals","contains","exists","not_exists","greater_than","greater_or_equal","less_than","less_or_equal","is_true","is_false","before","after"],"default":"equals"},"value":{"type":"string","default":""}}}}}},"children":{"mode":"any"},"editor":{"category":"query","icon":"collection","contexts":["entry","layout-template","archive-template"],"fields":{"settings.contentType":{"label":"Content Type","control":"select","group":"Query","optionsSource":"content-types"},"settings.limit":{"label":"Limit","control":"number","group":"Query"},"settings.source":{"label":"Source","control":"select","group":"Query"},"settings.excludeCurrent":{"label":"Exclude current","control":"checkbox","group":"Query"},"settings.orderBy":{"label":"Sort by","control":"select","group":"Query","optionsSource":"entry-fields"},"settings.direction":{"label":"Direction","control":"select","group":"Query"},"settings.termId":{"label":"Taxonomy term ID","control":"text","group":"Query"},"settings.filters":{"label":"Filters","group":"Query"}}}}'
WHERE namespace='core' AND name='collection' AND version=2;

UPDATE block_definitions SET schema_json = replace(schema_json, '"contexts":["entry","layout-template"]', '"contexts":["entry","layout-template","site-part","archive-template"]')
WHERE (namespace='core' AND name='site-name' AND version=1) OR (namespace='core' AND name='site-tagline' AND version=1);
