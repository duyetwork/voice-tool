-- ---------------------------------------------------------------------------
-- Bộ API key LLM + cấu hình chung.
--
-- Vì sao KHÔNG tái dùng ai_engine: bảng đó đã được cố tình thu về "sổ key TTS"
-- ở migration 000010 — 1 bản ghi = 1 key = 1 chủ sở hữu, không tên, không nhà
-- cung cấp. Bộ API LLM là thứ ngược lại: 1 bản ghi = NHIỀU key của NHIỀU nhà,
-- dùng chung cho NHIỀU người. Nhét cả hai vào một bảng là làm sống lại đúng
-- những cột mà 000010 vừa bỏ đi.
-- ---------------------------------------------------------------------------

-- 1 bộ = 1 túi key, có thể gồm key của nhiều nhà LLM khác nhau.
CREATE TABLE llm_api_set (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name             TEXT NOT NULL UNIQUE,
  note             TEXT,
  -- Toggle của admin: bộ này có hiện ra cho người dùng khác chọn hay không.
  visible_to_users BOOLEAN NOT NULL DEFAULT FALSE,
  created_by       UUID NOT NULL REFERENCES app_user(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Lần gần nhất một key trong bộ chạy ra nội dung. Để trên BỘ chứ không chỉ
  -- trên từng key: câu hỏi thường gặp ở màn quản lý là "bộ này còn ai dùng
  -- không", trả lời được mà không phải quét hết key con.
  last_used_at     TIMESTAMPTZ
);

CREATE TABLE llm_api_key (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  set_id               UUID NOT NULL REFERENCES llm_api_set(id) ON DELETE CASCADE,
  provider             VARCHAR(20) NOT NULL
                         CHECK (provider IN ('gemini','openai','anthropic')),
  api_key_encrypted    TEXT NOT NULL,
  -- label để phân biệt 2 key cùng nhà ("gemini cá nhân" / "gemini công ty").
  label                TEXT,
  -- Nhỏ hơn = thử trước, trong cùng 1 nhà.
  priority             INT NOT NULL DEFAULT 0,

  -- 3 cột dưới là SỨC KHOẺ KEY: router ghi, người dùng chỉ đọc.
  -- Tách disabled_at (key sai / bị thu hồi — chờ người sửa) khỏi
  -- cooldown_until (hết quota — tự hồi phục) vì hai thứ này xử lý khác nhau
  -- hoàn toàn: một cái cần người can thiệp, một cái chỉ cần chờ.
  disabled_at          TIMESTAMPTZ,
  cooldown_until       TIMESTAMPTZ,
  consecutive_failures INT NOT NULL DEFAULT 0,
  last_used_at         TIMESTAMPTZ,
  last_error           TEXT,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_llm_api_key_set ON llm_api_key(set_id, provider, priority, id);

-- 1 bộ dùng chung cho 1 hoặc nhiều người.
CREATE TABLE llm_api_set_user (
  set_id  UUID NOT NULL REFERENCES llm_api_set(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES app_user(id)   ON DELETE CASCADE,
  PRIMARY KEY (set_id, user_id)
);

CREATE INDEX idx_llm_api_set_user_user ON llm_api_set_user(user_id);

-- Cấu hình chung (chuỗi dự phòng, batch). JSONB để thêm khoá mới không cần
-- migration; chỉ admin ghi.
CREATE TABLE app_setting (
  key        TEXT PRIMARY KEY,
  value      JSONB NOT NULL,
  updated_by UUID REFERENCES app_user(id),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Bộ đã dùng, ghi lại để chạy lại / đối chiếu chi phí. ON DELETE SET NULL:
-- xoá bộ là việc thường ngày, còn voice đã sinh ra thì phải còn — đúng tiền lệ
-- voice.ai_engine_id ở migration 000010.
ALTER TABLE voice          ADD COLUMN llm_api_set_id UUID REFERENCES llm_api_set(id) ON DELETE SET NULL;
-- Mode C tự động (F2/F3) cũng gọi LLM mà không có ai bấm nút — không gán bộ
-- cho kênh thì luồng tự động không chạy được.
ALTER TABLE list_scheduled ADD COLUMN llm_api_set_id UUID REFERENCES llm_api_set(id) ON DELETE SET NULL;
ALTER TABLE list_breaking  ADD COLUMN llm_api_set_id UUID REFERENCES llm_api_set(id) ON DELETE SET NULL;

-- Model THẬT đã sinh ra lời đọc. Không suy lại được từ llm_api_set_id vì chuỗi
-- dự phòng đổi theo thời gian và theo sức khoẻ key: cùng một bộ, hôm nay chạy
-- gemini-2.5-flash-lite, mai hết quota thì chạy gpt-5.6-luna. Không lưu thì
-- không đối chiếu được chất lượng hay chi phí của một voice cụ thể.
ALTER TABLE voice ADD COLUMN llm_model_used TEXT;
