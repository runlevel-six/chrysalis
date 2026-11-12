-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Card descriptions live apart from cards so loading a board never reads
-- them, and they carry their own version: §5.5's edit-conflict check is about
-- concurrent description edits, not about a card being moved or renamed.

-- +goose Up
CREATE TABLE card_descriptions (
    card_id    TEXT PRIMARY KEY REFERENCES cards(id) ON DELETE CASCADE,
    doc_json   TEXT NOT NULL,            -- ProseMirror JSON
    version    INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
) STRICT;

-- +goose Down
DROP TABLE card_descriptions;
