-- name: CreateAIEngine :one
-- user_id là CHỦ SỞ HỮU key (worker chạy TTS của người đó bằng key này),
-- created_by là người bấm nút — khác nhau khi admin khai hộ.
--
-- is_active do service quyết định: key ĐẦU TIÊN của một người thì bật luôn
-- (không thì họ khai key xong voice vẫn báo "chưa bật key nào"), còn người đã
-- có key đang chạy thì key mới vào ở trạng thái tắt — thêm một key dự phòng
-- không được âm thầm đổi giọng đọc của họ.
INSERT INTO ai_engine (user_id, api_key_encrypted, created_by, is_active)
VALUES ($1, $2, $3, $4)
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
--
-- Xếp theo THỜI GIAN TẠO, mới nhất trước — giống mọi bảng khác trong hệ thống.
-- Trước đây admin thấy danh sách gom theo email: key vừa thêm rơi vào giữa
-- bảng theo thứ tự chữ cái, nên thao tác thường gặp nhất (khai key xong xem nó
-- đã vào chưa) lại là thao tác khó nhất.
SELECT ae.*, owner.email AS user_email, author.email AS created_by_email
FROM ai_engine ae
JOIN app_user owner  ON owner.id  = ae.user_id
JOIN app_user author ON author.id = ae.created_by
WHERE (sqlc.narg('owner')::uuid IS NULL OR ae.user_id = sqlc.narg('owner'))
ORDER BY ae.created_at DESC;

-- name: GetAIEngineForUser :one
-- Key mà worker dùng khi chạy TTS cho voice của user này: key ĐANG BẬT.
--
-- Chỉ số ít (uq_ai_engine_active_per_user bảo đảm mỗi người nhiều nhất 1 key
-- bật), nhưng vẫn LIMIT 1 để query không phụ thuộc vào index đó còn tồn tại.
-- Không có dòng nào = người này chưa khai key, hoặc đã tắt hết — cả hai đều
-- không chạy TTS được, và ttsFor nói ra đúng câu đó.
SELECT * FROM ai_engine
WHERE user_id = $1 AND is_active
ORDER BY created_at DESC
LIMIT 1;

-- name: CountActiveAIEngines :one
-- Người này có đang bật key nào không — dùng lúc thêm key mới để quyết định
-- key đó vào ở trạng thái bật hay tắt.
SELECT count(*) FROM ai_engine WHERE user_id = $1 AND is_active;

-- name: UpdateAIEngine :one
-- api_key_encrypted dùng COALESCE: bỏ trống ô API key ở form nghĩa là "giữ key
-- cũ" — key thật không bao giờ gửi về trình duyệt nên không có gì để gửi lại.
UPDATE ai_engine
SET api_key_encrypted = COALESCE(sqlc.narg('api_key_encrypted'), api_key_encrypted),
    user_id           = COALESCE(sqlc.narg('user_id'), user_id)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetAIEngineActive :one
-- Bật/tắt MỘT key. Người gọi chịu trách nhiệm tắt các key khác TRƯỚC khi bật
-- key này (xem DeactivateOtherAIEngines) — làm ngược lại là đụng
-- uq_ai_engine_active_per_user.
UPDATE ai_engine
SET is_active = sqlc.arg('is_active')
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeactivateOtherAIEngines :exec
-- Tắt mọi key khác của cùng một người. Tách thành câu riêng chạy TRƯỚC câu bật
-- vì unique index kiểm tra ngay trên từng dòng: gộp thành một UPDATE
-- `is_active = (id = $1)` có thể chạm đúng lúc hai dòng cùng bật và văng lỗi.
UPDATE ai_engine
SET is_active = FALSE
WHERE user_id = sqlc.arg('user_id') AND id <> sqlc.arg('id') AND is_active;

-- name: ActivateLatestAIEngine :exec
-- Bật key mới nhất cho người này, CHỈ KHI họ không còn key nào đang bật.
--
-- Gọi sau khi xoá hoặc chuyển đi key đang bật: trước khi có công tắc, xoá key
-- đang dùng thì key còn lại tự động thế chỗ (luật "key mới nhất thắng"). Không
-- làm vậy nữa thì một thao tác xoá lại lặng lẽ làm chết mọi voice tiếp theo.
UPDATE ai_engine ae
SET is_active = TRUE
WHERE ae.id = (
  SELECT newest.id FROM ai_engine newest
  WHERE newest.user_id = sqlc.arg('user_id')
  ORDER BY newest.created_at DESC
  LIMIT 1
)
AND NOT EXISTS (
  SELECT 1 FROM ai_engine running
  WHERE running.user_id = sqlc.arg('user_id') AND running.is_active
);

-- name: SetAIEngineStatus :exec
-- Ghi lại điều vừa học được về key sau một lần gọi TTS.
--
-- Chỉ gọi khi lần gọi đó THẬT SỰ nói lên điều gì về key (đọc được audio, hoặc
-- nhà cung cấp từ chối vì key/credit/rate limit). Lỗi mạng và lỗi 5xx của họ
-- không gọi vào đây: ghi 'unknown' đè lên một lần 402 là xoá mất đúng thông
-- tin người dùng cần.
UPDATE ai_engine
SET key_status        = sqlc.arg('key_status'),
    key_status_detail = sqlc.narg('key_status_detail'),
    key_status_at     = now()
WHERE id = sqlc.arg('id');

-- name: TouchAIEngineUsed :exec
-- Đóng dấu thời điểm key thật sự đọc ra audio. Chỉ gọi sau khi TTS thành công:
-- cột này để người dùng biết key nào còn sống, key nào khai xong bỏ đó.
UPDATE ai_engine SET last_used_at = now() WHERE id = $1;

-- name: DeleteAIEngine :execrows
DELETE FROM ai_engine WHERE id = $1;
