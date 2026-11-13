-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: ListQuoteCollectionsForUser :many
-- Live collections the user is a member of, with their role and entry count.
SELECT sqlc.embed(quote_collections), quote_collection_members.role,
       (SELECT COUNT(*) FROM quotes WHERE quotes.collection_id = quote_collections.id AND quotes.deleted_at IS NULL) AS entry_count
FROM quote_collections
JOIN quote_collection_members ON quote_collection_members.collection_id = quote_collections.id
WHERE quote_collection_members.user_id = ? AND quote_collections.deleted_at IS NULL
ORDER BY quote_collections.position, quote_collections.id;

-- name: ListTrashedQuoteCollectionsForOwner :many
SELECT * FROM quote_collections
WHERE owner_id = ? AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC;

-- name: GetQuoteCollectionRole :one
SELECT quote_collection_members.role FROM quote_collection_members
JOIN quote_collections ON quote_collections.id = quote_collection_members.collection_id
WHERE quote_collection_members.user_id = ? AND quote_collection_members.collection_id = ?
  AND quote_collections.deleted_at IS NULL;

-- name: GetQuoteCollection :one
SELECT * FROM quote_collections WHERE id = ?;

-- name: GetQuoteCollectionMemberRole :one
-- The user's role whether or not the collection is in the trash.
SELECT role FROM quote_collection_members WHERE user_id = ? AND collection_id = ?;

-- name: FindQuoteCollectionByName :one
-- The user's own live collection with this name (names compare without case).
SELECT * FROM quote_collections
WHERE owner_id = ? AND deleted_at IS NULL AND lower(name) = lower(sqlc.arg(name))
ORDER BY position LIMIT 1;

-- name: CountQuoteCollectionsForOwner :one
SELECT COUNT(*) FROM quote_collections WHERE owner_id = ? AND deleted_at IS NULL;

-- name: LastQuoteCollectionPosition :one
SELECT position FROM quote_collections WHERE owner_id = ? ORDER BY position DESC LIMIT 1;

-- name: CreateQuoteCollection :one
INSERT INTO quote_collections (id, owner_id, name, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: AddQuoteCollectionMember :exec
INSERT INTO quote_collection_members (collection_id, user_id, role) VALUES (?, ?, ?);

-- name: UpdateQuoteCollection :one
UPDATE quote_collections SET name = ?, position = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetQuoteCollectionDeleted :one
UPDATE quote_collections SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: ListQuotes :many
SELECT * FROM quotes WHERE collection_id = ? AND deleted_at IS NULL ORDER BY position, id;

-- name: ListTrashedQuotesForUser :many
-- Entries trashed one by one from live collections the user can edit.
SELECT quotes.* FROM quotes
JOIN quote_collections ON quote_collections.id = quotes.collection_id
JOIN quote_collection_members ON quote_collection_members.collection_id = quotes.collection_id
WHERE quote_collection_members.user_id = ? AND quote_collection_members.role IN ('owner', 'editor')
  AND quote_collections.deleted_at IS NULL AND quotes.deleted_at IS NOT NULL
ORDER BY quotes.deleted_at DESC
LIMIT 500;

-- name: ListQuotePool :many
-- Live entries in the given collections, among those the user can see, in
-- collection order and then entry order.
SELECT quotes.* FROM quotes
JOIN quote_collections ON quote_collections.id = quotes.collection_id
JOIN quote_collection_members ON quote_collection_members.collection_id = quotes.collection_id
WHERE quote_collection_members.user_id = sqlc.arg(user_id)
  AND quote_collections.deleted_at IS NULL AND quotes.deleted_at IS NULL
  AND quotes.collection_id IN (sqlc.slice(collection_ids))
ORDER BY quote_collections.position, quote_collections.id, quotes.position, quotes.id;

-- name: CountQuotes :one
SELECT COUNT(*) FROM quotes WHERE collection_id = ? AND deleted_at IS NULL;

-- name: GetQuote :one
SELECT * FROM quotes WHERE id = ?;

-- name: LastQuotePosition :one
SELECT position FROM quotes WHERE collection_id = ? ORDER BY position DESC LIMIT 1;

-- name: CreateQuote :one
INSERT INTO quotes (id, collection_id, text, attribution, reference, translation, tags_json, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateQuote :one
UPDATE quotes SET text = ?, attribution = ?, reference = ?, translation = ?, tags_json = ?, position = ?,
    updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetQuoteDeleted :one
UPDATE quotes SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: TrashAllQuotes :execrows
UPDATE quotes SET deleted_at = ?, updated_at = ?, version = version + 1
WHERE collection_id = ? AND deleted_at IS NULL;

-- name: PurgeTrashedQuotes :execrows
DELETE FROM quotes WHERE deleted_at IS NOT NULL AND deleted_at <= ?;

-- name: PurgeTrashedQuoteCollections :execrows
-- Their entries and memberships go with them (ON DELETE CASCADE).
DELETE FROM quote_collections WHERE deleted_at IS NOT NULL AND deleted_at <= ?;

-- name: GetCentering :one
SELECT * FROM user_centering WHERE user_id = ?;

-- name: UpsertCentering :exec
INSERT INTO user_centering (user_id, enabled, mode, rotate_every, order_mode, collection_ids_json,
    pinned_quote_id, current_quote_id, current_period, seen_loads_json, shuffle_seen_json, hidden_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    enabled = excluded.enabled, mode = excluded.mode, rotate_every = excluded.rotate_every,
    order_mode = excluded.order_mode, collection_ids_json = excluded.collection_ids_json,
    pinned_quote_id = excluded.pinned_quote_id, current_quote_id = excluded.current_quote_id,
    current_period = excluded.current_period, seen_loads_json = excluded.seen_loads_json,
    shuffle_seen_json = excluded.shuffle_seen_json, hidden_until = excluded.hidden_until,
    updated_at = excluded.updated_at;

-- name: SetBoardCenteringLine :exec
INSERT INTO user_board_view (user_id, board_id, show_centering_line, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, board_id) DO UPDATE SET
    show_centering_line = excluded.show_centering_line, updated_at = excluded.updated_at;

-- name: ListUsersByEmail :many
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg(email)) ORDER BY created_at;

-- name: ListQuoteCollectionMemberIDs :many
SELECT user_id FROM quote_collection_members WHERE collection_id = ?;
