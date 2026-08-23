-- name: CreateNavigationMenu :exec
INSERT INTO navigation_menus (id, name, slug, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetNavigationMenu :one
SELECT * FROM navigation_menus WHERE id = ? LIMIT 1;

-- name: GetNavigationMenuBySlug :one
SELECT * FROM navigation_menus WHERE slug = ? LIMIT 1;

-- name: ListNavigationMenus :many
SELECT * FROM navigation_menus ORDER BY name, id;

-- name: UpdateNavigationMenu :exec
UPDATE navigation_menus SET name = ?, slug = ?, updated_at = ? WHERE id = ?;

-- name: DeleteNavigationMenu :exec
DELETE FROM navigation_menus WHERE id = ?;

-- name: DeleteNavigationItemsByMenu :exec
DELETE FROM navigation_items WHERE menu_id = ?;

-- name: CreateNavigationItem :exec
INSERT INTO navigation_items (
    id, menu_id, parent_id, position, label, target_type,
    entry_id, url, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListNavigationItemsByMenu :many
SELECT
    ni.*,
    e.status AS entry_status,
    e.content_type_id AS entry_content_type,
    e.published_revision_id AS entry_published_revision_id,
    latest_revision.title AS entry_title,
    rt.path AS entry_path
FROM navigation_items ni
LEFT JOIN entries e ON e.id = ni.entry_id
LEFT JOIN entry_revisions latest_revision
    ON latest_revision.entry_id = e.id
    AND latest_revision.revision_number = (
        SELECT MAX(revision_number) FROM entry_revisions WHERE entry_id = e.id
    )
LEFT JOIN routes rt ON rt.id = (
    SELECT id FROM routes
    WHERE entry_id = ni.entry_id AND route_type = 'entry'
    ORDER BY path LIMIT 1
)
WHERE ni.menu_id = ?
ORDER BY COALESCE(ni.parent_id, ''), ni.position, ni.id;

-- name: ListNavigationLocations :many
SELECT location, menu_id FROM navigation_locations ORDER BY location;

-- name: ListNavigationLocationsForMenu :many
SELECT location FROM navigation_locations WHERE menu_id = ? ORDER BY location;

-- name: DeleteNavigationLocationsForMenu :exec
DELETE FROM navigation_locations WHERE menu_id = ?;

-- name: UpsertNavigationLocation :exec
INSERT INTO navigation_locations (location, menu_id) VALUES (?, ?)
ON CONFLICT(location) DO UPDATE SET menu_id = excluded.menu_id;

-- name: ListPublishedPagesForNavigation :many
SELECT e.id, r.title, rt.path
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
JOIN routes rt ON rt.id = (
    SELECT id FROM routes
    WHERE entry_id = e.id AND route_type = 'entry'
    ORDER BY path LIMIT 1
)
WHERE e.content_type_id = 'page' AND e.status = 'active'
ORDER BY r.title, e.id;

-- name: ListPublishedEntriesForNavigation :many
SELECT e.id, e.content_type_id, r.title, rt.path
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
JOIN routes rt ON rt.id = (
    SELECT id FROM routes
    WHERE entry_id = e.id AND route_type = 'entry'
    ORDER BY path LIMIT 1
)
JOIN content_types ct ON ct.id = e.content_type_id
WHERE e.status = 'active' AND ct.public = 1 AND rt.path IS NOT NULL
ORDER BY ct.id, r.title, e.id;
