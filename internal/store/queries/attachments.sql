-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: CreateAttachment :one
INSERT INTO attachments (id, board_id, card_id, uploader_id, kind, filename, mime, size_bytes,
                         storage_key, thumb_key, url, width, height, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = ?;

-- name: ListCardAttachments :many
SELECT * FROM attachments WHERE card_id = ? AND deleted_at IS NULL ORDER BY created_at, id;

-- name: CountCardAttachmentsForBoard :many
SELECT card_id, COUNT(*) AS n FROM attachments
WHERE board_id = ? AND card_id IS NOT NULL AND deleted_at IS NULL
GROUP BY card_id;

-- name: SetAttachmentDeleted :one
UPDATE attachments SET deleted_at = ? WHERE id = ? RETURNING *;

-- name: ListPurgeableAttachments :many
-- Attachments whose files can go: trashed themselves, or on a card, column
-- or board that's being purged, all past the cutoff.
SELECT attachments.* FROM attachments
LEFT JOIN cards ON cards.id = attachments.card_id
LEFT JOIN columns ON columns.id = cards.column_id
JOIN boards ON boards.id = attachments.board_id
WHERE (attachments.deleted_at IS NOT NULL AND attachments.deleted_at <= sqlc.arg(cutoff))
   OR (cards.deleted_at IS NOT NULL AND cards.deleted_at <= sqlc.arg(cutoff))
   OR (columns.deleted_at IS NOT NULL AND columns.deleted_at <= sqlc.arg(cutoff))
   OR (boards.deleted_at IS NOT NULL AND boards.deleted_at <= sqlc.arg(cutoff));

-- name: DeleteAttachment :exec
DELETE FROM attachments WHERE id = ?;
