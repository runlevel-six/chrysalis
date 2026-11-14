-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: GetBoardRole :one
-- The user's role on a live (not trashed) board. Every board-scoped write
-- checks this first.
SELECT board_members.role
FROM board_members
JOIN boards ON boards.id = board_members.board_id
WHERE board_members.user_id = ? AND board_members.board_id = ? AND boards.deleted_at IS NULL;

-- name: ListColumns :many
SELECT * FROM columns
WHERE board_id = ? AND deleted_at IS NULL
ORDER BY position, id;

-- name: ListCards :many
-- Cards in trashed columns are hidden with their column, and come back when
-- it's restored.
SELECT cards.* FROM cards
JOIN columns ON columns.id = cards.column_id
WHERE cards.board_id = ?
  AND cards.deleted_at IS NULL
  AND cards.is_template = 0
  AND cards.archived_at IS NULL
  AND columns.deleted_at IS NULL
ORDER BY cards.column_id, cards.position, cards.id;

-- name: GetColumn :one
SELECT * FROM columns WHERE id = ?;

-- name: GetCard :one
SELECT * FROM cards WHERE id = ?;

-- name: LastColumnPosition :one
SELECT position FROM columns WHERE board_id = ? ORDER BY position DESC LIMIT 1;

-- name: LastCardPosition :one
SELECT position FROM cards WHERE column_id = ? ORDER BY position DESC LIMIT 1;

-- name: CountLiveCardsInColumn :one
SELECT COUNT(*) FROM cards WHERE column_id = ? AND deleted_at IS NULL AND archived_at IS NULL;

-- name: CreateColumn :one
INSERT INTO columns (id, board_id, name, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateColumn :one
UPDATE columns SET
    name = ?, color = ?, color_mode = ?, soft_limit = ?, is_done_column = ?,
    position = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetColumnDeleted :one
-- deleted_at NULL restores from the trash.
UPDATE columns SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: CreateCard :one
INSERT INTO cards (id, board_id, column_id, title, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCard :one
UPDATE cards SET
    board_id = ?, column_id = ?, title = ?, position = ?, completed_at = ?,
    start_at = ?, due_at = ?, due_has_time = ?, archived_at = ?,
    cover_type = ?, cover_value = ?,
    updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: ListArchivedCards :many
SELECT cards.* FROM cards
JOIN columns ON columns.id = cards.column_id
WHERE cards.board_id = ? AND cards.archived_at IS NOT NULL
  AND cards.deleted_at IS NULL AND columns.deleted_at IS NULL
ORDER BY cards.archived_at DESC;

-- name: SetCardDeleted :one
UPDATE cards SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: ListTrashedColumns :many
SELECT * FROM columns WHERE board_id = ? AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC;

-- name: ListTrashedCards :many
-- Cards trashed on their own (their column is still live).
SELECT cards.* FROM cards
JOIN columns ON columns.id = cards.column_id
WHERE cards.board_id = ? AND cards.deleted_at IS NOT NULL AND columns.deleted_at IS NULL
ORDER BY cards.deleted_at DESC;

-- name: PurgeTrashedCards :execrows
DELETE FROM cards WHERE deleted_at IS NOT NULL AND deleted_at <= ?;

-- name: PurgeTrashedColumns :execrows
DELETE FROM columns WHERE deleted_at IS NOT NULL AND deleted_at <= ?;

-- name: UpsertCardView :exec
INSERT INTO user_card_view (user_id, card_id, expanded, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, card_id) DO UPDATE SET expanded = excluded.expanded, updated_at = excluded.updated_at;

-- name: UpsertColumnView :exec
INSERT INTO user_column_view (user_id, column_id, collapsed, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, column_id) DO UPDATE SET collapsed = excluded.collapsed, updated_at = excluded.updated_at;

-- name: ListCardViews :many
SELECT user_card_view.card_id, user_card_view.expanded
FROM user_card_view
JOIN cards ON cards.id = user_card_view.card_id
WHERE user_card_view.user_id = ? AND cards.board_id = ?;

-- name: ListColumnViews :many
SELECT user_column_view.column_id, user_column_view.collapsed
FROM user_column_view
JOIN columns ON columns.id = user_column_view.column_id
WHERE user_column_view.user_id = ? AND columns.board_id = ?;

-- name: ListTemplates :many
SELECT * FROM cards WHERE board_id = ? AND is_template = 1 AND deleted_at IS NULL ORDER BY title, id;

-- name: CopyCardRow :exec
INSERT INTO cards (id, board_id, column_id, title, position, start_at, due_at, due_has_time,
                   cover_type, cover_value, is_template, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListColumnCardPositions :many
SELECT id, position FROM cards
WHERE column_id = ? AND deleted_at IS NULL AND archived_at IS NULL AND is_template = 0
ORDER BY position, id;
