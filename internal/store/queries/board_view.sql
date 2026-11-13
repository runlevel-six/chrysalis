-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: GetBoardView :one
SELECT * FROM user_board_view WHERE user_id = ? AND board_id = ?;

-- name: UpsertBoardView :exec
INSERT INTO user_board_view (user_id, board_id, focus_json, hide_completed, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id, board_id) DO UPDATE SET
    focus_json = excluded.focus_json, hide_completed = excluded.hide_completed, updated_at = excluded.updated_at;

-- name: TouchBoardOpened :exec
INSERT INTO user_board_view (user_id, board_id, last_opened_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, board_id) DO UPDATE SET last_opened_at = excluded.last_opened_at;

-- name: LastOpenedBoard :one
-- The board the user opened most recently that they can still see.
SELECT user_board_view.board_id FROM user_board_view
JOIN board_members ON board_members.board_id = user_board_view.board_id AND board_members.user_id = user_board_view.user_id
JOIN boards ON boards.id = user_board_view.board_id
WHERE user_board_view.user_id = ? AND user_board_view.last_opened_at IS NOT NULL
  AND boards.deleted_at IS NULL AND boards.archived_at IS NULL
ORDER BY user_board_view.last_opened_at DESC
LIMIT 1;
