-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: ListLabels :many
SELECT * FROM labels WHERE board_id = ? AND deleted_at IS NULL ORDER BY position, id;

-- name: GetLabel :one
SELECT * FROM labels WHERE id = ?;

-- name: LastLabelPosition :one
SELECT position FROM labels WHERE board_id = ? ORDER BY position DESC LIMIT 1;

-- name: CreateLabel :one
INSERT INTO labels (id, board_id, name, color, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateLabel :one
UPDATE labels SET name = ?, color = ?, position = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetLabelDeleted :one
UPDATE labels SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: ListCardLabelsForBoard :many
-- Card/label pairs for live labels on a board.
SELECT card_labels.card_id, card_labels.label_id
FROM card_labels
JOIN labels ON labels.id = card_labels.label_id
WHERE labels.board_id = ? AND labels.deleted_at IS NULL;

-- name: ListLabelIDsForCard :many
SELECT card_labels.label_id
FROM card_labels
JOIN labels ON labels.id = card_labels.label_id
WHERE card_labels.card_id = ? AND labels.deleted_at IS NULL
ORDER BY labels.position, labels.id;

-- name: ClearCardLabels :exec
DELETE FROM card_labels WHERE card_id = ?;

-- name: AddCardLabel :exec
INSERT INTO card_labels (card_id, label_id) VALUES (?, ?) ON CONFLICT DO NOTHING;

-- name: PurgeTrashedLabels :execrows
DELETE FROM labels WHERE deleted_at IS NOT NULL AND deleted_at <= ?;
