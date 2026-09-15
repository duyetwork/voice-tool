-- name: CreateSkippedLog :exec
INSERT INTO skipped_log (list_breaking_id, post_id_external, reason, post_url, text_excerpt)
VALUES ($1, $2, $3, sqlc.narg('post_url'), sqlc.narg('text_excerpt'));

-- name: ListSkippedLogs :many
SELECT * FROM skipped_log
WHERE list_breaking_id = $1
ORDER BY checked_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountSkippedLogs :one
SELECT COUNT(*) FROM skipped_log WHERE list_breaking_id = $1;

-- name: CountSkippedLogsSince :one
-- Tỉ lệ bỏ qua giúp đánh giá regex có quá chặt/quá lỏng hay không.
SELECT COUNT(*) FROM skipped_log
WHERE list_breaking_id = $1 AND checked_at >= sqlc.arg('since');

-- name: DeleteSkippedLogsBefore :execrows
DELETE FROM skipped_log WHERE checked_at < sqlc.arg('before');
