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
