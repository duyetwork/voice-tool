-- name: CreateAuditLog :one
INSERT INTO audit_log (user_id, action, object_type, object_id, changes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_log
WHERE (sqlc.narg('object_type')::varchar IS NULL OR object_type = sqlc.narg('object_type'))
  AND (sqlc.narg('object_id')::uuid      IS NULL OR object_id   = sqlc.narg('object_id'))
  AND (sqlc.narg('user_id')::uuid        IS NULL OR user_id     = sqlc.narg('user_id'))
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');
