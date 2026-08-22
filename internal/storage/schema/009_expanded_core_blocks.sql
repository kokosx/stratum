-- Expanded core blocks: update existing (heading, text, button, section, stack)
-- and add new (grid, card, accordion, accordion-item, divider, quote).

-- ============================================================
-- Heading: add visualSize, tone, maxWidth
-- ============================================================
UPDATE block_definitions SET
    display_name = 'Heading',
    description = 'A semantic heading with independent visual sizing.',
    schema_json = '{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""},"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":2}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"visualSize":{"type":"string","enum":["auto","sm","md","lg","xl"],"default":"auto"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"maxWidth":{"type":"string","enum":["normal","wide","none"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"heading","fields":{"props.text":{"label":"Text","control":"textarea"},"props.level":{"label":"Level","control":"select"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.visualSize":{"label":"Visual size","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.maxWidth":{"label":"Max width","control":"segmented","group":"Layout"}}}}',
    template = '{{ if integerEquals .Props.level 1 }}<h1 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h1>{{ else if integerEquals .Props.level 3 }}<h3 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h3>{{ else if integerEquals .Props.level 4 }}<h4 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h4>{{ else if integerEquals .Props.level 5 }}<h5 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h5>{{ else if integerEquals .Props.level 6 }}<h6 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h6>{{ else }}<h2 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h2>{{ end }}',
    styles = '.stratum-heading{margin:0}.stratum-heading-size-sm{font-size:var(--st-h3-size,1.5rem)}.stratum-heading-size-md{font-size:var(--st-h2-size,2rem)}.stratum-heading-size-lg{font-size:var(--st-h1-size,2.5rem)}.stratum-heading-size-xl{font-size:clamp(2.5rem,5vw,4rem)}.stratum-heading-size-auto{}.stratum-maxw-normal{max-width:var(--st-content-width)}.stratum-maxw-wide{max-width:var(--st-wide-width)}.stratum-maxw-none{max-width:none}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'heading' AND version = 1;

-- ============================================================
-- Text: add size, maxWidth
-- ============================================================
UPDATE block_definitions SET
    display_name = 'Text',
    description = 'A paragraph of rich text.',
    schema_json = '{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"maxWidth":{"type":"string","enum":["narrow","normal","wide","none"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"text","fields":{"props.text":{"label":"Text","control":"textarea"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.maxWidth":{"label":"Max width","control":"segmented","group":"Layout"}}}}',
    template = '<p class="stratum-text stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }} stratum-text-size-{{ .Settings.size }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</p>',
    styles = '.stratum-text{margin:0}.stratum-text-size-sm{font-size:var(--st-small-size,.875rem)}.stratum-text-size-md{font-size:inherit}.stratum-text-size-lg{font-size:1.15rem;line-height:1.6}.stratum-maxw-narrow{max-width:38rem}.stratum-maxw-normal{max-width:var(--st-content-width)}.stratum-maxw-wide{max-width:var(--st-wide-width)}.stratum-maxw-none{max-width:none}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'text' AND version = 1;

-- ============================================================
-- Button: add size, width, align, openInNewTab, outline/ghost variants
-- ============================================================
UPDATE block_definitions SET
    display_name = 'Button',
    description = 'A link styled as a button.',
    schema_json = '{"schemaVersion":1,"props":{"type":"object","required":["label","url"],"properties":{"label":{"type":"string","default":"Button"},"url":{"type":"string","default":"#"}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["primary","secondary","outline","ghost"],"default":"primary"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"width":{"type":"string","enum":["auto","full"],"default":"auto"},"align":{"type":"string","enum":["left","center","right"],"default":"left"},"openInNewTab":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"button","fields":{"props.label":{"label":"Label","control":"text"},"props.url":{"label":"URL","control":"text"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.openInNewTab":{"label":"Open in new tab","control":"checkbox","group":"Link"}}}}',
    template = '<div class="stratum-btn-wrap stratum-align-{{ .Settings.align }}{{ if eq .Settings.width "full" }} stratum-btn-wrap-full{{ end }}"><a class="stratum-button stratum-button-{{ .Settings.variant }} stratum-button-size-{{ .Settings.size }}{{ if eq .Settings.width "full" }} stratum-button-full{{ end }}" href="{{ .Props.url }}"{{ if .Settings.openInNewTab }} target="_blank" rel="noopener noreferrer"{{ end }}>{{ .Props.label }}</a></div>',
    styles = '.stratum-btn-wrap{margin:0}.stratum-btn-wrap-full{display:block}.stratum-button{display:inline-block;padding:var(--st-button-padding-y,.65rem) var(--st-button-padding-x,1rem);border:2px solid transparent;border-radius:var(--st-button-radius,.35rem);font-weight:var(--st-button-font-weight,600);text-decoration:none;cursor:pointer}.stratum-button-primary{background:var(--st-color-primary,#175cd3);color:var(--st-color-primary-contrast,#fff)}.stratum-button-primary:hover{background:var(--st-color-primary-hover,#1d4ed8)}.stratum-button-secondary{background:var(--st-color-secondary,#475467);color:var(--st-color-primary-contrast,#fff)}.stratum-button-outline{border-color:var(--st-color-primary,#175cd3);color:var(--st-color-primary,#175cd3);background:transparent}.stratum-button-outline:hover{background:var(--st-color-primary,#175cd3);color:var(--st-color-primary-contrast,#fff)}.stratum-button-ghost{color:var(--st-color-primary,#175cd3);background:transparent}.stratum-button-ghost:hover{background:var(--st-color-surface-muted,#f4f6f8)}.stratum-button-size-sm{padding:.4rem .75rem;font-size:var(--st-small-size,.875rem)}.stratum-button-size-lg{padding:.85rem 1.5rem;font-size:1.1rem}.stratum-button-full{display:block;width:100%;text-align:center}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'button' AND version = 1;

-- ============================================================
-- Section: expanded settings
-- ============================================================
UPDATE block_definitions SET
    display_name = 'Section',
    description = 'A semantic page section wrapper.',
    schema_json = '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"width":{"type":"string","enum":["content","wide","full"],"default":"content"},"verticalSpacing":{"type":"string","enum":["none","sm","md","lg","xl"],"default":"md"},"horizontalPadding":{"type":"string","enum":["none","sm","md","lg"],"default":"md"},"align":{"type":"string","enum":["left","center"],"default":"left"},"background":{"type":"string","enum":["default","surface","muted","primary"],"default":"default"},"minHeight":{"type":"string","enum":["auto","small","medium","screen"],"default":"auto"},"anchorID":{"type":"string","default":""}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"section","fields":{"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.verticalSpacing":{"label":"Vertical spacing","control":"select","group":"Layout"},"settings.horizontalPadding":{"label":"Horizontal padding","control":"select","group":"Layout"},"settings.align":{"label":"Content alignment","control":"segmented","group":"Style"},"settings.background":{"label":"Background","control":"select","group":"Style"},"settings.minHeight":{"label":"Min height","control":"select","group":"Layout"},"settings.anchorID":{"label":"Anchor ID","control":"text","group":"Advanced"}}}}',
    template = '<section{{ if .Settings.anchorID }} id="{{ .Settings.anchorID }}"{{ end }} class="stratum-section stratum-section-width-{{ .Settings.width }} stratum-section-vspace-{{ .Settings.verticalSpacing }} stratum-section-hpad-{{ .Settings.horizontalPadding }} stratum-section-align-{{ .Settings.align }} stratum-section-bg-{{ .Settings.background }} stratum-section-minh-{{ .Settings.minHeight }}">{{ .Children }}</section>',
    styles = '.stratum-section{margin-inline:auto}.stratum-section-width-content{max-width:var(--st-content-width)}.stratum-section-width-wide{max-width:var(--st-wide-width)}.stratum-section-width-full{max-width:none}.stratum-section-vspace-none{padding-block:0}.stratum-section-vspace-sm{padding-block:var(--st-space-lg)}.stratum-section-vspace-md{padding-block:var(--st-space-2xl)}.stratum-section-vspace-lg{padding-block:var(--st-space-3xl)}.stratum-section-vspace-xl{padding-block:clamp(4rem,8vw,8rem)}.stratum-section-hpad-none{padding-inline:0}.stratum-section-hpad-sm{padding-inline:var(--st-page-padding)}.stratum-section-hpad-md{padding-inline:var(--st-page-padding)}.stratum-section-hpad-lg{padding-inline:clamp(2rem,5vw,6rem)}.stratum-section-align-left{text-align:left}.stratum-section-align-center{text-align:center}.stratum-section-bg-default{background:transparent}.stratum-section-bg-surface{background:var(--st-color-surface)}.stratum-section-bg-muted{background:var(--st-color-surface-muted)}.stratum-section-bg-primary{background:var(--st-color-primary);color:var(--st-color-primary-contrast)}.stratum-section-minh-auto{min-height:auto}.stratum-section-minh-small{min-height:30vh}.stratum-section-minh-medium{min-height:50vh}.stratum-section-minh-screen{min-height:100vh}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'section' AND version = 1;

-- ============================================================
-- Stack: expanded settings
-- ============================================================
UPDATE block_definitions SET
    display_name = 'Stack',
    description = 'A flexbox layout container.',
    schema_json = '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"direction":{"type":"string","enum":["vertical","horizontal"],"default":"vertical"},"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"md"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"stretch"},"justify":{"type":"string","enum":["start","center","end","between"],"default":"start"},"wrap":{"type":"boolean","default":false},"width":{"type":"string","enum":["auto","full"],"default":"auto"}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"stack","fields":{"settings.direction":{"label":"Direction","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Cross axis","control":"segmented","group":"Layout"},"settings.justify":{"label":"Main axis","control":"segmented","group":"Layout"},"settings.wrap":{"label":"Wrap","control":"checkbox","group":"Layout"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"}}}}',
    template = '<div class="stratum-stack stratum-stack-dir-{{ .Settings.direction }} stratum-stack-gap-{{ .Settings.gap }} stratum-stack-align-{{ .Settings.align }} stratum-stack-justify-{{ .Settings.justify }}{{ if .Settings.wrap }} stratum-stack-wrap{{ end }}{{ if eq .Settings.width "full" }} stratum-stack-full{{ end }}">{{ .Children }}</div>',
    styles = '.stratum-stack{display:flex}.stratum-stack-dir-vertical{flex-direction:column}.stratum-stack-dir-horizontal{flex-direction:row}.stratum-stack-gap-none{gap:0}.stratum-stack-gap-xs{gap:var(--st-space-xs)}.stratum-stack-gap-sm{gap:var(--st-space-sm)}.stratum-stack-gap-md{gap:var(--st-space-md)}.stratum-stack-gap-lg{gap:var(--st-space-lg)}.stratum-stack-gap-xl{gap:var(--st-space-xl)}.stratum-stack-align-start{align-items:flex-start}.stratum-stack-align-center{align-items:center}.stratum-stack-align-end{align-items:flex-end}.stratum-stack-align-stretch{align-items:stretch}.stratum-stack-justify-start{justify-content:flex-start}.stratum-stack-justify-center{justify-content:center}.stratum-stack-justify-end{justify-content:flex-end}.stratum-stack-justify-between{justify-content:space-between}.stratum-stack-wrap{flex-wrap:wrap}.stratum-stack-full{width:100%}@media(max-width:640px){.stratum-stack-dir-horizontal{flex-direction:column}}',
    updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'stack' AND version = 1;

-- ============================================================
-- Grid (new)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-grid-v1', 'core', 'grid', 1, 'Grid', 'A CSS grid layout container.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"columns":{"type":"integer","enum":[1,2,3,4],"default":2},"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"md"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"stretch"},"equalHeight":{"type":"boolean","default":false}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"grid","fields":{"settings.columns":{"label":"Columns","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Align","control":"segmented","group":"Layout"},"settings.equalHeight":{"label":"Equal height","control":"checkbox","group":"Layout"}}}}',
    'template',
    '<div class="stratum-grid stratum-grid-cols-{{ .Settings.columns }} stratum-grid-gap-{{ .Settings.gap }} stratum-grid-align-{{ .Settings.align }}{{ if .Settings.equalHeight }} stratum-grid-equal{{ end }}">{{ .Children }}</div>',
    '.stratum-grid{display:grid}.stratum-grid-cols-1{grid-template-columns:1fr}.stratum-grid-cols-2{grid-template-columns:repeat(2,1fr)}.stratum-grid-cols-3{grid-template-columns:repeat(3,1fr)}.stratum-grid-cols-4{grid-template-columns:repeat(4,1fr)}.stratum-grid-gap-none{gap:0}.stratum-grid-gap-xs{gap:var(--st-space-xs)}.stratum-grid-gap-sm{gap:var(--st-space-sm)}.stratum-grid-gap-md{gap:var(--st-space-md)}.stratum-grid-gap-lg{gap:var(--st-space-lg)}.stratum-grid-gap-xl{gap:var(--st-space-xl)}.stratum-grid-align-start{align-items:start}.stratum-grid-align-center{align-items:center}.stratum-grid-align-end{align-items:end}.stratum-grid-align-stretch{align-items:stretch}.stratum-grid-equal>*{min-height:0}@media(max-width:800px){.stratum-grid-cols-3,.stratum-grid-cols-4{grid-template-columns:repeat(2,1fr)}}@media(max-width:500px){.stratum-grid-cols-2,.stratum-grid-cols-3,.stratum-grid-cols-4{grid-template-columns:1fr}}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Card (new)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-card-v1', 'core', 'card', 1, 'Card', 'A container block for grouping related content.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["default","outlined","elevated","muted"],"default":"default"},"padding":{"type":"string","enum":["sm","md","lg"],"default":"md"},"radius":{"type":"string","enum":["sm","md","lg"],"default":"md"},"align":{"type":"string","enum":["start","center","end"],"default":"start"}}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"card","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.padding":{"label":"Padding","control":"segmented","group":"Style"},"settings.radius":{"label":"Radius","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}',
    'template',
    '<div class="stratum-card stratum-card-{{ .Settings.variant }} stratum-card-pad-{{ .Settings.padding }} stratum-card-radius-{{ .Settings.radius }} stratum-card-align-{{ .Settings.align }}">{{ .Children }}</div>',
    '.stratum-card{background:var(--st-color-surface);overflow:hidden}.stratum-card-default{border:var(--st-border-width) var(--st-border-style) var(--st-color-border)}.stratum-card-outlined{border:2px solid var(--st-color-border)}.stratum-card-elevated{border:none;box-shadow:var(--st-shadow-md)}.stratum-card-muted{background:var(--st-color-surface-muted);border:none}.stratum-card-pad-sm{padding:var(--st-space-sm)}.stratum-card-pad-md{padding:var(--st-space-lg)}.stratum-card-pad-lg{padding:var(--st-space-2xl)}.stratum-card-radius-sm{border-radius:var(--st-radius-sm)}.stratum-card-radius-md{border-radius:var(--st-radius-md)}.stratum-card-radius-lg{border-radius:var(--st-radius-lg)}.stratum-card-align-start{text-align:left}.stratum-card-align-center{text-align:center}.stratum-card-align-end{text-align:right}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Accordion (new)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-accordion-v1', 'core', 'accordion', 1, 'Accordion', 'A collapsible list of items.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["minimal","bordered","cards"],"default":"minimal"}}},"children":{"mode":"allowed","blocks":["core/accordion-item"],"min":1},"editor":{"category":"design","icon":"accordion","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}',
    'template',
    '<div class="stratum-accordion stratum-accordion-{{ .Settings.variant }}">{{ .Children }}</div>',
    '.stratum-accordion{display:flex;flex-direction:column}.stratum-accordion-minimal{gap:0}.stratum-accordion-bordered{gap:0;border:var(--st-border-width) var(--st-border-style) var(--st-color-border);border-radius:var(--st-radius-md);overflow:hidden}.stratum-accordion-cards{gap:var(--st-space-sm)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Accordion Item (new)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-accordion-item-v1', 'core', 'accordion-item', 1, 'Accordion Item', 'A single item inside an accordion.',
    '{"schemaVersion":1,"props":{"type":"object","required":["title"],"properties":{"title":{"type":"string","default":"Item"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"accordion-item","fields":{"props.title":{"label":"Title","control":"text"}}}}',
    'template',
    '<details class="stratum-accordion-item"><summary class="stratum-accordion-trigger">{{ .Props.title }}</summary><div class="stratum-accordion-content">{{ .Children }}</div></details>',
    '.stratum-accordion-item{border-bottom:var(--st-border-width) var(--st-border-style) var(--st-color-border)}.stratum-accordion-bordered .stratum-accordion-item{border-bottom:var(--st-border-width) var(--st-border-style) var(--st-color-border)}.stratum-accordion-bordered .stratum-accordion-item:last-child{border-bottom:none}.stratum-accordion-cards .stratum-accordion-item{border:var(--st-border-width) var(--st-border-style) var(--st-color-border);border-radius:var(--st-radius-md);padding:var(--st-space-sm) var(--st-space-md)}.stratum-accordion-cards .stratum-accordion-item[open]{background:var(--st-color-surface-muted)}.stratum-accordion-trigger{display:flex;align-items:center;justify-content:space-between;padding:var(--st-space-md) 0;cursor:pointer;font-weight:var(--st-heading-weight,600);list-style:none}.stratum-accordion-trigger::-webkit-details-marker{display:none}.stratum-accordion-trigger::after{content:"";width:.5em;height:.5em;border-right:2px solid currentColor;border-bottom:2px solid currentColor;transform:rotate(45deg);transition:transform .2s ease}.stratum-accordion-item[open]>.stratum-accordion-trigger::after{transform:rotate(-135deg)}.stratum-accordion-content{padding:0 0 var(--st-space-md)}.stratum-accordion-cards .stratum-accordion-trigger{padding:var(--st-space-sm) 0}.stratum-accordion-cards .stratum-accordion-content{padding:0 0 var(--st-space-sm)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Divider (new)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-divider-v1', 'core', 'divider', 1, 'Divider', 'A visual separator between blocks.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"style":{"type":"string","enum":["solid","dashed"],"default":"solid"},"width":{"type":"string","enum":["content","full"],"default":"full"},"spacing":{"type":"string","enum":["sm","md","lg"],"default":"md"}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"divider","fields":{"settings.style":{"label":"Style","control":"segmented","group":"Style"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.spacing":{"label":"Spacing","control":"select","group":"Layout"}}}}',
    'template',
    '<hr class="stratum-divider stratum-divider-{{ .Settings.style }} stratum-divider-width-{{ .Settings.width }} stratum-divider-space-{{ .Settings.spacing }}">',
    '.stratum-divider{border:none;border-top:var(--st-border-width) var(--st-border-style) var(--st-color-border);margin-inline:auto}.stratum-divider-dashed{border-top-style:dashed}.stratum-divider-width-content{max-width:var(--st-content-width)}.stratum-divider-width-full{max-width:none}.stratum-divider-space-sm{margin-block:var(--st-space-lg)}.stratum-divider-space-md{margin-block:var(--st-space-2xl)}.stratum-divider-space-lg{margin-block:var(--st-space-3xl)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Quote (new)
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-quote-v1', 'core', 'quote', 1, 'Quote', 'A blockquote with optional citation.',
    '{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""},"citation":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"style":{"type":"string","enum":["simple","bordered","large"],"default":"simple"},"align":{"type":"string","enum":["left","center"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"quote","fields":{"props.text":{"label":"Quote text","control":"textarea"},"props.citation":{"label":"Citation","control":"text"},"settings.style":{"label":"Style","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}',
    'template',
    '<blockquote class="stratum-quote stratum-quote-{{ .Settings.style }} stratum-quote-align-{{ .Settings.align }}"><p class="stratum-quote-text">{{ .Props.text }}</p>{{ if .Props.citation }}<cite class="stratum-quote-citation">{{ .Props.citation }}</cite>{{ end }}</blockquote>',
    '.stratum-quote{margin:0;padding:0}.stratum-quote-simple{border:none;padding-left:var(--st-space-lg);border-left:3px solid var(--st-color-border)}.stratum-quote-bordered{border-left:4px solid var(--st-color-primary);padding-left:var(--st-space-lg)}.stratum-quote-large{border:none;text-align:center;padding:var(--st-space-2xl) var(--st-space-lg)}.stratum-quote-large .stratum-quote-text{font-size:var(--st-h2-size,1.5rem);font-family:var(--st-font-heading);line-height:1.3}.stratum-quote-align-left{text-align:left}.stratum-quote-align-center{text-align:center}.stratum-quote-text{margin:0}.stratum-quote-citation{display:block;margin-top:var(--st-space-sm);font-style:normal;color:var(--st-color-text-muted)}.stratum-quote-large .stratum-quote-citation{margin-top:var(--st-space-md)}',
    'core', 1, unixepoch(), unixepoch()
);
