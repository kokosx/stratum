-- name: CreateAuditEvent :exec
INSERT INTO audit_events (id, actor_kind, actor_id, transport, action, resource_type, resource_id, revision_id, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditEventsByResource :many
SELECT * FROM audit_events WHERE resource_type = ? AND resource_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListAuditEventsByActor :many
SELECT * FROM audit_events WHERE actor_kind = ? AND actor_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: RecentAuditEvents :many
SELECT * FROM audit_events ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: GetAuditEvent :one
SELECT * FROM audit_events WHERE id = ? LIMIT 1;

-- name: CountAuditEvents :one
SELECT COUNT(*) FROM audit_events;

-- name: ListAuditEventsForRevision :many
SELECT * FROM audit_events WHERE revision_id = ? ORDER BY created_at DESC;
