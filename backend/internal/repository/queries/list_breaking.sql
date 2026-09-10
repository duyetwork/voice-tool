-- name: CreateListBreaking :one
INSERT INTO list_breaking (
  source_url, platform, content_type, collect_mode, prompt_id, regex_patterns,
  language_default, auto_process, auto_publish, status, scan_limit, scan_interval,
  created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetListBreaking :one
SELECT * FROM list_breaking WHERE id = $1;

-- name: ListListBreakings :many
SELECT lb.*, u.email AS created_by_email
FROM list_breaking lb
JOIN app_user u ON u.id = lb.created_by
WHERE (sqlc.narg('status')::varchar   IS NULL OR lb.status     = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar IS NULL OR lb.platform   = sqlc.narg('platform'))
  AND (sqlc.narg('created_by')::uuid  IS NULL OR lb.created_by = sqlc.narg('created_by'))
  AND (sqlc.narg('search')::text      IS NULL OR lb.source_url ~* sqlc.narg('search'))
-- Sắp xếp động theo cột thời gian đang chọn; mặc định kênh mới nhất trước.
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'last_scanned_at' AND sqlc.arg('dir')::text = 'asc'
       THEN lb.last_scanned_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'last_scanned_at' AND sqlc.arg('dir')::text = 'desc'
       THEN lb.last_scanned_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('dir')::text = 'asc' AND sqlc.arg('sort')::text <> 'last_scanned_at'
       THEN lb.created_at END ASC,
  lb.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListActiveListBreakings :many
SELECT * FROM list_breaking WHERE status = 'active' ORDER BY created_at;

-- name: ListDueListBreakings :many
-- Chỉ lấy kênh đã quá khoảng nghỉ của chính nó (scan_interval), fallback về
-- khoảng nghỉ mặc định của hệ thống khi kênh không cấu hình riêng.
SELECT * FROM list_breaking
WHERE status = 'active'
  AND (
    last_scanned_at IS NULL
    OR last_scanned_at + COALESCE(scan_interval, sqlc.arg('default_interval')::interval) <= now()
  )
ORDER BY last_scanned_at ASC NULLS FIRST;

-- name: UpdateListBreaking :one
UPDATE list_breaking
SET source_url       = COALESCE(sqlc.narg('source_url'), source_url),
    platform         = COALESCE(sqlc.narg('platform'), platform),
    content_type     = COALESCE(sqlc.narg('content_type'), content_type),
    collect_mode     = COALESCE(sqlc.narg('collect_mode'), collect_mode),
    prompt_id        = COALESCE(sqlc.narg('prompt_id'), prompt_id),
    regex_patterns   = COALESCE(sqlc.narg('regex_patterns'), regex_patterns),
    language_default = COALESCE(sqlc.narg('language_default'), language_default),
    auto_process     = COALESCE(sqlc.narg('auto_process'), auto_process),
    auto_publish     = COALESCE(sqlc.narg('auto_publish'), auto_publish),
    status           = COALESCE(sqlc.narg('status'), status),
    scan_limit       = COALESCE(sqlc.narg('scan_limit'), scan_limit),
    scan_interval    = COALESCE(sqlc.narg('scan_interval'), scan_interval)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: TouchListBreakingScanned :exec
UPDATE list_breaking SET last_scanned_at = now() WHERE id = $1;

-- name: DeleteListBreaking :execrows
DELETE FROM list_breaking WHERE id = $1;

-- name: CountListBreakings :one
-- Tổng số kênh khớp bộ lọc, để bảng phân trang biết có bao nhiêu trang.
SELECT COUNT(*)
FROM list_breaking lb
WHERE (sqlc.narg('status')::varchar   IS NULL OR lb.status     = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar IS NULL OR lb.platform   = sqlc.narg('platform'))
  AND (sqlc.narg('created_by')::uuid  IS NULL OR lb.created_by = sqlc.narg('created_by'))
  AND (sqlc.narg('search')::text      IS NULL OR lb.source_url ~* sqlc.narg('search'));
