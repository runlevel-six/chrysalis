-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Timestamps are stored as fixed-width UTC RFC 3339 strings
-- ("2006-01-02T15:04:05.000Z") so string comparison in SQL matches time order.

-- +goose Up
CREATE TABLE users (
    id           TEXT PRIMARY KEY,          -- ULID
    oidc_sub     TEXT NOT NULL UNIQUE,      -- OIDC sub, Remote-User, or the dev user
    email        TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
) STRICT;

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,            -- SHA-256 (hex) of the cookie token; the raw token is never stored
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX sessions_user_id ON sessions(user_id);
CREATE INDEX sessions_expires_at ON sessions(expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
