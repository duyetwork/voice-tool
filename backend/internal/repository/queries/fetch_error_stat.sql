-- name: BumpFetchErrorStat :exec
-- Đếm 1 lần bị nền tảng chặn. Gộp theo ngày nên bảng không phình theo từng lỗi,
-- và vẫn đủ để trả lời câu hỏi duy nhất nó sinh ra để trả lời: nền tảng nào
-- đang bị chặn nhiều tới mức đáng mua proxy.
INSERT INTO fetch_error_stat (day, platform, kind, count, last_at)
VALUES (CURRENT_DATE, $1, $2, 1, now())
ON CONFLICT (day, platform, kind) DO UPDATE
SET count = fetch_error_stat.count + 1, last_at = now();

-- name: ListFetchErrorStats :many
SELECT * FROM fetch_error_stat
WHERE day >= CURRENT_DATE - sqlc.arg('days')::int
ORDER BY day DESC, platform, kind;
