-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, label, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (endpoint) DO UPDATE SET
    user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth, label = excluded.label
RETURNING *;

-- name: ListPushSubscriptions :many
SELECT * FROM push_subscriptions WHERE user_id = ? ORDER BY created_at;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE endpoint = ? AND user_id = ?;

-- name: DeletePushSubscriptionByID :exec
DELETE FROM push_subscriptions WHERE id = ?;

-- name: TouchPushSubscription :exec
UPDATE push_subscriptions SET last_ok_at = ? WHERE id = ?;

-- name: GetServerSetting :one
SELECT value FROM server_settings WHERE key = ?;

-- name: InsertServerSetting :exec
INSERT INTO server_settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO NOTHING;
