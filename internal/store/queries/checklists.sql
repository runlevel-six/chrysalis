-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: ListChecklists :many
SELECT * FROM checklists WHERE card_id = ? AND deleted_at IS NULL ORDER BY position, id;

-- name: ListChecklistItemsForCard :many
SELECT checklist_items.* FROM checklist_items
JOIN checklists ON checklists.id = checklist_items.checklist_id
WHERE checklists.card_id = ? AND checklists.deleted_at IS NULL AND checklist_items.deleted_at IS NULL
ORDER BY checklist_items.position, checklist_items.id;

-- name: ChecklistCountsForBoard :many
-- Done/total item counts per card, for "2/5" on collapsed cards.
SELECT checklists.card_id,
       COUNT(*) AS total,
       CAST(SUM(checklist_items.done_at IS NOT NULL) AS INTEGER) AS done
FROM checklist_items
JOIN checklists ON checklists.id = checklist_items.checklist_id
JOIN cards ON cards.id = checklists.card_id
WHERE cards.board_id = ? AND checklists.deleted_at IS NULL AND checklist_items.deleted_at IS NULL
GROUP BY checklists.card_id;

-- name: GetChecklist :one
SELECT * FROM checklists WHERE id = ?;

-- name: GetChecklistItem :one
SELECT * FROM checklist_items WHERE id = ?;

-- name: LastChecklistPosition :one
SELECT position FROM checklists WHERE card_id = ? ORDER BY position DESC LIMIT 1;

-- name: LastChecklistItemPosition :one
SELECT position FROM checklist_items WHERE checklist_id = ? ORDER BY position DESC LIMIT 1;

-- name: CreateChecklist :one
INSERT INTO checklists (id, card_id, title, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateChecklist :one
UPDATE checklists SET title = ?, position = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetChecklistDeleted :one
UPDATE checklists SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: CreateChecklistItem :one
INSERT INTO checklist_items (id, checklist_id, text, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateChecklistItem :one
UPDATE checklist_items SET checklist_id = ?, text = ?, done_at = ?, position = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetChecklistItemDeleted :one
UPDATE checklist_items SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: PurgeTrashedChecklists :execrows
DELETE FROM checklists WHERE deleted_at IS NOT NULL AND deleted_at <= ?;

-- name: PurgeTrashedChecklistItems :execrows
DELETE FROM checklist_items WHERE deleted_at IS NOT NULL AND deleted_at <= ?;
