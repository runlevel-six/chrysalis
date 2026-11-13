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

-- name: UpdateBoard :one
UPDATE boards SET name = ?, icon = ?, position = ?, settings_json = ?, background_type = ?, background_value = ?,
    archived_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: ListArchivedBoardsForUser :many
SELECT sqlc.embed(boards), board_members.role
FROM boards
JOIN board_members ON board_members.board_id = boards.id
WHERE board_members.user_id = ?
  AND boards.deleted_at IS NULL
  AND boards.archived_at IS NOT NULL
ORDER BY boards.archived_at DESC, boards.id;

-- name: ListTrashedBoardsForOwner :many
-- Only owners delete boards, so only they see them in the trash.
SELECT sqlc.embed(boards), board_members.role
FROM boards
JOIN board_members ON board_members.board_id = boards.id
WHERE board_members.user_id = ? AND board_members.role = 'owner'
  AND boards.deleted_at IS NOT NULL
ORDER BY boards.deleted_at DESC, boards.id;

-- name: GetBoardForUserIncludingTrashed :one
SELECT sqlc.embed(boards), board_members.role
FROM boards
JOIN board_members ON board_members.board_id = boards.id
WHERE board_members.user_id = ? AND boards.id = ?;

-- name: SetBoardDeleted :one
UPDATE boards SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: PurgeTrashedBoards :execrows
-- Columns, cards, labels, attachments and view state go with them (ON DELETE
-- CASCADE); attachment files are removed first by PurgeFiles.
DELETE FROM boards WHERE deleted_at IS NOT NULL AND deleted_at <= ?;
