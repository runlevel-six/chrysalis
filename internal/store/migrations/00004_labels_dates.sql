-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE labels (
    id         TEXT PRIMARY KEY,           -- ULID
    board_id   TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name       TEXT NOT NULL DEFAULT '',   -- may be empty: a color-only label
    color      TEXT NOT NULL,              -- palette key (internal/service/labels.go), mapped to real colors per theme
    position   TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    version    INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX labels_board ON labels(board_id, position);

CREATE TABLE card_labels (
    card_id  TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    label_id TEXT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, label_id)
) STRICT;

CREATE INDEX card_labels_label ON card_labels(label_id);

-- Dates: a date-only value is stored as "YYYY-MM-DD" (no time zone: the
-- same calendar day everywhere); with due_has_time = 1 it's a UTC timestamp.
ALTER TABLE cards ADD COLUMN start_at TEXT;
ALTER TABLE cards ADD COLUMN due_at TEXT;
ALTER TABLE cards ADD COLUMN due_has_time INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE cards DROP COLUMN due_has_time;
ALTER TABLE cards DROP COLUMN due_at;
ALTER TABLE cards DROP COLUMN start_at;
DROP TABLE card_labels;
DROP TABLE labels;
