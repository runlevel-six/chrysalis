-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Reminders (docs/DESIGN.md section 3.9): opt-in globally (off by default)
-- and per card, personal to each person, delivered to an in-app list.

-- +goose Up
ALTER TABLE user_prefs ADD COLUMN reminders_enabled INTEGER NOT NULL DEFAULT 0;
-- The person's IANA time zone, sent by their browser, for "today" and
-- "tomorrow" and for when a date-only due date's reminder goes off.
ALTER TABLE user_prefs ADD COLUMN time_zone TEXT NOT NULL DEFAULT '';

CREATE TABLE card_reminders (
    card_id        TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- due: when it's due; before: offset_minutes before that; at: a set time.
    kind           TEXT NOT NULL CHECK (kind IN ('due', 'before', 'at')),
    offset_minutes INTEGER NOT NULL DEFAULT 0 CHECK (offset_minutes >= 0),
    at             TEXT,                  -- UTC, for 'at'
    time_zone      TEXT NOT NULL DEFAULT 'UTC',
    remind_at      TEXT,                  -- UTC; NULL when it can't be worked out (no due date)
    snoozed_until  TEXT,                  -- UTC; overrides remind_at once
    fired_at       TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (card_id, user_id)
) STRICT;

CREATE INDEX card_reminders_pending ON card_reminders(remind_at, snoozed_until) WHERE fired_at IS NULL;
CREATE INDEX card_reminders_user ON card_reminders(user_id);

CREATE TABLE notifications (
    id           TEXT PRIMARY KEY,         -- ULID
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id      TEXT REFERENCES cards(id) ON DELETE CASCADE,
    board_id     TEXT REFERENCES boards(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('reminder')),
    text         TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    read_at      TEXT,
    dismissed_at TEXT
) STRICT;

CREATE INDEX notifications_user ON notifications(user_id, created_at);

-- +goose Down
DROP TABLE notifications;
DROP TABLE card_reminders;
ALTER TABLE user_prefs DROP COLUMN time_zone;
ALTER TABLE user_prefs DROP COLUMN reminders_enabled;
