-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Custom fields per board (docs/DESIGN.md section 4, Phase 2): text, number,
-- date, single choice and checkbox, with values on cards. Each field can show
-- on collapsed cards as a small chip, off by default.

-- +goose Up
CREATE TABLE custom_fields (
    id                TEXT PRIMARY KEY,   -- ULID
    board_id          TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    type              TEXT NOT NULL CHECK (type IN ('text', 'number', 'date', 'select', 'checkbox')),
    options_json      TEXT NOT NULL DEFAULT '[]', -- for select: [{"id","name","color"}]
    show_on_collapsed INTEGER NOT NULL DEFAULT 0,
    position          TEXT NOT NULL,      -- fractional index (internal/order)
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    deleted_at        TEXT,               -- soft delete (trash)
    version           INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE INDEX custom_fields_board ON custom_fields(board_id, position);

CREATE TABLE card_field_values (
    card_id    TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    field_id   TEXT NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
    value_json TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (card_id, field_id)
) STRICT;

CREATE INDEX card_field_values_field ON card_field_values(field_id);

-- +goose Down
DROP TABLE card_field_values;
DROP TABLE custom_fields;
