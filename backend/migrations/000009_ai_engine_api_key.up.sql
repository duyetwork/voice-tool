-- ---------------------------------------------------------------------------
-- AI Engine trở thành "API key của từng người", không còn là danh mục chung.
--
-- Vì sao: TTS chỉ còn 1 nhà cung cấp (3voices), nên thứ thật sự cần quản lý
-- không phải "chọn engine nào" mà là "key của ai". Mỗi user tự khai key của
-- mình, worker đọc bằng đúng key của người sở hữu voice — quota và chi phí về
-- đúng người dùng.
--
--   * api_key_encrypted: AES-256-GCM như token multime (TOKEN_ENCRYPTION_KEY).
--     Không bao giờ trả về API; API chỉ trả bản che (4 ký tự cuối).
--   * voice_id: giọng đã lưu bên 3voices, để trống thì dùng /tts/design.
--   * created_by: chủ sở hữu. Admin thấy tất cả, user chỉ thấy key của mình.
-- ---------------------------------------------------------------------------

ALTER TABLE ai_engine
  ADD COLUMN api_key_encrypted TEXT,
  ADD COLUMN voice_id          TEXT,
  ADD COLUMN created_by        UUID REFERENCES app_user(id) ON DELETE CASCADE;

-- Engine cũ (tạo khi chưa có khái niệm chủ sở hữu) về tay admin đầu tiên để
-- không mồ côi; không có user nào thì bảng đang rỗng hoặc chỉ có rác dev.
UPDATE ai_engine
SET created_by = (
  SELECT id FROM app_user
  ORDER BY (role = 'admin') DESC, created_at
  LIMIT 1
)
WHERE created_by IS NULL;

DELETE FROM ai_engine WHERE created_by IS NULL;

ALTER TABLE ai_engine ALTER COLUMN created_by SET NOT NULL;

-- Chỉ còn 1 provider: mọi bản ghi cũ (mock/elevenlabs) quy về 3voices.
UPDATE ai_engine SET provider = '3voices' WHERE provider <> '3voices';
ALTER TABLE ai_engine
  ALTER COLUMN provider SET DEFAULT '3voices',
  ADD CONSTRAINT ck_ai_engine_provider CHECK (provider = '3voices');

-- Worker tra key theo chủ sở hữu ở mỗi lần chạy TTS.
CREATE INDEX idx_ai_engine_created_by ON ai_engine(created_by);
