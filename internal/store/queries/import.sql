-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: ImportColumn :exec
-- Bulk inserts for importing a whole board (docs/DESIGN.md section 8) set every
-- field in one statement.
INSERT INTO columns (id, board_id, name, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ImportCard :exec
INSERT INTO cards (id, board_id, column_id, title, position, completed_at, archived_at,
                   start_at, due_at, due_has_time, cover_type, cover_value, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ImportLabel :exec
INSERT INTO labels (id, board_id, name, color, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ImportChecklist :exec
INSERT INTO checklists (id, card_id, title, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ImportChecklistItem :exec
INSERT INTO checklist_items (id, checklist_id, text, done_at, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: SetBoardBackground :exec
UPDATE boards SET background_type = ?, background_value = ? WHERE id = ?;
