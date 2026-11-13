-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Web Push for reminders (docs/DESIGN.md section 3.9): each browser or
-- installed app a person turns it on for, the server's VAPID identity, and
-- quiet hours when nothing is pushed.

-- +goose Up
CREATE TABLE push_subscriptions (
    id         TEXT PRIMARY KEY,          -- ULID
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT NOT NULL UNIQUE,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',  -- e.g. "Safari on iPhone", for the person's own reference
    created_at TEXT NOT NULL,
    last_ok_at TEXT
) STRICT;

CREATE INDEX push_subscriptions_user ON push_subscriptions(user_id);

-- Server-wide values the server makes for itself, such as its VAPID key.
CREATE TABLE server_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;

-- "HH:MM" local times; both empty means no quiet hours.
ALTER TABLE user_prefs ADD COLUMN quiet_start TEXT NOT NULL DEFAULT '';
ALTER TABLE user_prefs ADD COLUMN quiet_end TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE user_prefs DROP COLUMN quiet_end;
ALTER TABLE user_prefs DROP COLUMN quiet_start;
DROP TABLE server_settings;
DROP TABLE push_subscriptions;
