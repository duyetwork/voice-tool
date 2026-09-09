-- name: CreatePrompt :one
INSERT INTO prompt (name, content, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPrompt :one
SELECT * FROM prompt WHERE id = $1;

-- name: ListPrompts :many
SELECT * FROM prompt
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdatePrompt :one
UPDATE prompt
SET name    = COALESCE(sqlc.narg('name'), name),
    content = COALESCE(sqlc.narg('content'), content)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeletePrompt :execrows
DELETE FROM prompt WHERE id = $1;
