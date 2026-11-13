-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: ListComments :many
-- A card's live comments, oldest first, with each author's display name.
SELECT comments.id, comments.card_id, comments.author_id, users.display_name AS author_name,
       comments.body_json, comments.created_at, comments.updated_at, comments.version
FROM comments
JOIN users ON users.id = comments.author_id
WHERE comments.card_id = ? AND comments.deleted_at IS NULL
ORDER BY comments.created_at, comments.id;

-- name: CreateComment :exec
INSERT INTO comments (id, card_id, author_id, body_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);
