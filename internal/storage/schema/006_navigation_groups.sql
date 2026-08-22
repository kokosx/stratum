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
