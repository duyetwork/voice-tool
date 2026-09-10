-- ---------------------------------------------------------------------------
-- ai_engine trở thành SỔ API KEY thuần.
--
-- Sau khi chốt 3voices là nhà cung cấp TTS duy nhất, mọi cột còn lại của bảng
-- đều là câu trả lời cho một câu hỏi không ai còn hỏi nữa:
--
--   * name                — người dùng không đặt tên cho một thứ chỉ có 1 loại
--   * supported_languages — 3voices công bố danh sách ngôn ngữ của chính nó
--                           (tts.ThreeVoices.SupportedLanguages), khai lại bằng
--                           tay chỉ tạo ra một bản sao sai lệch theo thời gian
--   * is_active           — key hỏng thì xoá, không ai "tắt" một cái key
--
-- Cái THẬT SỰ cần quản lý chỉ còn: key của ai, ai khai nó, khai lúc nào, lần
-- cuối chạy TTS là khi nào.
--
--   * user_id      — CHỦ SỞ HỮU key: worker chạy TTS cho voice của người này
--                    bằng key này, nên quota và hoá đơn 3voices về đúng họ.
--   * created_by   — NGƯỜI KHAI. Thường trùng user_id; khác nhau khi admin
--                    khai hộ (mua key rồi gán cho từng người).
--   * last_used_at — lần gần nhất key được dùng để đọc một voice.
-- ---------------------------------------------------------------------------

ALTER TABLE ai_engine
  ADD COLUMN user_id      UUID REFERENCES app_user(id) ON DELETE CASCADE,
  ADD COLUMN last_used_at TIMESTAMPTZ;

-- Trước migration này chỉ có 1 khái niệm "người": người khai cũng là chủ sở hữu.
UPDATE ai_engine SET user_id = created_by WHERE user_id IS NULL;

-- Bản ghi không có key không đọc được chữ nào — nó là rác từ thời bảng này còn
-- là danh mục engine, giữ lại chỉ làm bẩn màn hình quản lý key.
DELETE FROM ai_engine WHERE user_id IS NULL OR api_key_encrypted IS NULL;

ALTER TABLE ai_engine
  ALTER COLUMN user_id           SET NOT NULL,
  ALTER COLUMN api_key_encrypted SET NOT NULL;

ALTER TABLE ai_engine
  DROP COLUMN name,
  DROP COLUMN supported_languages,
  DROP COLUMN is_active;

-- Xoá key là thao tác thường ngày trên UI, nhưng voice đã sinh ra thì còn đó và
-- trỏ ngược về key. FK mặc định (NO ACTION) khiến mọi key từng chạy đều không
-- xoá được — giữ nguyên lịch sử voice, chỉ bỏ con trỏ tới key đã xoá.
ALTER TABLE voice
  DROP CONSTRAINT voice_ai_engine_id_fkey,
  ADD CONSTRAINT voice_ai_engine_id_fkey
    FOREIGN KEY (ai_engine_id) REFERENCES ai_engine(id) ON DELETE SET NULL;

-- Worker tra key theo chủ sở hữu, lấy key mới nhất (xem GetAIEngineForUser).
DROP INDEX IF EXISTS idx_ai_engine_created_by;
CREATE INDEX idx_ai_engine_user ON ai_engine(user_id, created_at DESC);
