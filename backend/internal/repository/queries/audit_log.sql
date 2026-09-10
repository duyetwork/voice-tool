-- name: CreateAuditLog :one
INSERT INTO audit_log (user_id, action, object_type, object_id, changes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListAuditLogs :many
-- Kèm email người thao tác để bảng nhật ký hiện được "ai làm" mà không phải
-- gọi thêm API. LEFT JOIN cho chắc: bản ghi audit không được mất chỉ vì user
-- tham chiếu không đọc được.
SELECT al.*, u.email AS user_email
FROM audit_log al
LEFT JOIN app_user u ON u.id = al.user_id
WHERE (sqlc.narg('object_type')::varchar IS NULL OR al.object_type = sqlc.narg('object_type'))
  AND (sqlc.narg('object_id')::uuid      IS NULL OR al.object_id   = sqlc.narg('object_id'))
  AND (sqlc.narg('user_id')::uuid        IS NULL OR al.user_id     = sqlc.narg('user_id'))
ORDER BY
  CASE WHEN sqlc.arg('dir')::text = 'asc' THEN al.created_at END ASC,
  al.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountAuditLogs :one
-- Tổng số bản ghi khớp bộ lọc, để bảng phân trang biết có bao nhiêu trang.
SELECT COUNT(*) FROM audit_log
WHERE (sqlc.narg('object_type')::varchar IS NULL OR object_type = sqlc.narg('object_type'))
  AND (sqlc.narg('object_id')::uuid      IS NULL OR object_id   = sqlc.narg('object_id'))
  AND (sqlc.narg('user_id')::uuid        IS NULL OR user_id     = sqlc.narg('user_id'));
