-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: UpsertUser :one
INSERT INTO users (id, oidc_sub, email, display_name, created_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (oidc_sub) DO UPDATE SET
    email        = excluded.email,
    display_name = excluded.display_name,
    last_seen_at = excluded.last_seen_at
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, created_at, expires_at, user_agent)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionUser :one
SELECT sqlc.embed(sessions), sqlc.embed(users)
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.id = ? AND sessions.expires_at > ?;

-- name: ExtendSession :exec
UPDATE sessions SET expires_at = ? WHERE id = ?;

-- name: TouchUser :exec
UPDATE users SET last_seen_at = ? WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= ?;
