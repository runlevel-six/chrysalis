-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE checklists (
    id         TEXT PRIMARY KEY,         -- ULID
    card_id    TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    position   TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    version    INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX checklists_card ON checklists(card_id, position);

CREATE TABLE checklist_items (
    id           TEXT PRIMARY KEY,       -- ULID
    checklist_id TEXT NOT NULL REFERENCES checklists(id) ON DELETE CASCADE,
    text         TEXT NOT NULL,
    done_at      TEXT,
    position     TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    deleted_at   TEXT,
    version      INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX checklist_items_list ON checklist_items(checklist_id, position);

-- +goose Down
DROP TABLE checklist_items;
DROP TABLE checklists;
