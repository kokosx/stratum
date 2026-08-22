INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES
    (
        'core-heading-v1', 'core', 'heading', 1, 'Heading', 'A section heading.',
        '{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""},"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":2}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"heading","fields":{"props.text":{"label":"Text","control":"textarea"},"props.level":{"label":"Level","control":"select"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}',
        'template', '{{ if integerEquals .Props.level 1 }}<h1 class="stratum-align-{{ .Settings.align }}">{{ .Props.text }}</h1>{{ else if integerEquals .Props.level 3 }}<h3 class="stratum-align-{{ .Settings.align }}">{{ .Props.text }}</h3>{{ else if integerEquals .Props.level 4 }}<h4 class="stratum-align-{{ .Settings.align }}">{{ .Props.text }}</h4>{{ else if integerEquals .Props.level 5 }}<h5 class="stratum-align-{{ .Settings.align }}">{{ .Props.text }}</h5>{{ else if integerEquals .Props.level 6 }}<h6 class="stratum-align-{{ .Settings.align }}">{{ .Props.text }}</h6>{{ else }}<h2 class="stratum-align-{{ .Settings.align }}">{{ .Props.text }}</h2>{{ end }}',
        '.stratum-align-left{text-align:left}.stratum-align-center{text-align:center}.stratum-align-right{text-align:right}',
        'core', 1, unixepoch(), unixepoch()
    ),
    (
        'core-text-v1', 'core', 'text', 1, 'Text', 'A paragraph of plain text.',
        '{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"tone":{"type":"string","enum":["default","muted","accent"],"default":"default"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"text","fields":{"props.text":{"label":"Text","control":"textarea"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"}}}}',
        'template', '<p class="stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }}">{{ .Props.text }}</p>',
        '.stratum-tone-muted{color:var(--st-color-text-muted,#667085)}.stratum-tone-accent{color:var(--st-color-primary,#175cd3)}',
        'core', 1, unixepoch(), unixepoch()
    ),
    (
        'core-button-v1', 'core', 'button', 1, 'Button', 'A link styled as a button.',
        '{"schemaVersion":1,"props":{"type":"object","required":["label","url"],"properties":{"label":{"type":"string","default":"Button"},"url":{"type":"string","default":"#"}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["primary","secondary"],"default":"primary"}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"button","fields":{"props.label":{"label":"Label","control":"text"},"props.url":{"label":"URL","control":"text"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}',
        'template', '<a class="stratum-button stratum-button-{{ .Settings.variant }}" href="{{ .Props.url }}">{{ .Props.label }}</a>',
        '.stratum-button{display:inline-block;padding:var(--st-button-padding-y,.65rem) var(--st-button-padding-x,1rem);border-radius:var(--st-button-radius,.35rem);font-weight:var(--st-button-font-weight,600);text-decoration:none}.stratum-button-primary{background:var(--st-color-primary,#175cd3);color:var(--st-color-primary-contrast,#fff)}.stratum-button-secondary{border:var(--st-border-width,1px) solid currentColor;color:var(--st-color-secondary,currentColor)}',
        'core', 1, unixepoch(), unixepoch()
    ),
    (
        'core-section-v1', 'core', 'section', 1, 'Section', 'A container for grouping blocks.',
        '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"width":{"type":"string","enum":["normal","wide","full"],"default":"normal"},"spacing":{"type":"string","enum":["none","sm","md","lg"],"default":"md"}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"section","fields":{"settings.width":{"label":"Width","control":"segmented","group":"Style"},"settings.spacing":{"label":"Spacing","control":"select","group":"Style"}}}}',
        'template', '<section class="stratum-section stratum-width-{{ .Settings.width }} stratum-spacing-{{ .Settings.spacing }}">{{ .Children }}</section>',
        '.stratum-section{margin-inline:auto}.stratum-width-normal{max-width:var(--st-content-width,720px)}.stratum-width-wide{max-width:var(--st-wide-width,1100px)}.stratum-width-full{max-width:none}.stratum-spacing-none{padding-block:0}.stratum-spacing-sm{padding-block:var(--st-space-lg,1rem)}.stratum-spacing-md{padding-block:var(--st-space-2xl,2rem)}.stratum-spacing-lg{padding-block:var(--st-space-3xl,4rem)}',
        'core', 1, unixepoch(), unixepoch()
    ),
    (
        'core-stack-v1', 'core', 'stack', 1, 'Stack', 'A vertical layout container.',
        '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"gap":{"type":"string","enum":["none","sm","md","lg"],"default":"md"},"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"stack","fields":{"settings.gap":{"label":"Gap","control":"select","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}',
        'template', '<div class="stratum-stack stratum-gap-{{ .Settings.gap }} stratum-align-{{ .Settings.align }}">{{ .Children }}</div>',
        '.stratum-stack{display:flex;flex-direction:column}.stratum-gap-none{gap:0}.stratum-gap-sm{gap:var(--st-space-sm,.5rem)}.stratum-gap-md{gap:var(--st-space-md,1rem)}.stratum-gap-lg{gap:var(--st-space-lg,2rem)}',
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
