-- name: CreateAgent :exec
INSERT INTO agents (id, name, status, default_author_id, created_by_user_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetAgent :one
SELECT * FROM agents WHERE id = ? LIMIT 1;

-- name: ListAgents :many
SELECT * FROM agents ORDER BY created_at DESC, name ASC;

-- name: UpdateAgent :exec
UPDATE agents SET name = ?, default_author_id = ?, updated_at = ? WHERE id = ?;

-- name: UpdateAgentStatus :exec
UPDATE agents SET status = ?, updated_at = ? WHERE id = ?;

-- name: CreateAgentToken :exec
INSERT INTO agent_tokens (id, agent_id, token_hash, token_prefix, label, created_at, expires_at, last_used_at, revoked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: LookupAgentByTokenHash :one
SELECT
    a.id as agent_id, a.name as agent_name, a.status as agent_status, a.default_author_id,
    t.id as token_id, t.token_hash, t.token_prefix, t.label, t.expires_at, t.revoked_at, t.last_used_at
FROM agent_tokens t
JOIN agents a ON a.id = t.agent_id
WHERE t.token_hash = ?
LIMIT 1;

-- name: ListAgentTokens :many
SELECT * FROM agent_tokens WHERE agent_id = ? ORDER BY created_at DESC;

-- name: GetAgentToken :one
SELECT * FROM agent_tokens WHERE id = ? LIMIT 1;

-- name: RevokeAgentToken :exec
UPDATE agent_tokens SET revoked_at = ? WHERE id = ?;

-- name: UpdateAgentTokenLastUsed :exec
UPDATE agent_tokens SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR ? - last_used_at > 300);

-- name: DeleteAgentTokensByAgent :exec
DELETE FROM agent_tokens WHERE agent_id = ?;

-- name: ListAgentGrants :many
SELECT agent_id, permission, scope FROM agent_grants WHERE agent_id = ? ORDER BY permission, scope;

-- name: AddAgentGrant :exec
INSERT OR IGNORE INTO agent_grants (agent_id, permission, scope) VALUES (?, ?, ?);

-- name: RemoveAgentGrant :exec
DELETE FROM agent_grants WHERE agent_id = ? AND permission = ? AND scope = ?;

-- name: DeleteAgentGrants :exec
DELETE FROM agent_grants WHERE agent_id = ?;

-- name: CountActiveAgents :one
SELECT COUNT(*) FROM agents WHERE status = 'active';

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = ?;
