-- name: ListTermsByTaxonomy :many
SELECT * FROM terms WHERE taxonomy_id = ? ORDER BY name COLLATE NOCASE;

-- name: ListTermsByTaxonomyWithCounts :many
SELECT
    t.id, t.taxonomy_id, t.parent_id, t.name, t.slug, t.description, t.created_at, t.updated_at,
    COUNT(DISTINCT e.id) AS published_count
FROM terms t
LEFT JOIN entry_revision_terms ert ON ert.term_id = t.id
LEFT JOIN entries e ON e.published_revision_id = ert.revision_id AND e.status = 'active'
WHERE t.taxonomy_id = ?
GROUP BY t.id
ORDER BY t.name COLLATE NOCASE;

-- name: GetTerm :one
SELECT * FROM terms WHERE id = ? LIMIT 1;

-- name: GetTermBySlug :one
SELECT * FROM terms WHERE taxonomy_id = ? AND slug = ? LIMIT 1;

-- name: GetTermByTaxonomyAndSlug :one
SELECT * FROM terms WHERE taxonomy_id = ? AND slug = ? LIMIT 1;

-- name: CreateTerm :exec
INSERT INTO terms (id, taxonomy_id, parent_id, name, slug, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateTerm :exec
UPDATE terms SET parent_id = ?, name = ?, slug = ?, description = ?, updated_at = ? WHERE id = ?;

-- name: DeleteTerm :exec
DELETE FROM terms WHERE id = ?;

-- name: ListChildTerms :many
SELECT * FROM terms WHERE parent_id = ? ORDER BY name COLLATE NOCASE;

-- name: SearchTermsByTaxonomy :many
SELECT * FROM terms WHERE taxonomy_id = ? AND (name LIKE '%' || ? || '%' OR slug LIKE '%' || ? || '%') ORDER BY name COLLATE NOCASE LIMIT ? OFFSET ?;

-- name: CountTermsByTaxonomy :one
SELECT COUNT(*) FROM terms WHERE taxonomy_id = ?;
