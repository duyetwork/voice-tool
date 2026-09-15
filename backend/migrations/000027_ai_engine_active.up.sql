-- ---------------------------------------------------------------------------
-- Bật/tắt API key TTS — mỗi người CHỈ MỘT key đang bật.
--
-- Migration 000010 đã bỏ cột is_active với lý do "key hỏng thì xoá, không ai
-- tắt một cái key". Cột quay lại nhưng KHÔNG mang nghĩa cũ:
--
--   nghĩa cũ (000001) : key này còn dùng được không — trạng thái của bản thân
--                       key, và đúng là xoá đi thì gọn hơn.
--   nghĩa mới         : trong những key CỦA MỘT NGƯỜI, key nào đang được dùng
--                       để đọc voice. Đây là một LỰA CHỌN, không phải tình
--                       trạng hỏng/tốt — giữ key cũ lại để tháng sau quay về
--                       dùng tiếp là việc bình thường, xoá đi là mất key.
--
-- Trước migration này luật ngầm là "key mới khai nhất thắng" (GetAIEngineForUser
-- ORDER BY created_at DESC LIMIT 1). Luật đó không nói ra ở đâu trên màn hình:
-- người có 2 key không có cách nào biết voice của mình đang chạy bằng key nào,
-- càng không có cách chọn. Cột này biến luật ngầm thành một công tắc nhìn thấy
-- được.
-- ---------------------------------------------------------------------------

ALTER TABLE ai_engine ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill giữ NGUYÊN hành vi đang chạy: key mới nhất của mỗi người chính là
-- key hệ thống vẫn đang dùng, nên nó là key được bật. Không làm vậy thì sau
-- migration mọi người đều "chưa bật key nào" và toàn bộ voice B/C chết.
UPDATE ai_engine SET is_active = TRUE
WHERE id IN (
  SELECT DISTINCT ON (user_id) id
  FROM ai_engine
  ORDER BY user_id, created_at DESC
);

-- "Chỉ 1 key bật cho 1 người" là luật của DỮ LIỆU, không phải của UI: một
-- request gọi thẳng API, hai tab bấm cùng lúc, hay một bug sau này đều không
-- tạo ra được 2 key cùng bật. Partial index vì nhiều key TẮT của cùng một
-- người là hoàn toàn hợp lệ.
CREATE UNIQUE INDEX uq_ai_engine_active_per_user ON ai_engine(user_id) WHERE is_active;
