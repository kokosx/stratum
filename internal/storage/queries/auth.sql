-- name: HasAdmin :one
SELECT EXISTS(
    SELECT 1
    FROM users
WHERE role = 'admin'
);

-- name: CreateUser :exec
INSERT INTO users (id, email, password_hash, role, status, created_at, updated_at)
VALUES (?, ?, ?, ?, 'active', ?, ?);

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, status, created_at, updated_at
FROM users
WHERE email = ?
LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSessionUser :one
SELECT u.id, u.email, u.role, u.status, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ?
LIMIT 1;

-- name: ListUsers :many
SELECT id, email, role, status, created_at, updated_at
FROM users
ORDER BY email;

-- name: GetUserByID :one
SELECT id, email, password_hash, role, status, created_at, updated_at
FROM users WHERE id = ? LIMIT 1;

-- name: CountActiveAdmins :one
SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active';

-- name: UpdateUserRole :exec
UPDATE users SET role = ?, updated_at = ? WHERE id = ?;

-- name: UpdateUserStatus :exec
UPDATE users SET status = ?, updated_at = ? WHERE id = ?;

-- name: DeleteSessionsForUser :exec
DELETE FROM sessions WHERE user_id = ?;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = ?;

-- name: UpdateSiteTitle :exec
UPDATE site_settings
SET site_title = ?, updated_at = ?
WHERE id = 1;
