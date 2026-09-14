-- name: CreateListScheduled :one
INSERT INTO list_scheduled (
  source_url, platform, content_type, collect_mode, prompt_id, scan_frequency,
  language_default, auto_process, auto_publish, status, scan_limit,
  max_posts_per_run, created_by, llm_api_set_id,
  timezone, active_from_min, active_to_min, active_weekdays, fixed_times_min,
  backfill_limit
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
  sqlc.narg('llm_api_set_id'), sqlc.arg('timezone'),
  sqlc.narg('active_from_min'), sqlc.narg('active_to_min'),
  sqlc.arg('active_weekdays'), sqlc.arg('fixed_times_min'),
  sqlc.arg('backfill_limit')
)
RETURNING *;

-- name: GetListScheduled :one
SELECT * FROM list_scheduled WHERE id = $1;

-- name: ListListScheduleds :many
SELECT ls.*, u.email AS created_by_email
FROM list_scheduled ls
JOIN app_user u ON u.id = ls.created_by
WHERE (sqlc.narg('status')::varchar   IS NULL OR ls.status     = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar IS NULL OR ls.platform   = sqlc.narg('platform'))
  AND (sqlc.narg('created_by')::uuid  IS NULL OR ls.created_by = sqlc.narg('created_by'))
  AND (sqlc.narg('search')::text      IS NULL OR ls.source_url ~* sqlc.narg('search'))
-- Sắp xếp động theo cột thời gian đang chọn; mặc định kênh mới nhất trước.
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'last_scanned_at' AND sqlc.arg('dir')::text = 'asc'
       THEN ls.last_scanned_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'last_scanned_at' AND sqlc.arg('dir')::text = 'desc'
       THEN ls.last_scanned_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('dir')::text = 'asc' AND sqlc.arg('sort')::text <> 'last_scanned_at'
       THEN ls.created_at END ASC,
  ls.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListActiveListScheduleds :many
SELECT * FROM list_scheduled WHERE status = 'active' ORDER BY created_at;

-- name: UpdateListScheduled :one
UPDATE list_scheduled
SET source_url        = COALESCE(sqlc.narg('source_url'), source_url),
    platform          = COALESCE(sqlc.narg('platform'), platform),
    content_type      = COALESCE(sqlc.narg('content_type'), content_type),
    collect_mode      = COALESCE(sqlc.narg('collect_mode'), collect_mode),
    prompt_id         = COALESCE(sqlc.narg('prompt_id'), prompt_id),
    scan_frequency    = COALESCE(sqlc.narg('scan_frequency'), scan_frequency),
    language_default  = COALESCE(sqlc.narg('language_default'), language_default),
    auto_process      = COALESCE(sqlc.narg('auto_process'), auto_process),
    auto_publish      = COALESCE(sqlc.narg('auto_publish'), auto_publish),
    status            = COALESCE(sqlc.narg('status'), status),
    scan_limit        = COALESCE(sqlc.narg('scan_limit'), scan_limit),
    backfill_limit    = COALESCE(sqlc.narg('backfill_limit'), backfill_limit),
    -- Cùng lý do với khung giờ: NULL ở max_posts_per_run nghĩa là "không giới
    -- hạn", nên chỉ COALESCE thì người dùng đặt trần rồi không gỡ ra được nữa.
    max_posts_per_run = CASE WHEN sqlc.arg('clear_max_posts')::bool THEN NULL
                             ELSE COALESCE(sqlc.narg('max_posts_per_run'), max_posts_per_run) END,
    llm_api_set_id    = COALESCE(sqlc.narg('llm_api_set_id'), llm_api_set_id),
    timezone          = COALESCE(sqlc.narg('timezone'), timezone),
    -- Khung giờ dùng cờ `clear_*` chứ không chỉ COALESCE: NULL ở đây vừa có
    -- nghĩa "không sửa" vừa có nghĩa "bỏ khung giờ, quay lại 24/7", và chỉ
    -- COALESCE thì người dùng không bao giờ xoá được khung giờ đã đặt.
    active_from_min   = CASE WHEN sqlc.arg('clear_window')::bool THEN NULL
                             ELSE COALESCE(sqlc.narg('active_from_min'), active_from_min) END,
    active_to_min     = CASE WHEN sqlc.arg('clear_window')::bool THEN NULL
                             ELSE COALESCE(sqlc.narg('active_to_min'), active_to_min) END,
    active_weekdays   = COALESCE(sqlc.narg('active_weekdays'), active_weekdays),
    fixed_times_min   = COALESCE(sqlc.narg('fixed_times_min'), fixed_times_min)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: MarkListScheduledBackfilled :exec
-- Đóng vòng quét đầu. Cột riêng chứ không suy ra từ last_synced_post_id: kênh
-- chưa có bài nào thì mốc đó vẫn NULL sau một vòng quét hoàn toàn hợp lệ.
UPDATE list_scheduled SET backfill_done_at = now() WHERE id = $1;

-- name: SetLastSyncedPostID :exec
UPDATE list_scheduled
SET last_synced_post_id = $2, last_scanned_at = now()
WHERE id = $1;

-- name: TouchListScheduledScanned :exec
UPDATE list_scheduled SET last_scanned_at = now() WHERE id = $1;

-- name: DeleteListScheduled :execrows
DELETE FROM list_scheduled WHERE id = $1;

-- name: CountListScheduleds :one
-- Tổng số kênh khớp bộ lọc, để bảng phân trang biết có bao nhiêu trang.
SELECT COUNT(*)
FROM list_scheduled ls
WHERE (sqlc.narg('status')::varchar   IS NULL OR ls.status     = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar IS NULL OR ls.platform   = sqlc.narg('platform'))
  AND (sqlc.narg('created_by')::uuid  IS NULL OR ls.created_by = sqlc.narg('created_by'))
  AND (sqlc.narg('search')::text      IS NULL OR ls.source_url ~* sqlc.narg('search'));
