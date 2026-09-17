-- ---------------------------------------------------------------------------
-- Proxy (lối ra mạng cho các request quét)
-- ---------------------------------------------------------------------------

-- name: CreateScrapeProxy :one
INSERT INTO scrape_proxy (label, platform, endpoint_encrypted, endpoint_masked, kind, created_by)
VALUES ($1, sqlc.narg('platform'), $2, $3, $4, $5)
RETURNING *;

-- name: GetScrapeProxy :one
SELECT * FROM scrape_proxy WHERE id = $1;

-- name: ListScrapeProxies :many
SELECT * FROM scrape_proxy
WHERE sqlc.narg('platform')::varchar IS NULL
   OR platform IS NULL
   OR platform = sqlc.narg('platform')
ORDER BY label;

-- name: ClaimScrapeProxy :one
-- Nhận proxy cho MỘT request: nghỉ lâu nhất trước. Chọn-và-đánh-dấu trong một
-- câu, cùng lý do với ClaimScrapeVia.
--
-- `platform IS NULL` lọt vào kết quả: đó là gateway residential dùng chung cho
-- mọi nền tảng. Proxy gán đích danh nền tảng được ưu tiên hơn (ORDER BY đẩy
-- `platform IS NULL` xuống sau) vì gán đích danh là có chủ đích.
--
-- Chỉ lấy `active`: `degraded` và `dead` đứng ngoài vòng chọn cho tới khi có
-- người bật lại tay. Xem MarkScrapeProxyBlocked.
UPDATE scrape_proxy
SET last_used_at = now()
WHERE id = (
  -- Bí danh `c` vì cùng lý do với ClaimScrapeVia.
  SELECT c.id FROM scrape_proxy c
  WHERE c.status = 'active'
    AND (c.platform IS NULL OR c.platform = sqlc.arg('platform'))
  ORDER BY (c.platform IS NULL), c.last_used_at ASC NULLS FIRST, c.id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: UpdateScrapeProxy :one
-- Dán endpoint mới cũng là reset sức khoẻ — cùng lý do với UpdateScrapeVia.
UPDATE scrape_proxy
SET label              = COALESCE(sqlc.narg('label'), label),
    kind               = COALESCE(sqlc.narg('kind'), kind),
    endpoint_encrypted = COALESCE(sqlc.narg('endpoint_encrypted'), endpoint_encrypted),
    endpoint_masked    = COALESCE(sqlc.narg('endpoint_masked'), endpoint_masked),
    status = CASE
               WHEN sqlc.narg('endpoint_encrypted') IS NOT NULL THEN 'active'
               ELSE COALESCE(sqlc.narg('status'), status)
             END,
    consecutive_blocks =
      CASE WHEN sqlc.narg('endpoint_encrypted') IS NULL THEN consecutive_blocks ELSE 0 END,
    last_error =
      CASE WHEN sqlc.narg('endpoint_encrypted') IS NULL THEN last_error ELSE NULL END,
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteScrapeProxy :execrows
DELETE FROM scrape_proxy WHERE id = $1;

-- name: MarkScrapeProxyUsed :exec
-- Request đi qua proxy này xong mà không bị chặn.
UPDATE scrape_proxy
SET used_today = CASE WHEN today < CURRENT_DATE THEN 1 ELSE used_today + 1 END,
    errors_today = CASE WHEN today < CURRENT_DATE THEN 0 ELSE errors_today END,
    today              = CURRENT_DATE,
    consecutive_blocks = 0,
    updated_at         = now()
WHERE id = $1;

-- name: MarkScrapeProxyBlocked :one
-- Nền tảng chặn IP (`bot_block`): đây là lỗi của lối ra mạng.
--
--   chưa đủ ngưỡng      -> vẫn active
--   đủ ngưỡng lần đầu   -> degraded (ra khỏi vòng chọn, còn cứu được bằng tay)
--   vẫn hỏng tiếp       -> dead
--
-- KHÔNG có đường tự hồi sinh theo thời gian, khác hẳn via: một IP đã bị Meta/X
-- liệt thì chờ bao lâu cũng vậy. Tự bật lại chỉ tạo ra một vòng lặp hỏng đều
-- đặn và làm nhiễu số liệu của các proxy còn tốt.
UPDATE scrape_proxy
SET consecutive_blocks = consecutive_blocks + 1,
    errors_today = CASE WHEN today < CURRENT_DATE THEN 1 ELSE errors_today + 1 END,
    used_today   = CASE WHEN today < CURRENT_DATE THEN 1 ELSE used_today END,
    today         = CURRENT_DATE,
    last_error_at = now(),
    last_error    = sqlc.narg('last_error'),
    status = CASE
               WHEN consecutive_blocks + 1 < sqlc.arg('threshold')::int THEN status
               WHEN status = 'degraded' THEN 'dead'
               ELSE 'degraded'
             END,
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: NoteScrapeProxyError :exec
-- Lỗi không quy được cho proxy: đếm vào tỉ lệ hỏng của ngày, không đụng trạng thái.
UPDATE scrape_proxy
SET errors_today = CASE WHEN today < CURRENT_DATE THEN 1 ELSE errors_today + 1 END,
    used_today   = CASE WHEN today < CURRENT_DATE THEN 1 ELSE used_today END,
    today         = CURRENT_DATE,
    last_error_at = now(),
    last_error    = sqlc.narg('last_error'),
    updated_at    = now()
WHERE id = sqlc.arg('id');

-- name: ResetScrapeProxyDaily :execrows
UPDATE scrape_proxy
SET errors_today = 0, used_today = 0, today = CURRENT_DATE, updated_at = now()
WHERE today < CURRENT_DATE;

-- ---------------------------------------------------------------------------
-- via_usage_log
-- ---------------------------------------------------------------------------

-- name: CreateViaUsageLog :exec
INSERT INTO via_usage_log (
  via_id, proxy_id, list_breaking_id, list_scheduled_id,
  platform, result, posts_found, detail
) VALUES (
  $1, sqlc.narg('proxy_id'), sqlc.narg('list_breaking_id'), sqlc.narg('list_scheduled_id'),
  sqlc.arg('platform'), sqlc.arg('result'), sqlc.arg('posts_found'), sqlc.narg('detail')
);

-- name: ListViaUsageLogs :many
SELECT l.*, v.label AS via_label, p.label AS proxy_label
FROM via_usage_log l
JOIN scrape_via v ON v.id = l.via_id
LEFT JOIN scrape_proxy p ON p.id = l.proxy_id
WHERE (sqlc.narg('via_id')::uuid IS NULL OR l.via_id = sqlc.narg('via_id'))
  AND (sqlc.narg('platform')::varchar IS NULL OR l.platform = sqlc.narg('platform'))
ORDER BY l.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountViaUsageLogs :one
SELECT COUNT(*) FROM via_usage_log l
WHERE (sqlc.narg('via_id')::uuid IS NULL OR l.via_id = sqlc.narg('via_id'))
  AND (sqlc.narg('platform')::varchar IS NULL OR l.platform = sqlc.narg('platform'));

-- name: ScrapeHourlyLoad :many
-- Số lượt quét theo GIỜ trong ngày, `days` ngày gần nhất.
--
-- Biểu đồ này trả lời đúng một câu: lịch quét có bị dồn cục không. Vài trăm
-- kênh quét 1 lần/ngày mà tất cả rơi vào cùng một khung giờ thì nền tảng nhìn
-- thấy một đợt tấn công, còn bảng thống kê lỗi thì chỉ nói "bị chặn" mà không
-- nói vì sao — cột giờ này mới nói ra.
SELECT EXTRACT(HOUR FROM created_at)::int AS hour,
       platform,
       COUNT(*)::bigint AS total,
       COUNT(*) FILTER (WHERE result <> 'success')::bigint AS failed
FROM via_usage_log
WHERE created_at >= now() - (sqlc.arg('days')::int * interval '1 day')
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: DeleteViaUsageLogsBefore :execrows
DELETE FROM via_usage_log WHERE created_at < sqlc.arg('before');
