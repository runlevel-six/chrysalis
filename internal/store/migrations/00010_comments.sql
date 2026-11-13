-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Card comments (docs/DESIGN.md §3.1, §7.1). The comments UI is Phase 2;
-- for now comments arrive only through the Trello import (§8) and are
-- readable through the API. The body is a ProseMirror document, like card
-- descriptions.

-- +goose Up
CREATE TABLE comments (
    id         TEXT PRIMARY KEY,          -- ULID
    card_id    TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES users(id),
    body_json  TEXT NOT NULL,             -- ProseMirror JSON
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,                      -- trash
    version    INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX comments_card ON comments(card_id, created_at);

-- +goose Down
DROP TABLE comments;
