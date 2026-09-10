-- name: CreatePrompt :one
INSERT INTO prompt (name, content, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPrompt :one
SELECT * FROM prompt WHERE id = $1;

-- name: ListPrompts :many
SELECT * FROM prompt
ORDER BY
  CASE WHEN sqlc.arg('dir')::text = 'asc' THEN created_at END ASC,
  created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: UpdatePrompt :one
UPDATE prompt
SET name    = COALESCE(sqlc.narg('name'), name),
    content = COALESCE(sqlc.narg('content'), content)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeletePrompt :execrows
DELETE FROM prompt WHERE id = $1;

-- name: CountPrompts :one
SELECT COUNT(*) FROM prompt;
