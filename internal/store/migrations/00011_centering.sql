-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- The centering line (docs/DESIGN.md §3.13): collections of short entries
-- (Scripture or quotes), shareable like boards, and each person's own
-- settings and rotation state, kept server-side so every device agrees.

-- +goose Up
CREATE TABLE quote_collections (
    id         TEXT PRIMARY KEY,          -- ULID
    owner_id   TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    position   TEXT NOT NULL,             -- fractional index (internal/order), per owner
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,                      -- soft delete (trash)
    version    INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX quote_collections_owner_position ON quote_collections(owner_id, position);

-- Every collection-scoped query joins here for the current user (§6.2).
CREATE TABLE quote_collection_members (
    collection_id TEXT NOT NULL REFERENCES quote_collections(id) ON DELETE CASCADE,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role          TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    PRIMARY KEY (collection_id, user_id)
) STRICT;

CREATE INDEX quote_collection_members_user ON quote_collection_members(user_id);

CREATE TABLE quotes (
    id            TEXT PRIMARY KEY,       -- ULID
    collection_id TEXT NOT NULL REFERENCES quote_collections(id) ON DELETE CASCADE,
    text          TEXT NOT NULL,
    attribution   TEXT NOT NULL DEFAULT '', -- who said or wrote it
    reference     TEXT NOT NULL DEFAULT '', -- a citation, such as a chapter and verse
    translation   TEXT NOT NULL DEFAULT '', -- such as "WEB"
    tags_json     TEXT NOT NULL DEFAULT '[]',
    position      TEXT NOT NULL,          -- fractional index, within the collection
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT,
    version       INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX quotes_collection_position ON quotes(collection_id, position);

-- One row per person, created the first time they change anything. The line
-- is off until they turn it on.
CREATE TABLE user_centering (
    user_id             TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled             INTEGER NOT NULL DEFAULT 0,
    mode                TEXT NOT NULL DEFAULT 'rotate' CHECK (mode IN ('rotate', 'pinned')),
    rotate_every        TEXT NOT NULL DEFAULT 'daily' CHECK (rotate_every IN ('page_load', 'hourly', 'daily', 'weekly')),
    order_mode          TEXT NOT NULL DEFAULT 'sequential' CHECK (order_mode IN ('sequential', 'shuffle')),
    collection_ids_json TEXT NOT NULL DEFAULT '[]', -- the collections that feed the line
    pinned_quote_id     TEXT REFERENCES quotes(id) ON DELETE SET NULL,
    current_quote_id    TEXT REFERENCES quotes(id) ON DELETE SET NULL,
    -- The rotation period the current entry was chosen for, in the person's
    -- local time ("2026-05-31" for daily, say); '' when not yet chosen.
    current_period      TEXT NOT NULL DEFAULT '',
    -- Page loads that have already had their turn, so a refetch from the same
    -- page doesn't rotate again (page_load interval only).
    seen_loads_json     TEXT NOT NULL DEFAULT '[]',
    -- Shuffle: the entries shown this cycle, in order, so nothing repeats
    -- until all have been shown (and "previous" can walk back).
    shuffle_seen_json   TEXT NOT NULL DEFAULT '[]',
    hidden_until        TEXT,                       -- "hide for today": the next local midnight, UTC
    updated_at          TEXT NOT NULL
) STRICT;

-- The per-board override: NULL follows the person's setting, 0 hides the line
-- on this board, 1 shows it.
ALTER TABLE user_board_view ADD COLUMN show_centering_line INTEGER;

-- +goose Down
ALTER TABLE user_board_view DROP COLUMN show_centering_line;
DROP TABLE user_centering;
DROP TABLE quotes;
DROP TABLE quote_collection_members;
DROP TABLE quote_collections;
