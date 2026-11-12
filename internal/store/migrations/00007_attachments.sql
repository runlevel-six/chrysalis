-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE attachments (
    id          TEXT PRIMARY KEY,           -- ULID
    board_id    TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    card_id     TEXT REFERENCES cards(id) ON DELETE CASCADE, -- NULL: board-level (backgrounds)
    uploader_id TEXT NOT NULL REFERENCES users(id),
    kind        TEXT NOT NULL CHECK (kind IN ('file', 'image', 'link')),
    filename    TEXT NOT NULL DEFAULT '',   -- display name only; never used as a path
    mime        TEXT NOT NULL DEFAULT '',   -- sniffed from the bytes, not the client's claim
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    storage_key TEXT NOT NULL DEFAULT '',
    thumb_key   TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',   -- for links
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT
) STRICT;

CREATE INDEX attachments_card ON attachments(card_id);
CREATE INDEX attachments_board ON attachments(board_id);

-- Card covers: a palette color, or one of the card's image attachments.
ALTER TABLE cards ADD COLUMN cover_type TEXT NOT NULL DEFAULT 'none' CHECK (cover_type IN ('none', 'color', 'image'));
ALTER TABLE cards ADD COLUMN cover_value TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE cards DROP COLUMN cover_value;
ALTER TABLE cards DROP COLUMN cover_type;
DROP TABLE attachments;
