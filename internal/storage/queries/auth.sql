-- name: HasAdmin :one
SELECT EXISTS(
    SELECT 1
    FROM users
    WHERE role = 'admin'
);

-- name: CreateUser :exec
INSERT INTO users (id, email, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, created_at, updated_at
FROM users
WHERE email = ?
LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSessionUser :one
SELECT u.id, u.email, u.role, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ?
LIMIT 1;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = ?;

-- name: UpdateSiteTitle :exec
UPDATE site_settings
SET site_title = ?, updated_at = ?
WHERE id = 1;
