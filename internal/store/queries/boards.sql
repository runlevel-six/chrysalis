-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: CreateBoard :one
INSERT INTO boards (id, owner_id, name, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: AddBoardMember :exec
INSERT INTO board_members (board_id, user_id, role) VALUES (?, ?, ?);

-- name: LastBoardPosition :one
-- Includes trashed and archived boards, so a restored board's old position
-- never collides with a newer one.
SELECT position FROM boards WHERE owner_id = ? ORDER BY position DESC LIMIT 1;

-- name: ListBoardsForUser :many
SELECT sqlc.embed(boards), board_members.role
FROM boards
JOIN board_members ON board_members.board_id = boards.id
WHERE board_members.user_id = ?
  AND boards.deleted_at IS NULL
  AND boards.archived_at IS NULL
ORDER BY boards.position, boards.id;

-- name: GetBoardForUser :one
SELECT sqlc.embed(boards), board_members.role
FROM boards
JOIN board_members ON board_members.board_id = boards.id
WHERE board_members.user_id = ?
  AND boards.id = ?
  AND boards.deleted_at IS NULL;
