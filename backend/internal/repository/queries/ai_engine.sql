-- name: CreateAIEngine :one
-- user_id là CHỦ SỞ HỮU key (worker chạy TTS của người đó bằng key này),
-- created_by là người bấm nút — khác nhau khi admin khai hộ.
INSERT INTO ai_engine (user_id, api_key_encrypted, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAIEngine :one
SELECT ae.*, owner.email AS user_email, author.email AS created_by_email
FROM ai_engine ae
JOIN app_user owner  ON owner.id  = ae.user_id
JOIN app_user author ON author.id = ae.created_by
WHERE ae.id = $1;

-- name: ListAIEngines :many
-- `owner` NULL = xem tất cả (admin). User thường luôn được service ép owner =
-- chính mình, nên không đọc được key của người khác dù gọi thẳng API.
SELECT ae.*, owner.email AS user_email, author.email AS created_by_email
FROM ai_engine ae
JOIN app_user owner  ON owner.id  = ae.user_id
JOIN app_user author ON author.id = ae.created_by
WHERE (sqlc.narg('owner')::uuid IS NULL OR ae.user_id = sqlc.narg('owner'))
ORDER BY owner.email, ae.created_at DESC;

-- name: GetAIEngineForUser :one
-- Key mà worker dùng khi chạy TTS cho voice của user này: key mới khai nhất
-- (thay key thì key mới thắng ngay, không phải xoá key cũ trước).
SELECT * FROM ai_engine
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateAIEngine :one
-- api_key_encrypted dùng COALESCE: bỏ trống ô API key ở form nghĩa là "giữ key
-- cũ" — key thật không bao giờ gửi về trình duyệt nên không có gì để gửi lại.
UPDATE ai_engine
SET api_key_encrypted = COALESCE(sqlc.narg('api_key_encrypted'), api_key_encrypted),
    user_id           = COALESCE(sqlc.narg('user_id'), user_id)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: TouchAIEngineUsed :exec
-- Đóng dấu thời điểm key thật sự đọc ra audio. Chỉ gọi sau khi TTS thành công:
-- cột này để người dùng biết key nào còn sống, key nào khai xong bỏ đó.
UPDATE ai_engine SET last_used_at = now() WHERE id = $1;

-- name: DeleteAIEngine :execrows
DELETE FROM ai_engine WHERE id = $1;
