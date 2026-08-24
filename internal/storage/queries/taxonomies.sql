-- name: CreateTaxonomy :exec
INSERT INTO taxonomies (id, content_type_id, singular_name, plural_name, hierarchical, public, route_base, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
-- name: GetTaxonomy :one
SELECT * FROM taxonomies WHERE id = ? LIMIT 1;
-- name: ListTaxonomies :many
SELECT * FROM taxonomies ORDER BY id;
-- name: ListTaxonomiesByContentType :many
SELECT * FROM taxonomies WHERE content_type_id = ? ORDER BY id;
-- name: UpdateTaxonomy :exec
UPDATE taxonomies SET singular_name = ?, plural_name = ?, hierarchical = ?, public = ?, route_base = ?, updated_at = ? WHERE id = ?;
-- name: DeleteTaxonomy :exec
DELETE FROM taxonomies WHERE id = ?;
