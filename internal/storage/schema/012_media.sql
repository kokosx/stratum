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
