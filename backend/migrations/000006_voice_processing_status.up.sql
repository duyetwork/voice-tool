-- ---------------------------------------------------------------------------
-- Voice có thêm trạng thái `processing`.
--
-- Trước đây record voice chỉ được tạo SAU khi worker xử lý xong, nên bấm "Tạo
-- Voice" xong bảng Voice trống trơn cho tới lúc job chạy hết — người dùng
-- tưởng mất bài. Giờ API tạo sẵn record `processing` rồi mới enqueue; worker
-- điền file + metadata vào đúng record đó (hoặc chuyển sang `failed`).
-- ---------------------------------------------------------------------------

ALTER TABLE voice DROP CONSTRAINT IF EXISTS voice_publish_status_check;

ALTER TABLE voice
  ADD CONSTRAINT voice_publish_status_check
  CHECK (publish_status IN ('processing', 'draft', 'ready', 'published', 'failed'));
