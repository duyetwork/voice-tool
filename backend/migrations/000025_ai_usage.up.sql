-- ---------------------------------------------------------------------------
-- Đo lượng dùng AI — để "tối ưu chi phí" thôi đoán mò.
--
-- Màn Cài đặt đã cho đổi chuỗi dự phòng LLM và bật batch, nhưng không có con số
-- nào nói đổi xong rẻ hơn hay đắt hơn. Bảng này ghi MỖI lần gọi nhà cung cấp
-- AI: bao nhiêu token vào/ra (LLM), bao nhiêu ký tự và bao nhiêu giây audio
-- (TTS/STT).
--
-- KHÔNG có cột tiền. Đơn giá nằm ở app_setting (admin tự khai theo hoá đơn của
-- mình) và tiền được tính lúc ĐỌC. Lưu tiền vào từng dòng nghĩa là khai sai một
-- con số thì mọi dòng đã ghi sai vĩnh viễn; tính lúc đọc thì sửa đơn giá là
-- bảng thống kê đúng lại ngay.
--
-- Ghi cả lần THẤT BẠI (ok = false): một model liên tục lỗi vẫn đốt token đầu
-- vào, và tỉ lệ lỗi theo model là nửa còn lại của câu hỏi "model nào đáng tiền".
--
-- Dòng ở đây là dữ liệu vận hành, không phải sổ sách — job dọn dẹp xoá theo
-- AI_USAGE_RETENTION (mặc định 90 ngày).
-- ---------------------------------------------------------------------------

CREATE TABLE ai_usage (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  kind          VARCHAR(10) NOT NULL CHECK (kind IN ('llm','tts','stt')),
  -- provider: 'anthropic' | 'openai' | 'gemini' | '3voices' | 'whisper' ...
  provider      TEXT        NOT NULL,
  -- model: ID model của LLM, hoặc voice_id của TTS. Rỗng khi nhà không có khái
  -- niệm đó (mock).
  model         TEXT        NOT NULL DEFAULT '',
  -- Ai chịu chi phí này. NULL khi lần gọi không gắn với người nào (job nền).
  user_id       UUID        REFERENCES app_user(id) ON DELETE SET NULL,
  input_tokens  BIGINT      NOT NULL DEFAULT 0,
  output_tokens BIGINT      NOT NULL DEFAULT 0,
  -- characters: TTS tính tiền theo ký tự chứ không theo token.
  characters    BIGINT      NOT NULL DEFAULT 0,
  -- audio_seconds: độ dài audio sinh ra (TTS) hoặc đưa vào (STT).
  audio_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
  ok            BOOLEAN     NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_ai_usage_at ON ai_usage(at DESC);
