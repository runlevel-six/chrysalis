-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE columns (
    id             TEXT PRIMARY KEY,         -- ULID
    board_id       TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    color          TEXT NOT NULL DEFAULT '', -- '' = no tint
    color_mode     TEXT NOT NULL DEFAULT 'header' CHECK (color_mode IN ('header', 'full')),
    soft_limit     INTEGER,                  -- display only, never blocks
    is_done_column INTEGER NOT NULL DEFAULT 0,
    position       TEXT NOT NULL,            -- fractional index within the board
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    deleted_at     TEXT,                     -- trash
    version        INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX columns_board_position ON columns(board_id, position);

CREATE TABLE cards (
    id           TEXT PRIMARY KEY,           -- ULID
    board_id     TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    column_id    TEXT NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    position     TEXT NOT NULL,              -- fractional index within the column
    completed_at TEXT,
    archived_at  TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    deleted_at   TEXT,                       -- trash
    version      INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX cards_column_position ON cards(column_id, position);
CREATE INDEX cards_board ON cards(board_id);

-- Per-user view state (docs/DESIGN.md §3.3). Not content: no versions,
-- last write wins. No row means "use the board's default".
CREATE TABLE user_card_view (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id    TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    expanded   INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, card_id)
) STRICT;

CREATE TABLE user_column_view (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    column_id  TEXT NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    collapsed  INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, column_id)
) STRICT;

-- +goose Down
DROP TABLE user_column_view;
DROP TABLE user_card_view;
DROP TABLE cards;
DROP TABLE columns;
