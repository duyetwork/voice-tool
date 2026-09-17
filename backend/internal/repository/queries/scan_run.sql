-- name: StartScanRun :one
-- Mở 1 vòng quét. Dòng này hiện lên bảng kênh là "Đang quét" cho tới khi
-- FinishScanRun đóng nó lại.
INSERT INTO scan_run (
  list_breaking_id, list_scheduled_id, trigger_kind, triggered_by
) VALUES (
  sqlc.narg('list_breaking_id'), sqlc.narg('list_scheduled_id'),
  sqlc.arg('trigger_kind'), sqlc.narg('triggered_by')
)
RETURNING *;

-- name: FinishScanRun :exec
-- Đóng vòng quét kèm kết quả. Ghi một lần ở cuối chứ không cộng dồn từng bài:
-- vòng quét đã có sẵn bộ đếm trong bộ nhớ, và mỗi bài một UPDATE thì một kênh
-- scan_limit=200 thành 200 lượt ghi cho một dòng.
UPDATE scan_run
SET finished_at    = now(),
    status         = sqlc.arg('status'),
    fetched        = sqlc.arg('fetched'),
    posts_created  = sqlc.arg('posts_created'),
    voices_created = sqlc.arg('voices_created'),
    skipped        = sqlc.arg('skipped'),
    error          = sqlc.narg('error')
WHERE id = sqlc.arg('id');

-- name: ListScanRuns :many
-- Lịch sử của ĐÚNG MỘT kênh — service luôn truyền đúng một trong hai id.
SELECT r.*, u.email AS triggered_by_email
FROM scan_run r
LEFT JOIN app_user u ON u.id = r.triggered_by
WHERE (sqlc.narg('list_breaking_id')::uuid  IS NOT NULL AND r.list_breaking_id  = sqlc.narg('list_breaking_id'))
   OR (sqlc.narg('list_scheduled_id')::uuid IS NOT NULL AND r.list_scheduled_id = sqlc.narg('list_scheduled_id'))
ORDER BY r.started_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountScanRuns :one
SELECT COUNT(*) FROM scan_run r
WHERE (sqlc.narg('list_breaking_id')::uuid  IS NOT NULL AND r.list_breaking_id  = sqlc.narg('list_breaking_id'))
   OR (sqlc.narg('list_scheduled_id')::uuid IS NOT NULL AND r.list_scheduled_id = sqlc.narg('list_scheduled_id'));

-- name: SumScanRunsSince :one
-- Tổng kết cửa sổ gần đây, hiện ở đầu tab Lịch sử quét. Cần vì một trang 20
-- dòng không trả lời được "kênh này mấy hôm nay có ra gì không" — mà đó mới là
-- câu hỏi người ta mở tab này để hỏi.
SELECT COUNT(*)::bigint                        AS runs,
       COALESCE(SUM(r.posts_created), 0)::bigint  AS posts_created,
       COALESCE(SUM(r.voices_created), 0)::bigint AS voices_created,
       COUNT(*) FILTER (WHERE r.status = 'error')::bigint AS failed
FROM scan_run r
WHERE ((sqlc.narg('list_breaking_id')::uuid  IS NOT NULL AND r.list_breaking_id  = sqlc.narg('list_breaking_id'))
    OR (sqlc.narg('list_scheduled_id')::uuid IS NOT NULL AND r.list_scheduled_id = sqlc.narg('list_scheduled_id')))
  AND r.started_at >= sqlc.arg('since');

-- name: FailStaleScanRuns :execrows
-- Đóng những vòng quét không bao giờ kết thúc.
--
-- Worker bị kill giữa vòng quét thì không ai chạy FinishScanRun, và dòng đó
-- nằm lại ở 'running' vĩnh viễn. Trên bảng kênh nó chỉ sai cho tới vòng kế
-- tiếp (bảng đọc vòng MỚI NHẤT), nhưng trong tab lịch sử thì nó sai mãi — và
-- một dòng "đang quét" từ tuần trước là thứ khiến người đọc mất lòng tin vào
-- cả bảng.
UPDATE scan_run
SET status      = 'error',
    finished_at = now(),
    error       = COALESCE(error, 'Vòng quét không kết thúc — worker dừng giữa chừng')
WHERE status = 'running' AND started_at < sqlc.arg('before');

-- name: DeleteScanRunsBefore :execrows
DELETE FROM scan_run WHERE started_at < sqlc.arg('before');
