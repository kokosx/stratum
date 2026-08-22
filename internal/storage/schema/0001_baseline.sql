
-- ===== 001_initial.sql =====
PRAGMA foreign_keys = ON;


-- ============================================================
-- Content Types
-- ============================================================

CREATE TABLE content_types (
    id TEXT PRIMARY KEY,

    display_name TEXT NOT NULL,
    plural_name TEXT NOT NULL,

    hierarchical INTEGER NOT NULL DEFAULT 0
        CHECK (hierarchical IN (0, 1)),

    public INTEGER NOT NULL DEFAULT 1
        CHECK (public IN (0, 1)),

    config_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(config_json)),

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);


-- ============================================================
-- Entries
-- ============================================================

CREATE TABLE entries (
    id TEXT PRIMARY KEY,

    content_type_id TEXT NOT NULL,

    slug TEXT NOT NULL,

    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'private', 'trash')),

    author_id TEXT,

    published_revision_id TEXT,

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    published_at INTEGER,

    FOREIGN KEY(content_type_id)
        REFERENCES content_types(id)
        ON DELETE RESTRICT,

    FOREIGN KEY(published_revision_id)
        REFERENCES entry_revisions(id)
        ON DELETE SET NULL,

    UNIQUE(content_type_id, slug)
);

CREATE INDEX idx_entries_content_type
    ON entries(content_type_id);

CREATE INDEX idx_entries_status
    ON entries(status);

CREATE INDEX idx_entries_slug
    ON entries(slug);


-- ============================================================
-- Entry Revisions
-- ============================================================

CREATE TABLE entry_revisions (
    id TEXT PRIMARY KEY,

    entry_id TEXT NOT NULL,

    revision_number INTEGER NOT NULL
        CHECK (revision_number > 0),

    title TEXT NOT NULL,

    excerpt TEXT,

    document_json TEXT NOT NULL
        CHECK (json_valid(document_json)),

    seo_title TEXT,
    seo_description TEXT,

    created_by TEXT,

    created_at INTEGER NOT NULL,

    FOREIGN KEY(entry_id)
        REFERENCES entries(id)
        ON DELETE CASCADE,

    UNIQUE(entry_id, revision_number)
);

CREATE INDEX idx_entry_revisions_entry
    ON entry_revisions(entry_id);

CREATE INDEX idx_entry_revisions_created_at
    ON entry_revisions(created_at);


-- ============================================================
-- Block Definitions
-- ============================================================

CREATE TABLE block_definitions (
    id TEXT PRIMARY KEY,

    namespace TEXT NOT NULL,
    name TEXT NOT NULL,

    version INTEGER NOT NULL
        CHECK (version > 0),

    display_name TEXT NOT NULL,
    description TEXT,

    schema_json TEXT NOT NULL
        CHECK (json_valid(schema_json)),

    renderer_type TEXT NOT NULL DEFAULT 'template',

    template TEXT,
    styles TEXT,

    source TEXT NOT NULL,

    enabled INTEGER NOT NULL DEFAULT 1
        CHECK (enabled IN (0, 1)),

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    UNIQUE(namespace, name, version)
);

CREATE INDEX idx_block_definitions_lookup
    ON block_definitions(namespace, name, version);

CREATE INDEX idx_block_definitions_enabled
    ON block_definitions(enabled);


-- ============================================================
-- Routes
-- ============================================================

CREATE TABLE routes (
    id TEXT PRIMARY KEY,

    path TEXT NOT NULL UNIQUE,

    entry_id TEXT,

    route_type TEXT NOT NULL
        CHECK (
            route_type IN (
                'entry',
                'archive',
                'redirect',
                'system'
            )
        ),

    redirect_to TEXT,

    redirect_status INTEGER
        CHECK (
            redirect_status IS NULL
            OR redirect_status IN (301, 302, 307, 308)
        ),

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    FOREIGN KEY(entry_id)
        REFERENCES entries(id)
        ON DELETE CASCADE
);

CREATE INDEX idx_routes_entry
    ON routes(entry_id);


-- ============================================================
-- Site Settings
-- ============================================================

CREATE TABLE site_settings (
    id INTEGER PRIMARY KEY
        CHECK (id = 1),

    site_title TEXT NOT NULL DEFAULT 'Stratum',

    site_tagline TEXT NOT NULL DEFAULT '',

    homepage_mode TEXT NOT NULL DEFAULT 'latest_posts'
        CHECK (
            homepage_mode IN (
                'latest_posts',
                'page'
            )
        ),

    homepage_entry_id TEXT,

    posts_page_entry_id TEXT,

    posts_per_page INTEGER NOT NULL DEFAULT 10
        CHECK (posts_per_page > 0),

    language TEXT NOT NULL DEFAULT 'en',

    timezone TEXT NOT NULL DEFAULT 'UTC',

    active_theme TEXT NOT NULL DEFAULT 'stratum/default',

    indexing_enabled INTEGER NOT NULL DEFAULT 1
        CHECK (indexing_enabled IN (0, 1)),

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    FOREIGN KEY(homepage_entry_id)
        REFERENCES entries(id)
        ON DELETE SET NULL,

    FOREIGN KEY(posts_page_entry_id)
        REFERENCES entries(id)
        ON DELETE SET NULL
);


-- ============================================================
-- Seed: Core Content Types
-- ============================================================

INSERT INTO content_types (
    id,
    display_name,
    plural_name,
    hierarchical,
    public,
    config_json,
    created_at,
    updated_at
)
VALUES
    (
        'page',
        'Page',
        'Pages',
        1,
        1,
        '{}',
        unixepoch(),
        unixepoch()
    ),
    (
        'post',
        'Post',
        'Posts',
        0,
        1,
        '{}',
        unixepoch(),
        unixepoch()
    );


-- ============================================================
-- Seed: Site Settings
-- ============================================================

INSERT INTO site_settings (
    id,
    created_at,
    updated_at
)
VALUES (
    1,
    unixepoch(),
    unixepoch()
);

-- ===== 002_auth.sql =====
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,

    FOREIGN KEY(user_id)
        REFERENCES users(id)
        ON DELETE CASCADE
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

-- ===== 003_core_blocks.sql =====
INSERT INTO block_definitions (
    id,
    namespace,
    name,
    version,
    display_name,
    schema_json,
    renderer_type,
    template,
    source,
    enabled,
    created_at,
    updated_at
)
VALUES
    (
        'core-heading-v1',
        'core',
        'heading',
        1,
        'Heading',
        '{"type":"object","required":["text"],"properties":{"text":{"type":"string"},"level":{"type":"integer","minimum":1,"maximum":6}}}',
        'template',
        '<h2>{{ .Props.text }}</h2>',
        'core',
        1,
        unixepoch(),
        unixepoch()
    ),
    (
        'core-text-v1',
        'core',
        'text',
        1,
        'Text',
        '{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}',
        'template',
        '<p>{{ .Props.text }}</p>',
        'core',
        1,
        unixepoch(),
        unixepoch()
    )
ON CONFLICT(namespace, name, version) DO UPDATE SET
    display_name = excluded.display_name,
    schema_json = excluded.schema_json,
    renderer_type = excluded.renderer_type,
    template = excluded.template,
    source = excluded.source,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at;

-- ===== 004_block_schema_v1.sql =====
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

-- ===== 005_navigation.sql =====
PRAGMA foreign_keys = ON;

CREATE TABLE navigation_menus (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug TEXT NOT NULL UNIQUE CHECK (length(trim(slug)) > 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE navigation_items (
    id TEXT PRIMARY KEY,
    menu_id TEXT NOT NULL,
    parent_id TEXT,
    position INTEGER NOT NULL CHECK (position >= 0),
    label TEXT NOT NULL CHECK (length(trim(label)) > 0),
    target_type TEXT NOT NULL CHECK (target_type IN ('entry', 'url')),
    entry_id TEXT,
    url TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(menu_id) REFERENCES navigation_menus(id) ON DELETE CASCADE,
    FOREIGN KEY(parent_id) REFERENCES navigation_items(id) ON DELETE CASCADE,
    -- entry_id deliberately has no FK: a deleted Entry leaves an editable,
    -- visibly broken admin item while the public loader suppresses it.
    CHECK (
        (target_type = 'entry' AND entry_id IS NOT NULL AND url IS NULL)
        OR
        (target_type = 'url' AND entry_id IS NULL AND url IS NOT NULL AND length(trim(url)) > 0)
    ),
    UNIQUE(menu_id, parent_id, position)
);

CREATE INDEX idx_navigation_items_menu ON navigation_items(menu_id);
CREATE INDEX idx_navigation_items_parent ON navigation_items(parent_id);
CREATE INDEX idx_navigation_items_entry ON navigation_items(entry_id);

CREATE TABLE navigation_locations (
    location TEXT PRIMARY KEY CHECK (length(trim(location)) > 0),
    menu_id TEXT NOT NULL,
    FOREIGN KEY(menu_id) REFERENCES navigation_menus(id) ON DELETE CASCADE
);

CREATE INDEX idx_navigation_locations_menu ON navigation_locations(menu_id);

INSERT INTO navigation_menus (id, name, slug, created_at, updated_at) VALUES
    ('default-main-navigation', 'Main Navigation', 'main-navigation', unixepoch(), unixepoch()),
    ('default-footer-navigation', 'Footer', 'footer', unixepoch(), unixepoch());

INSERT INTO navigation_locations (location, menu_id) VALUES
    ('primary', 'default-main-navigation'),
    ('footer', 'default-footer-navigation');

-- ===== 006_navigation_groups.sql =====
CREATE TABLE navigation_items_new (
    id TEXT PRIMARY KEY,
    menu_id TEXT NOT NULL,
    parent_id TEXT,
    position INTEGER NOT NULL CHECK (position >= 0),
    label TEXT NOT NULL CHECK (length(trim(label)) > 0),
    target_type TEXT NOT NULL CHECK (target_type IN ('entry', 'url', 'group')),
    entry_id TEXT,
    url TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(menu_id) REFERENCES navigation_menus(id) ON DELETE CASCADE,
    FOREIGN KEY(parent_id) REFERENCES navigation_items_new(id) ON DELETE CASCADE,
    CHECK (
        (target_type = 'entry' AND entry_id IS NOT NULL AND url IS NULL)
        OR
        (target_type = 'url' AND entry_id IS NULL AND url IS NOT NULL AND length(trim(url)) > 0)
        OR
        (target_type = 'group' AND entry_id IS NULL AND url IS NULL)
    ),
    UNIQUE(menu_id, parent_id, position)
);

INSERT INTO navigation_items_new (
    id, menu_id, parent_id, position, label, target_type,
    entry_id, url, created_at, updated_at
)
SELECT
    id, menu_id, parent_id, position, label, target_type,
    entry_id, url, created_at, updated_at
FROM navigation_items;

DROP TABLE navigation_items;
ALTER TABLE navigation_items_new RENAME TO navigation_items;

CREATE INDEX idx_navigation_items_menu ON navigation_items(menu_id);
CREATE INDEX idx_navigation_items_parent ON navigation_items(parent_id);
CREATE INDEX idx_navigation_items_entry ON navigation_items(entry_id);

-- ===== 007_theme_customizations.sql =====
CREATE TABLE theme_customizations (
    theme_id TEXT PRIMARY KEY CHECK (length(trim(theme_id)) > 0),
    theme_version INTEGER NOT NULL CHECK (theme_version >= 0),
    settings_json TEXT NOT NULL CHECK (json_valid(settings_json) AND json_type(settings_json) = 'object'),
    custom_css TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

UPDATE site_settings
SET active_theme = 'stratum/default', updated_at = unixepoch()
WHERE active_theme = 'default';

INSERT INTO theme_customizations (theme_id, theme_version, settings_json, custom_css, updated_at)
VALUES ('stratum/default', 1, '{}', '', unixepoch())
ON CONFLICT(theme_id) DO NOTHING;

-- ===== 008_theme_block_tokens.sql =====
UPDATE block_definitions SET styles = '.stratum-tone-muted{color:var(--st-color-text-muted,#667085)}.stratum-tone-accent{color:var(--st-color-primary,#175cd3)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'text' AND version = 1;

UPDATE block_definitions SET styles = '.stratum-button{display:inline-block;padding:var(--st-button-padding-y,.65rem) var(--st-button-padding-x,1rem);border-radius:var(--st-button-radius,.35rem);font-weight:var(--st-button-font-weight,600);text-decoration:none}.stratum-button-primary{background:var(--st-color-primary,#175cd3);color:var(--st-color-primary-contrast,#fff)}.stratum-button-secondary{border:var(--st-border-width,1px) solid currentColor;color:var(--st-color-secondary,currentColor)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'button' AND version = 1;

UPDATE block_definitions SET styles = '.stratum-section{margin-inline:auto}.stratum-width-normal{max-width:var(--st-content-width,720px)}.stratum-width-wide{max-width:var(--st-wide-width,1100px)}.stratum-width-full{max-width:none}.stratum-spacing-none{padding-block:0}.stratum-spacing-sm{padding-block:var(--st-space-lg,1rem)}.stratum-spacing-md{padding-block:var(--st-space-2xl,2rem)}.stratum-spacing-lg{padding-block:var(--st-space-3xl,4rem)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'section' AND version = 1;

UPDATE block_definitions SET styles = '.stratum-stack{display:flex;flex-direction:column}.stratum-gap-none{gap:0}.stratum-gap-sm{gap:var(--st-space-sm,.5rem)}.stratum-gap-md{gap:var(--st-space-md,1rem)}.stratum-gap-lg{gap:var(--st-space-lg,2rem)}', updated_at = unixepoch()
WHERE namespace = 'core' AND name = 'stack' AND version = 1;

-- ===== 009_expanded_core_blocks.sql =====
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

-- ===== 010_seo.sql =====
-- SEO layer: per-revision canonical URL and the site's canonical public origin.
ALTER TABLE entry_revisions ADD COLUMN canonical_url TEXT;
ALTER TABLE site_settings ADD COLUMN site_url TEXT NOT NULL DEFAULT '';

-- ===== 011_site_settings_runtime.sql =====
-- Site runtime settings: sitemap, robots, speculation rules, and a title
-- separator. These are first-party, explicitly typed settings stored on the
-- site_settings singleton. They are NOT a generic key/value options store.
ALTER TABLE site_settings ADD COLUMN sitemap_enabled INTEGER NOT NULL DEFAULT 1
    CHECK (sitemap_enabled IN (0, 1));

ALTER TABLE site_settings ADD COLUMN robots_mode TEXT NOT NULL DEFAULT 'managed'
    CHECK (robots_mode IN ('managed', 'custom'));

ALTER TABLE site_settings ADD COLUMN robots_custom TEXT NOT NULL DEFAULT '';

ALTER TABLE site_settings ADD COLUMN speculation_mode TEXT NOT NULL DEFAULT 'off'
    CHECK (speculation_mode IN ('off', 'prefetch', 'prerender'));

ALTER TABLE site_settings ADD COLUMN speculation_eagerness TEXT NOT NULL DEFAULT 'conservative'
    CHECK (speculation_eagerness IN ('conservative', 'moderate', 'eager'));

ALTER TABLE site_settings ADD COLUMN title_separator TEXT NOT NULL DEFAULT '–';

-- ===== 012_media.sql =====
-- Media domain: central asset store. The database keeps metadata and the
-- storage keys needed to reconstruct an asset; binary blobs live in controlled
-- storage (initially the local filesystem under data/media).

CREATE TABLE media (
    id TEXT PRIMARY KEY,

    original_filename TEXT NOT NULL,
    storage_key TEXT NOT NULL,

    mime_type TEXT NOT NULL,

    asset_type TEXT NOT NULL DEFAULT 'image'
        CHECK (asset_type IN ('image', 'video', 'audio', 'document', 'other')),

    file_size INTEGER NOT NULL
        CHECK (file_size >= 0),

    width INTEGER,
    height INTEGER,

    alt_text TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    caption TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',

    author_id TEXT,

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    UNIQUE (storage_key),

    FOREIGN KEY (author_id)
        REFERENCES users (id)
        ON DELETE SET NULL
);

CREATE INDEX idx_media_created_at ON media (created_at);
CREATE INDEX idx_media_asset_type ON media (asset_type);


-- Generated derivatives (responsive sizes, favicon sizes, future AVIF, ...).
-- The original upload is stored as kind 'original' so it can always be re-derived.
CREATE TABLE media_variants (
    id TEXT PRIMARY KEY,

    media_id TEXT NOT NULL,

    kind TEXT NOT NULL,
    storage_key TEXT NOT NULL,

    mime_type TEXT NOT NULL,

    width INTEGER,
    height INTEGER,

    file_size INTEGER NOT NULL
        CHECK (file_size >= 0),

    created_at INTEGER NOT NULL,

    FOREIGN KEY (media_id)
        REFERENCES media (id)
        ON DELETE CASCADE,

    UNIQUE (media_id, kind)
);

CREATE INDEX idx_media_variants_media ON media_variants (media_id);

-- ===== 013_site_icon.sql =====
-- Site Icon (favicon) references a single media asset. The CMS generates the
-- needed favicon variants from that asset; the user never manages the files.
-- SQLite cannot add a FOREIGN KEY to an existing column via ALTER TABLE, so
-- referential integrity is enforced in the application: saving validates that
-- the media asset exists, and deletion is guarded by CountMediaUsage.
ALTER TABLE site_settings ADD COLUMN site_icon_media_id TEXT;

CREATE INDEX idx_site_settings_site_icon ON site_settings (site_icon_media_id);

-- ===== 014_featured_media.sql =====
-- Featured Media / Featured Image on an Entry. The model references a media
-- asset by id; no UI is wired yet, but the column enables the feature later
-- without a schema rebuild. Content revisions already reference media by id.
-- Like site_icon_media_id, integrity is enforced in the application (SQLite
-- cannot add a FOREIGN KEY to an existing column via ALTER TABLE).
ALTER TABLE entries ADD COLUMN featured_media_id TEXT;

CREATE INDEX idx_entries_featured_media ON entries (featured_media_id);

-- ===== 015_core_image_block.sql =====
-- Core Image block: references a media asset by id and only stores usage-specific
-- data (alt override, caption). The renderer resolves mediaId to the
-- optimized variants, so URLs never live in the document.
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-image-v1', 'core', 'image', 1, 'Image', 'An image selected from the Media Library.',
    '{"schemaVersion":1,"props":{"type":"object","required":["mediaId"],"properties":{"mediaId":{"type":"string","default":""},"alt":{"type":"string","default":""},"caption":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["none","left","center"],"default":"none"},"decorative":{"type":"boolean","default":false},"sizes":{"type":"string","default":""},"eager":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"image","fields":{"props.mediaId":{"label":"Image","control":"media","group":"Content"},"props.alt":{"label":"Alt text","control":"text","group":"Accessibility"},"settings.decorative":{"label":"Decorative (no alt)","control":"checkbox","group":"Accessibility"},"props.caption":{"label":"Caption","control":"text","group":"Content"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.sizes":{"label":"Sizes (responsive)","control":"text","group":"Advanced"},"settings.eager":{"label":"Load eagerly (LCP)","control":"checkbox","group":"Advanced"}}}}',
    'template',
     '{{ $m := media .Props.mediaId }}{{ if $m.Src }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}"{{ if .Settings.sizes }} sizes="{{ .Settings.sizes }}"{{ end }}{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}" decoding="async" fetchpriority="{{ if .Settings.eager }}high{{ end }}">{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing">Image unavailable</div>{{ end }}',
    '.stratum-image{margin:0}.stratum-image img{display:block;max-width:100%;height:auto}.stratum-image-fit-cover img{object-fit:cover;width:100%;height:100%}.stratum-image-fit-contain img{object-fit:contain}.stratum-image-ar-1-1{aspect-ratio:1/1}.stratum-image-ar-4-3{aspect-ratio:4/3}.stratum-image-ar-16-9{aspect-ratio:16/9}.stratum-image-ar-3-2{aspect-ratio:3/2}.stratum-image-ar-1-1 img,.stratum-image-ar-4-3 img,.stratum-image-ar-16-9 img,.stratum-image-ar-3-2 img{width:100%;height:100%}.stratum-image-align-left{text-align:left}.stratum-image-align-center{text-align:center}.stratum-image-align-right{text-align:right}.stratum-image-align-full img{width:100%}.stratum-image-caption{margin-top:var(--st-space-xs);font-size:var(--st-small-size,.875rem);color:var(--st-color-text-muted)}.stratum-image-missing{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)}',
    'core', 1, unixepoch(), unixepoch()
);
-- ===== 015_stage1_blocks.sql =====
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

-- ===== 016_stage2_content.sql =====
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

-- ===== 017_dynamic_blocks.sql =====
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

-- ===== 018_fix_core_image_template.sql =====
-- Fix the core/image template: it referenced props/settings keys
-- (mediaID, objectFit, aspectRatio, loading) that do not exist on the block's
-- actual schema (mediaId, align, decorative, sizes, eager), so .Props.mediaID was
-- always empty and images never resolved. It also emitted a static attribute from
-- inside an {{ if }} action, which breaks html/template. Align the template with the
-- real schema and make fetchpriority a static attribute with a conditional value.
UPDATE block_definitions
SET template = '{{ $m := media .Props.mediaId }}{{ if $m.Src }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}"{{ if .Settings.sizes }} sizes="{{ .Settings.sizes }}"{{ end }}{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}" decoding="async" fetchpriority="{{ if .Settings.eager }}high{{ end }}">{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing">Image unavailable</div>{{ end }}',
    updated_at = unixepoch()
WHERE id = 'core-image-v1';

-- ===== 018_stage2_media.sql =====
-- Stage 2 media + structured blocks: Table, Gallery, Video.

-- ============================================================
-- Table: responsive structured content
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-table-v1', 'core', 'table', 1, 'Table', 'A responsive table with optional header and striped rows.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"header":{"type":"string","default":""},"body":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"caption":{"type":"string","default":""},"striped":{"type":"boolean","default":false},"variant":{"type":"string","enum":["bordered","plain"],"default":"bordered"}}},"children":{"mode":"none"},"editor":{"category":"content","icon":"table","fields":{"props.header":{"label":"Header row (cells separated by |)","control":"text","group":"Content"},"props.body":{"label":"Rows (one per line, cells separated by |)","control":"textarea","group":"Content"},"settings.caption":{"label":"Caption","control":"text","group":"Content"},"settings.striped":{"label":"Striped","control":"checkbox","group":"Style"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}',
    'template',
    '<figure class="stratum-table-fig stratum-table-variant-{{ .Settings.variant }}">{{ if .Props.caption }}<figcaption class="stratum-table-caption">{{ .Props.caption }}</figcaption>{{ end }}<div class="stratum-table-scroll"><table class="stratum-table">{{ if .Props.header }}<thead><tr>{{ range split "|" .Props.header }}<th scope="col">{{ . }}</th>{{ end }}</tr></thead>{{ end }}<tbody>{{ range lines .Props.body }}<tr>{{ range split "|" . }}<td>{{ . }}</td>{{ end }}</tr>{{ end }}</tbody></table></div></figure>',
    '.stratum-table-fig{margin:0}.stratum-table{width:100%;border-collapse:collapse}.stratum-table-variant-bordered{border:1px solid var(--st-color-border)}.stratum-table-variant-bordered th,.stratum-table-variant-bordered td{border:1px solid var(--st-color-border)}.stratum-table th,.stratum-table td{padding:var(--st-space-sm) var(--st-space-md);text-align:left}.stratum-table thead th{background:var(--st-color-surface-muted);font-weight:600}.stratum-table-striped tbody tr:nth-child(even){background:var(--st-color-surface-muted)}.stratum-table-caption{margin-bottom:var(--st-space-xs);font-size:var(--st-small-size,.875rem);color:var(--st-color-text-muted)}.stratum-table-scroll{overflow-x:auto}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Gallery: responsive image grid
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-gallery-v1', 'core', 'gallery', 1, 'Gallery', 'A responsive grid of images from the Media Library.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"images":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"columns":{"type":"integer","enum":[2,3,4],"default":3},"gap":{"type":"string","enum":["none","xs","sm","md","lg"],"default":"sm"},"aspectRatio":{"type":"string","enum":["auto","1:1","4:3","16:9","3:2"],"default":"auto"},"captions":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"gallery","fields":{"props.images":{"label":"Image IDs (comma separated)","control":"text","group":"Content"},"settings.columns":{"label":"Columns","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.aspectRatio":{"label":"Aspect ratio","control":"select","group":"Style"},"settings.captions":{"label":"Show captions","control":"checkbox","group":"Content"}}}}',
    'template',
    '{{ $ids := split "," .Props.images }}{{ if $ids }}<div class="stratum-gallery stratum-gallery-cols-{{ .Settings.columns }} stratum-gallery-gap-{{ .Settings.gap }}{{ if ne .Settings.aspectRatio "auto" }} stratum-gallery-ar-{{ .Settings.aspectRatio }}{{ end }}">{{ range $ids }}{{ $m := media . }}{{ if $m.Src }}<figure class="stratum-gallery-item">{{ if $m.SrcSet }}<img src="{{ $m.Src }}" srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 33vw,(min-width:640px) 50vw,100vw" alt="{{ $m.Alt }}" loading="lazy" decoding="async">{{ else }}<img src="{{ $m.Src }}" alt="{{ $m.Alt }}" loading="lazy" decoding="async">{{ end }}</figure>{{ end }}{{ end }}</div>{{ else }}<div class="stratum-gallery stratum-gallery-empty">No images selected</div>{{ end }}',
    '.stratum-gallery{display:grid;grid-template-columns:repeat(2,1fr);gap:var(--st-space-sm)}.stratum-gallery-cols-2{grid-template-columns:repeat(2,1fr)}.stratum-gallery-cols-3{grid-template-columns:repeat(3,1fr)}.stratum-gallery-cols-4{grid-template-columns:repeat(4,1fr)}.stratum-gallery-gap-none{gap:0}.stratum-gallery-gap-xs{gap:var(--st-space-xs)}.stratum-gallery-gap-sm{gap:var(--st-space-sm)}.stratum-gallery-gap-md{gap:var(--st-space-md)}.stratum-gallery-gap-lg{gap:var(--st-space-lg)}.stratum-gallery-ar-1-1 .stratum-gallery-item{aspect-ratio:1/1}.stratum-gallery-ar-4-3 .stratum-gallery-item{aspect-ratio:4/3}.stratum-gallery-ar-16-9 .stratum-gallery-item{aspect-ratio:16/9}.stratum-gallery-ar-3-2 .stratum-gallery-item{aspect-ratio:3/2}.stratum-gallery-item{margin:0;overflow:hidden}.stratum-gallery-item img{display:block;width:100%;height:100%;object-fit:cover}.stratum-gallery-empty{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)}@media(max-width:640px){.stratum-gallery-cols-3,.stratum-gallery-cols-4{grid-template-columns:repeat(2,1fr)}}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Video: YouTube, Vimeo or self-hosted file
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-video-v1', 'core', 'video', 1, 'Video', 'Embed a YouTube, Vimeo or self-hosted video.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{"url":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"provider":{"type":"string","enum":["youtube","vimeo","file"],"default":"youtube"},"aspectRatio":{"type":"string","enum":["16:9","4:3","1:1"],"default":"16:9"},"autoplay":{"type":"boolean","default":false},"mute":{"type":"boolean","default":false},"loop":{"type":"boolean","default":false},"controls":{"type":"boolean","default":true},"poster":{"type":"string","default":""}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"video","fields":{"props.url":{"label":"Video URL","control":"text","group":"Content"},"settings.provider":{"label":"Provider","control":"segmented","group":"Source"},"settings.aspectRatio":{"label":"Aspect ratio","control":"select","group":"Style"},"settings.autoplay":{"label":"Autoplay","control":"checkbox","group":"Playback"},"settings.mute":{"label":"Muted","control":"checkbox","group":"Playback"},"settings.loop":{"label":"Loop","control":"checkbox","group":"Playback"},"settings.controls":{"label":"Show controls","control":"checkbox","group":"Playback"},"settings.poster":{"label":"Poster image ID (file only)","control":"text","group":"Source"}}}}',
    'template',
    '{{ $id := "" }}{{ if eq .Settings.provider "youtube" }}{{ $id = youtubeID .Props.url }}{{ else if eq .Settings.provider "vimeo" }}{{ $id = vimeoID .Props.url }}{{ end }}<div class="stratum-video stratum-video-ar-{{ .Settings.aspectRatio }}">{{ if and (eq .Settings.provider "youtube") $id }}<iframe class="stratum-video-frame" src="https://www.youtube.com/embed/{{ $id }}?rel=0{{ if .Settings.autoplay }}&autoplay=1{{ end }}{{ if .Settings.mute }}&mute=1{{ end }}{{ if .Settings.loop }}&loop=1{{ end }}" title="YouTube video" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen loading="lazy"></iframe>{{ else if and (eq .Settings.provider "vimeo") $id }}<iframe class="stratum-video-frame" src="https://player.vimeo.com/video/{{ $id }}{{ if .Settings.autoplay }}?autoplay=1{{ end }}{{ if .Settings.mute }}&muted=1{{ end }}{{ if .Settings.loop }}&loop=1{{ end }}" title="Vimeo video" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen loading="lazy"></iframe>{{ else if eq .Settings.provider "file" }}{{ $p := media .Settings.poster }}<video class="stratum-video-frame"{{ if .Settings.controls }} controls{{ end }}{{ if .Settings.autoplay }} autoplay{{ end }}{{ if .Settings.mute }} muted{{ end }}{{ if .Settings.loop }} loop{{ end }}{{ if $p.Src }} poster="{{ $p.Src }}"{{ end }} src="{{ .Props.url }}"></video>{{ else }}<div class="stratum-video-missing">Video unavailable</div>{{ end }}</div>',
    '.stratum-video{position:relative;width:100%;margin:0}.stratum-video-ar-16-9{aspect-ratio:16/9}.stratum-video-ar-4-3{aspect-ratio:4/3}.stratum-video-ar-1-1{aspect-ratio:1/1}.stratum-video-frame{position:absolute;inset:0;width:100%;height:100%;border:0;display:block}.stratum-video-missing{aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;border:1px dashed var(--st-color-border);color:var(--st-color-text-muted)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ===== 019_stage2_dynamic.sql =====
-- Stage 2 dynamic blocks: Featured Image, Site Logo, Social Links, Breadcrumbs.
-- Also extends site_settings with the branding columns these blocks read.

ALTER TABLE site_settings ADD COLUMN site_logo_media_id TEXT;
ALTER TABLE site_settings ADD COLUMN social_links TEXT;

-- ============================================================
-- Featured Image: the current entry's featured media
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-featured-image-v1', 'core', 'featured-image', 1, 'Featured Image', 'The featured image of the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"aspectRatio":{"type":"string","enum":["auto","1:1","4:3","16:9","3:2"],"default":"16:9"},"objectFit":{"type":"string","enum":["cover","contain"],"default":"cover"},"align":{"type":"string","enum":["left","center","right","full"],"default":"full"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"image","fields":{"settings.aspectRatio":{"label":"Aspect ratio","control":"select","group":"Style"},"settings.objectFit":{"label":"Object fit","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ $m := media .Context.Entry.FeaturedImage }}{{ if $m.Src }}<figure class="stratum-featured-image stratum-featured-image-fit-{{ .Settings.objectFit }} stratum-featured-image-ar-{{ .Settings.aspectRatio }} stratum-featured-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}" sizes="(min-width:992px) 768px,(min-width:640px) 480px,100vw"{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ $m.Alt }}" loading="lazy" decoding="async"></figure>{{ else }}<div class="stratum-featured-image stratum-featured-image-missing">Featured image</div>{{ end }}',
    '.stratum-featured-image{margin:0}.stratum-featured-image img{display:block;max-width:100%;height:auto}.stratum-featured-image-fit-cover img{object-fit:cover;width:100%;height:100%}.stratum-featured-image-fit-contain img{object-fit:contain}.stratum-featured-image-ar-1-1{aspect-ratio:1/1}.stratum-featured-image-ar-4-3{aspect-ratio:4/3}.stratum-featured-image-ar-16-9{aspect-ratio:16/9}.stratum-featured-image-ar-3-2{aspect-ratio:3/2}.stratum-featured-image-ar-1-1 img,.stratum-featured-image-ar-4-3 img,.stratum-featured-image-ar-16-9 img,.stratum-featured-image-ar-3-2 img{width:100%;height:100%}.stratum-featured-image-align-center{text-align:center}.stratum-featured-image-align-center img{margin-inline:auto}.stratum-featured-image-missing{padding:var(--st-space-lg);border:1px dashed var(--st-color-border);text-align:center;color:var(--st-color-text-muted)}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Site Logo: the site logo from Site Settings
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-logo-v1', 'core', 'logo', 1, 'Site Logo', 'The site logo from Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"link":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"branding","icon":"site","fields":{"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.link":{"label":"Link to home","control":"checkbox","group":"Style"}}}}',
    'template',
    '{{ if .Context.Site.LogoURL }}{{ if .Settings.link }}<a class="stratum-logo" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}<img class="stratum-logo-img stratum-logo-size-{{ .Settings.size }}" src="{{ .Context.Site.LogoURL }}" alt="{{ .Context.Site.Name }}">{{ if .Settings.link }}</a>{{ end }}{{ else if .Context.Site.Name }}{{ if .Settings.link }}<a class="stratum-logo stratum-logo-text" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}{{ .Context.Site.Name }}{{ if .Settings.link }}</a>{{ end }}{{ else }}<span class="stratum-placeholder">Site logo</span>{{ end }}',
    '.stratum-logo{display:inline-flex;align-items:center;text-decoration:none;color:inherit;font-weight:700}.stratum-logo-img{display:block;max-width:100%}.stratum-logo-size-sm .stratum-logo-img,.stratum-logo-img.stratum-logo-size-sm{height:1.75rem}.stratum-logo-size-md .stratum-logo-img,.stratum-logo-img.stratum-logo-size-md{height:2.25rem}.stratum-logo-size-lg .stratum-logo-img,.stratum-logo-img.stratum-logo-size-lg{height:3rem}.stratum-logo-text{font-size:1.25rem}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Social Links: configured site social profiles
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-social-links-v1', 'core', 'social-links', 1, 'Social Links', 'Social profile links configured in Site Settings.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["start","center","end"],"default":"start"}}},"children":{"mode":"none"},"editor":{"category":"branding","icon":"social","fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}',
    'template',
    '{{ if .Context.Site.SocialLinks }}<ul class="stratum-social stratum-social-align-{{ .Settings.align }}">{{ range .Context.Site.SocialLinks }}<li class="stratum-social-item"><a class="stratum-social-link" href="{{ .URL }}" target="_blank" rel="noopener noreferrer">{{ if .Label }}{{ .Label }}{{ else }}{{ .Platform }}{{ end }}</a></li>{{ end }}</ul>{{ else }}<span class="stratum-placeholder">Social links</span>{{ end }}',
    '.stratum-social{display:flex;flex-wrap:wrap;gap:var(--st-space-md);list-style:none;margin:0;padding:0}.stratum-social-align-center{justify-content:center}.stratum-social-align-end{justify-content:flex-end}.stratum-social-link{display:inline-flex;align-items:center;gap:.35rem;color:var(--st-color-primary);text-decoration:none}.stratum-social-link:hover{text-decoration:underline}',
    'core', 1, unixepoch(), unixepoch()
);

-- ============================================================
-- Breadcrumbs: system-generated from the current entry
-- ============================================================
INSERT INTO block_definitions (
    id, namespace, name, version, display_name, description, schema_json,
    renderer_type, template, styles, source, enabled, created_at, updated_at
)
VALUES (
    'core-breadcrumbs-v1', 'core', 'breadcrumbs', 1, 'Breadcrumbs', 'A breadcrumb trail for the current entry.',
    '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"showHome":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"breadcrumbs","fields":{"settings.showHome":{"label":"Show home","control":"checkbox","group":"Content"}}}}',
    'template',
    '{{ $home := "/" }}{{ if .Context.Site.URL }}{{ $home = .Context.Site.URL }}{{ end }}<nav class="stratum-breadcrumbs" aria-label="Breadcrumb"><ol class="stratum-breadcrumbs-list">{{ if .Settings.showHome }}<li class="stratum-breadcrumbs-item"><a href="{{ $home }}">Home</a></li>{{ end }}{{ if .Context.Entry.Title }}<li class="stratum-breadcrumbs-item stratum-breadcrumbs-current" aria-current="page">{{ .Context.Entry.Title }}</li>{{ end }}</ol></nav>',
    '.stratum-breadcrumbs{margin:0}.stratum-breadcrumbs-list{display:flex;flex-wrap:wrap;align-items:center;gap:.5rem;list-style:none;margin:0;padding:0;font-size:var(--st-small-size,.875rem);color:var(--st-color-text-muted)}.stratum-breadcrumbs-item:not(:first-child)::before{content:"/";margin-right:.5rem;color:var(--st-color-text-muted)}.stratum-breadcrumbs-item a{color:inherit;text-decoration:none}.stratum-breadcrumbs-item a:hover{text-decoration:underline}.stratum-breadcrumbs-current{color:var(--st-color-text)}',
    'core', 1, unixepoch(), unixepoch()
);
