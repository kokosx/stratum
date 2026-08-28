-- name: CreateForm :exec
INSERT INTO forms (id, name, schema_version, definition_json, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateForm :exec
UPDATE forms SET name = ?, schema_version = ?, definition_json = ?, active = ?, updated_at = ? WHERE id = ?;

-- name: GetForm :one
SELECT id, name, schema_version, definition_json, active, created_at, updated_at FROM forms WHERE id = ? LIMIT 1;

-- name: ListForms :many
SELECT f.id, f.name, f.schema_version, f.definition_json, f.active, f.created_at, f.updated_at,
       COUNT(s.id) AS submission_count,
       CAST(COALESCE(SUM(CASE WHEN s.status = 'new' THEN 1 ELSE 0 END), 0) AS INTEGER) AS new_count
FROM forms f LEFT JOIN form_submissions s ON s.form_id = f.id
GROUP BY f.id ORDER BY f.name, f.id;

-- name: ListActiveForms :many
SELECT id, name, schema_version, definition_json, active, created_at, updated_at FROM forms WHERE active = 1 ORDER BY name, id;

-- name: DeleteForm :exec
DELETE FROM forms WHERE id = ?;

-- name: CountFormSubmissions :one
SELECT COUNT(*) FROM form_submissions WHERE form_id = ?;

-- name: CreateFormSubmission :exec
INSERT INTO form_submissions (id, form_id, status, values_json, schema_snapshot_json, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetFormSubmission :one
SELECT id, form_id, status, values_json, schema_snapshot_json, created_at FROM form_submissions WHERE id = ? LIMIT 1;

-- name: ListFormSubmissions :many
SELECT id, form_id, status, values_json, schema_snapshot_json, created_at
FROM form_submissions WHERE form_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?;

-- name: ListAllFormSubmissions :many
SELECT id, form_id, status, values_json, schema_snapshot_json, created_at
FROM form_submissions WHERE form_id = ? ORDER BY created_at ASC, id ASC;

-- name: UpdateFormSubmissionStatus :exec
UPDATE form_submissions SET status = ? WHERE id = ?;

-- name: DeleteFormSubmission :exec
DELETE FROM form_submissions WHERE id = ?;

-- name: ListDocumentsForFormReferenceScan :many
SELECT document_json FROM entry_revisions
WHERE id IN (SELECT id FROM entry_revisions WHERE (entry_id, revision_number) IN (SELECT entry_id, MAX(revision_number) FROM entry_revisions GROUP BY entry_id))
   OR id IN (SELECT published_revision_id FROM entries WHERE published_revision_id IS NOT NULL)
UNION
SELECT document_json FROM layout_template_revisions
WHERE id IN (SELECT id FROM layout_template_revisions WHERE (layout_template_id, revision_number) IN (SELECT layout_template_id, MAX(revision_number) FROM layout_template_revisions GROUP BY layout_template_id))
   OR id IN (SELECT published_revision_id FROM layout_templates WHERE published_revision_id IS NOT NULL)
UNION
SELECT document_json FROM site_part_revisions
WHERE id IN (SELECT id FROM site_part_revisions WHERE (site_part_id, revision_number) IN (SELECT site_part_id, MAX(revision_number) FROM site_part_revisions GROUP BY site_part_id))
   OR id IN (SELECT published_revision_id FROM site_parts WHERE published_revision_id IS NOT NULL);
