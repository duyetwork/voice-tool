-- name: CreateAIEngine :one
INSERT INTO ai_engine (name, provider, supported_languages, is_active)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAIEngine :one
SELECT * FROM ai_engine WHERE id = $1;

-- name: ListAIEngines :many
SELECT * FROM ai_engine
WHERE (sqlc.narg('only_active')::bool IS NOT TRUE OR is_active = TRUE)
ORDER BY name;

-- name: GetDefaultAIEngine :one
SELECT * FROM ai_engine WHERE is_active = TRUE ORDER BY name LIMIT 1;

-- name: UpdateAIEngine :one
UPDATE ai_engine
SET name                = COALESCE(sqlc.narg('name'), name),
    provider            = COALESCE(sqlc.narg('provider'), provider),
    supported_languages = COALESCE(sqlc.narg('supported_languages'), supported_languages),
    is_active           = COALESCE(sqlc.narg('is_active'), is_active)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteAIEngine :execrows
DELETE FROM ai_engine WHERE id = $1;
