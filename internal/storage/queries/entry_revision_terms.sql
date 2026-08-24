-- name: SetTermsForRevision :exec
INSERT INTO entry_revision_terms (revision_id, term_id) VALUES (?, ?) ON CONFLICT(revision_id, term_id) DO NOTHING;
-- name: DeleteTermsForRevision :exec
DELETE FROM entry_revision_terms WHERE revision_id = ?;
-- name: ListTermsForRevision :many
SELECT terms.* FROM terms INNER JOIN entry_revision_terms ON entry_revision_terms.term_id = terms.id WHERE entry_revision_terms.revision_id = ? ORDER BY terms.name;
-- name: ListTermIDsForRevision :many
SELECT term_id FROM entry_revision_terms WHERE revision_id = ?;
-- name: ListPublishedEntriesByTerm :many
SELECT entries.id, entries.slug, entries.content_type_id, latest_revision.title, latest_revision.excerpt, entries.featured_media_id, routes.path AS route_path, entries.published_at, entries.first_published_at, COALESCE(entries.published_revision_id, '') AS revision_id
FROM entries
INNER JOIN entry_revision_terms ON entry_revision_terms.revision_id = entries.published_revision_id
INNER JOIN entry_revisions AS latest_revision ON latest_revision.id = entries.published_revision_id
INNER JOIN routes ON routes.entry_id = entries.id AND routes.route_type = 'entry'
WHERE entry_revision_terms.term_id = ? AND entries.content_type_id = ? AND entries.status = 'active' AND entries.published_revision_id IS NOT NULL
ORDER BY entries.published_at DESC, entries.id DESC
LIMIT ? OFFSET ?;
-- name: ListPublishedEntriesByTermAsc :many
SELECT entries.id, entries.slug, entries.content_type_id, latest_revision.title, latest_revision.excerpt, entries.featured_media_id, routes.path AS route_path, entries.published_at, entries.first_published_at, COALESCE(entries.published_revision_id, '') AS revision_id
FROM entries
INNER JOIN entry_revision_terms ON entry_revision_terms.revision_id = entries.published_revision_id
INNER JOIN entry_revisions AS latest_revision ON latest_revision.id = entries.published_revision_id
INNER JOIN routes ON routes.entry_id = entries.id AND routes.route_type = 'entry'
WHERE entry_revision_terms.term_id = ? AND entries.content_type_id = ? AND entries.status = 'active' AND entries.published_revision_id IS NOT NULL
ORDER BY entries.published_at ASC, entries.id ASC
LIMIT ? OFFSET ?;
-- name: ListPublishedEntriesByTermCount :one
SELECT COUNT(*) FROM entries
INNER JOIN entry_revision_terms ON entry_revision_terms.revision_id = entries.published_revision_id
WHERE entry_revision_terms.term_id = ? AND entries.content_type_id = ? AND entries.status = 'active' AND entries.published_revision_id IS NOT NULL;
-- name: CountPublishedEntriesByTerm :one
SELECT COUNT(*) FROM entries
INNER JOIN entry_revision_terms ON entry_revision_terms.revision_id = entries.published_revision_id
WHERE entry_revision_terms.term_id = ?;
