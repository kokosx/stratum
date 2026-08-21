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

    active_theme TEXT NOT NULL DEFAULT 'default',

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