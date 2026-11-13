-- Copyright 2026 Jason Hall
-- SPDX-License-Identifier: Apache-2.0

-- name: GetPrefs :one
SELECT * FROM user_prefs WHERE user_id = ?;

-- name: UpsertPrefs :one
INSERT INTO user_prefs (user_id, theme, mode, density, motion, completion_flutter, highlight_overdue,
    reminders_enabled, time_zone, quiet_start, quiet_end, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    theme = excluded.theme, mode = excluded.mode, density = excluded.density, motion = excluded.motion,
    completion_flutter = excluded.completion_flutter, highlight_overdue = excluded.highlight_overdue,
    reminders_enabled = excluded.reminders_enabled, time_zone = excluded.time_zone,
    quiet_start = excluded.quiet_start, quiet_end = excluded.quiet_end,
    updated_at = excluded.updated_at
RETURNING *;
