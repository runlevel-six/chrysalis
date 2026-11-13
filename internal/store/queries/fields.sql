-- name: CreateField :one
INSERT INTO custom_fields (id, board_id, name, type, options_json, show_on_collapsed, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetField :one
SELECT * FROM custom_fields WHERE id = ?;

-- name: ListFields :many
SELECT * FROM custom_fields WHERE board_id = ? AND deleted_at IS NULL ORDER BY position, id;

-- name: CountFields :one
SELECT COUNT(*) FROM custom_fields WHERE board_id = ? AND deleted_at IS NULL;

-- name: LastFieldPosition :one
SELECT position FROM custom_fields WHERE board_id = ? ORDER BY position DESC LIMIT 1;

-- name: UpdateField :one
UPDATE custom_fields SET name = ?, options_json = ?, show_on_collapsed = ?, position = ?,
    updated_at = ?, version = version + 1
WHERE id = ?
RETURNING *;

-- name: SetFieldDeleted :one
UPDATE custom_fields SET deleted_at = ?, updated_at = ?, version = version + 1 WHERE id = ? RETURNING *;

-- name: ListBoardFieldValues :many
SELECT card_field_values.* FROM card_field_values
JOIN cards ON cards.id = card_field_values.card_id
JOIN custom_fields ON custom_fields.id = card_field_values.field_id
WHERE cards.board_id = ? AND custom_fields.deleted_at IS NULL;

-- name: ListCardFieldValues :many
SELECT card_field_values.* FROM card_field_values
JOIN custom_fields ON custom_fields.id = card_field_values.field_id
WHERE card_field_values.card_id = ? AND custom_fields.deleted_at IS NULL;

-- name: UpsertFieldValue :exec
INSERT INTO card_field_values (card_id, field_id, value_json, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (card_id, field_id) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at;

-- name: DeleteFieldValue :exec
DELETE FROM card_field_values WHERE card_id = ? AND field_id = ?;

-- name: ListFieldValues :many
SELECT * FROM card_field_values WHERE field_id = ?;

-- name: ClearCardFieldValues :exec
DELETE FROM card_field_values WHERE card_id = ?;

-- name: PurgeTrashedFields :execrows
DELETE FROM custom_fields WHERE deleted_at IS NOT NULL AND deleted_at <= ?;
