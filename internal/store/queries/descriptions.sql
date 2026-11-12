-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: GetCardDescription :one
SELECT * FROM card_descriptions WHERE card_id = ?;

-- name: UpsertCardDescription :one
INSERT INTO card_descriptions (card_id, doc_json, version, updated_at, updated_by)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (card_id) DO UPDATE SET
    doc_json = excluded.doc_json, version = excluded.version,
    updated_at = excluded.updated_at, updated_by = excluded.updated_by
RETURNING *;

-- name: ListCardsWithDescription :many
SELECT card_descriptions.card_id FROM card_descriptions
JOIN cards ON cards.id = card_descriptions.card_id
WHERE cards.board_id = ? AND card_descriptions.doc_json != '';
