-- name: GetAppSetting :one
SELECT * FROM app_setting WHERE key = $1;

-- name: ListAppSettings :many
SELECT * FROM app_setting ORDER BY key;

-- name: UpsertAppSetting :one
INSERT INTO app_setting (key, value, updated_by, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
RETURNING *;
