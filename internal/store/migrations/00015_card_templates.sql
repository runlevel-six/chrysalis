-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- Card templates (docs/DESIGN.md section 4, Phase 2): a card kept as a
-- pattern for new ones. Templates stay out of the columns; they're listed
-- in Board settings and offered when adding a card.

-- +goose Up
ALTER TABLE cards ADD COLUMN is_template INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE cards DROP COLUMN is_template;
