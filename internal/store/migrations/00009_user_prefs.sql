-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Personal preferences (docs/DESIGN.md §3.5, §3.6, §3.12). Defaults follow
-- calm-by-default: follow the system for light/dark and motion, the
-- completion flutter on (it can be switched off), overdue highlighting off.

-- +goose Up
CREATE TABLE user_prefs (
    user_id            TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme              TEXT NOT NULL DEFAULT 'default',
    mode               TEXT NOT NULL DEFAULT 'system' CHECK (mode IN ('system', 'light', 'dark')),
    density            TEXT NOT NULL DEFAULT 'comfortable' CHECK (density IN ('comfortable', 'compact')),
    motion             TEXT NOT NULL DEFAULT 'system' CHECK (motion IN ('system', 'reduce', 'allow')),
    completion_flutter INTEGER NOT NULL DEFAULT 1,
    highlight_overdue  INTEGER NOT NULL DEFAULT 0,
    updated_at         TEXT NOT NULL
) STRICT;

-- +goose Down
DROP TABLE user_prefs;
