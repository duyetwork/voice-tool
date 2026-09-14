-- name: CreateListBreaking :one
INSERT INTO list_breaking (
  source_url, platform, content_type, collect_mode, prompt_id, regex_patterns,
  language_default, auto_process, auto_publish, status, scan_limit, scan_interval,
  created_by, llm_api_set_id,
  timezone, active_from_min, active_to_min, active_weekdays,
  backfill_limit, max_posts_per_run, random_author
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
  sqlc.narg('llm_api_set_id'), sqlc.arg('timezone'),
  sqlc.narg('active_from_min'), sqlc.narg('active_to_min'), sqlc.arg('active_weekdays'),
  sqlc.arg('backfill_limit'), sqlc.narg('max_posts_per_run'), sqlc.arg('random_author')
)
RETURNING *;

-- name: GetListBreaking :one
SELECT * FROM list_breaking WHERE id = $1;

-- name: ListListBreakings :many
-- Hai số đếm, cùng lý do và cùng cách dựng với ListListScheduleds.
SELECT lb.*, u.email AS created_by_email,
       (SELECT COUNT(*) FROM source_post sp
         WHERE sp.list_breaking_id = lb.id) AS post_count,
       (SELECT COUNT(*) FROM voice v
          JOIN source_post sp2 ON sp2.id = v.source_post_id
         WHERE sp2.list_breaking_id = lb.id) AS voice_count
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
    scan_interval    = COALESCE(sqlc.narg('scan_interval'), scan_interval),
    llm_api_set_id   = COALESCE(sqlc.narg('llm_api_set_id'), llm_api_set_id),
    timezone         = COALESCE(sqlc.narg('timezone'), timezone),
    active_from_min  = CASE WHEN sqlc.arg('clear_window')::bool THEN NULL
                            ELSE COALESCE(sqlc.narg('active_from_min'), active_from_min) END,
    active_to_min    = CASE WHEN sqlc.arg('clear_window')::bool THEN NULL
                            ELSE COALESCE(sqlc.narg('active_to_min'), active_to_min) END,
    active_weekdays  = COALESCE(sqlc.narg('active_weekdays'), active_weekdays),
    backfill_limit   = COALESCE(sqlc.narg('backfill_limit'), backfill_limit),
    -- Cùng lý do với khung giờ: NULL ở max_posts_per_run nghĩa là "không giới
    -- hạn", nên chỉ COALESCE thì người dùng đặt trần rồi không gỡ ra được nữa.
    max_posts_per_run = CASE WHEN sqlc.arg('clear_max_posts')::bool THEN NULL
                             ELSE COALESCE(sqlc.narg('max_posts_per_run'), max_posts_per_run) END,
    random_author     = COALESCE(sqlc.narg('random_author'), random_author)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: MarkListBreakingBackfilled :exec
-- Đóng vòng quét đầu: ghi mốc và nhớ những id đã cố tình bỏ qua.
--
-- Kênh Breaking không có mốc đồng bộ nên nếu không nhớ, chính những bài này sẽ
-- quay lại ở vòng sau như thể vừa đăng.
UPDATE list_breaking
SET backfill_done_at = now(), backfill_excluded_ids = sqlc.arg('excluded_ids')
WHERE id = sqlc.arg('id');

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

-- name: CascadeLanguageFromListBreaking :execrows
-- Đối xứng với CascadeLanguageFromListScheduled — xem lý do ở đó.
UPDATE source_post SET language = sqlc.arg('language')
WHERE list_breaking_id = sqlc.arg('list_id')
  AND language <> sqlc.arg('language');

-- name: CascadeVoiceLanguageFromListBreaking :execrows
UPDATE voice v SET language = sqlc.arg('language')
FROM source_post sp
WHERE sp.id = v.source_post_id
  AND sp.list_breaking_id = sqlc.arg('list_id')
  AND v.voice_file_url IS NULL
  AND v.language <> sqlc.arg('language');

-- name: SetListBreakingScanError :exec
-- Ghi lỗi của vòng quét gần nhất lên kênh, hoặc xoá nó khi vòng quét chạy sạch.
-- Người dùng chỉ nhìn thấy bảng kênh, không nhìn thấy log worker.
UPDATE list_breaking SET last_error = sqlc.narg('last_error') WHERE id = sqlc.arg('id');
