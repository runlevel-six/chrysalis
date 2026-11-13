-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Per-user, per-board view settings (docs/DESIGN.md §3.3, §3.4): focus mode,
-- the card filter, hide-completed, and when the board was last opened.
-- View state, not content: last write wins.

-- +goose Up
CREATE TABLE user_board_view (
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    board_id       TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    focus_json     TEXT NOT NULL DEFAULT '{}',
    hide_completed INTEGER NOT NULL DEFAULT 0,
    last_opened_at TEXT,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (user_id, board_id)
) STRICT;

CREATE INDEX user_board_view_last_opened ON user_board_view(user_id, last_opened_at);

-- +goose Down
DROP TABLE user_board_view;
