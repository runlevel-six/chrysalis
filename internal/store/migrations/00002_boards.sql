-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE boards (
    id               TEXT PRIMARY KEY,      -- ULID
    owner_id         TEXT NOT NULL REFERENCES users(id),
    name             TEXT NOT NULL,
    icon             TEXT NOT NULL DEFAULT '',
    background_type  TEXT NOT NULL DEFAULT 'color' CHECK (background_type IN ('color', 'gradient', 'image')),
    background_value TEXT NOT NULL DEFAULT '', -- '' = the theme's default surface
    settings_json    TEXT NOT NULL DEFAULT '{}',
    position         TEXT NOT NULL,         -- fractional index (internal/order), per owner
    archived_at      TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    deleted_at       TEXT,                  -- soft delete (trash)
    version          INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX boards_owner_position ON boards(owner_id, position);

-- Every board-scoped query joins here for the current user (docs/DESIGN.md §6.2).
CREATE TABLE board_members (
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role     TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    PRIMARY KEY (board_id, user_id)
) STRICT;

CREATE INDEX board_members_user ON board_members(user_id);

-- +goose Down
DROP TABLE board_members;
DROP TABLE boards;
