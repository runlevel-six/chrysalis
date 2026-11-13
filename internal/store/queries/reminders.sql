-- name: GetReminder :one
SELECT * FROM card_reminders WHERE card_id = ? AND user_id = ?;

-- name: UpsertReminder :one
INSERT INTO card_reminders (card_id, user_id, kind, offset_minutes, at, time_zone, remind_at, snoozed_until, fired_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)
ON CONFLICT (card_id, user_id) DO UPDATE SET
    kind = excluded.kind, offset_minutes = excluded.offset_minutes, at = excluded.at,
    time_zone = excluded.time_zone, remind_at = excluded.remind_at,
    snoozed_until = NULL, fired_at = NULL, updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteReminder :exec
DELETE FROM card_reminders WHERE card_id = ? AND user_id = ?;

-- name: ListCardReminders :many
SELECT * FROM card_reminders WHERE card_id = ?;

-- name: SetReminderSchedule :exec
UPDATE card_reminders SET remind_at = ?, fired_at = ?, updated_at = ? WHERE card_id = ? AND user_id = ?;

-- name: SnoozeReminder :exec
UPDATE card_reminders SET snoozed_until = ?, fired_at = NULL, updated_at = ? WHERE card_id = ? AND user_id = ?;

-- name: ListDueReminders :many
SELECT card_reminders.* FROM card_reminders
WHERE card_reminders.fired_at IS NULL
  AND COALESCE(card_reminders.snoozed_until, card_reminders.remind_at) IS NOT NULL
  AND COALESCE(card_reminders.snoozed_until, card_reminders.remind_at) <= ?
ORDER BY COALESCE(card_reminders.snoozed_until, card_reminders.remind_at)
LIMIT 500;

-- name: MarkReminderFired :exec
UPDATE card_reminders SET fired_at = ?, snoozed_until = NULL WHERE card_id = ? AND user_id = ?;

-- name: CreateNotification :one
INSERT INTO notifications (id, user_id, card_id, board_id, kind, text, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListNotifications :many
SELECT * FROM notifications
WHERE user_id = ? AND dismissed_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 100;

-- name: GetNotification :one
SELECT * FROM notifications WHERE id = ? AND user_id = ?;

-- name: MarkNotificationsRead :exec
UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL;

-- name: DismissNotification :exec
UPDATE notifications SET dismissed_at = ?, read_at = COALESCE(read_at, ?) WHERE id = ? AND user_id = ?;

-- name: DismissAllNotifications :exec
UPDATE notifications SET dismissed_at = ?, read_at = COALESCE(read_at, ?) WHERE user_id = ? AND dismissed_at IS NULL;

-- name: PurgeOldNotifications :execrows
DELETE FROM notifications WHERE created_at <= ?;
