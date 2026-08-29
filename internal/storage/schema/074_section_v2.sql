-- 074_section_v2.sql
-- EPIC 7 Final Art Direction: Section v2 with full-bleed split (outer+inner)
-- Historical Section v1 remains untouched and fully renderable.
PRAGMA foreign_keys = ON;

INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-section-v2', 'core', 'section', 2, 'Section', 'A semantic page section with full-bleed background and constrained inner content.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"width":{"type":"string","enum":["content","wide","full"],"default":"content"},"verticalSpacing":{"type":"string","enum":["none","sm","md","lg","xl"],"default":"md"},"horizontalPadding":{"type":"string","enum":["none","sm","md","lg"],"default":"md"},"align":{"type":"string","enum":["left","center"],"default":"left"},"background":{"type":"string","enum":["default","surface","muted","primary"],"default":"default"},"minHeight":{"type":"string","enum":["auto","small","medium","screen"],"default":"auto"},"anchorID":{"type":"string","default":""}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"section","fields":{"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.verticalSpacing":{"label":"Vertical spacing","control":"select","group":"Layout"},"settings.horizontalPadding":{"label":"Horizontal padding","control":"select","group":"Layout"},"settings.align":{"label":"Content alignment","control":"segmented","group":"Style"},"settings.background":{"label":"Background","control":"select","group":"Style"},"settings.minHeight":{"label":"Min height","control":"select","group":"Layout"},"settings.anchorID":{"label":"Anchor ID","control":"text","group":"Advanced"}}}}',
    'template',
    '<section{{ if .Settings.anchorID }} id="{{ .Settings.anchorID }}"{{ end }} class="stratum-section stratum-section--v2 stratum-section-bg-{{ .Settings.background }} stratum-section-vspace-{{ .Settings.verticalSpacing }} stratum-section-minh-{{ .Settings.minHeight }} stratum-section-align-{{ .Settings.align }}"><div class="stratum-section__inner stratum-section-width-{{ .Settings.width }} stratum-section-hpad-{{ .Settings.horizontalPadding }}">{{ .Children }}</div></section>',
    '.stratum-section--v2{width:100%;max-width:none;margin-inline:0;box-sizing:border-box}.stratum-section--v2.stratum-section-vspace-none{padding-block:0}.stratum-section--v2.stratum-section-vspace-sm{padding-block:clamp(20px,2.5vw,28px)}.stratum-section--v2.stratum-section-vspace-md{padding-block:clamp(36px,4vw,56px)}.stratum-section--v2.stratum-section-vspace-lg{padding-block:clamp(56px,6vw,80px)}.stratum-section--v2.stratum-section-vspace-xl{padding-block:clamp(80px,8vw,112px)}.stratum-section--v2.stratum-section-minh-auto{min-height:auto}.stratum-section--v2.stratum-section-minh-small{min-height:30vh}.stratum-section--v2.stratum-section-minh-medium{min-height:50vh}.stratum-section--v2.stratum-section-minh-screen{min-height:100vh;display:flex;flex-direction:column;justify-content:center}.stratum-section--v2.stratum-section-bg-default{background:transparent}.stratum-section--v2.stratum-section-bg-surface{background:var(--st-color-surface)}.stratum-section--v2.stratum-section-bg-muted{background:var(--st-color-surface-muted)}.stratum-section--v2.stratum-section-bg-primary{background:var(--st-color-primary);color:var(--st-color-primary-contrast)}.stratum-section--v2.stratum-section-bg-primary .stratum-heading{color:var(--st-color-primary-contrast)}.stratum-section--v2.stratum-section-bg-primary .stratum-text{color:var(--st-color-primary-contrast)}.stratum-section--v2.stratum-section-bg-primary .stratum-button-primary{background:var(--st-color-primary-contrast);color:var(--st-color-primary)}.stratum-section--v2.stratum-section-bg-primary .stratum-button-primary:hover{background:color-mix(in srgb,var(--st-color-primary-contrast) 88%,var(--st-color-primary));color:var(--st-color-primary)}.stratum-section__inner{width:100%;margin-inline:auto;box-sizing:border-box;min-width:0}.stratum-section__inner.stratum-section-width-content{max-width:var(--st-content-width)}.stratum-section__inner.stratum-section-width-wide{max-width:var(--st-wide-width)}.stratum-section__inner.stratum-section-width-full{max-width:none}.stratum-section__inner.stratum-section-hpad-none{padding-inline:0}.stratum-section__inner.stratum-section-hpad-sm{padding-inline:var(--st-space-md)}.stratum-section__inner.stratum-section-hpad-md{padding-inline:var(--st-space-lg)}.stratum-section__inner.stratum-section-hpad-lg{padding-inline:clamp(2rem,5vw,6rem)}.stratum-section-align-left{text-align:left}.stratum-section-align-center{text-align:center}',
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
