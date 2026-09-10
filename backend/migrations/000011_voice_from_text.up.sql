-- ---------------------------------------------------------------------------
-- Voice tạo thẳng từ text, không qua Bài Post.
--
-- Vì sao phá lệ "mọi Voice đều đi qua Bài Post" (specs -1): luật đó tồn tại để
-- Voice lấy từ MỘT URL luôn truy vết được về đúng bài gốc, và để chạy lại
-- Voice khác từ cùng bài mà không phải fetch URL lần nữa. Text gõ tay không có
-- URL, không có bài gốc, không có gì để fetch lại — Bài Post sinh ra chỉ là
-- một bản ghi rỗng đứng giữa, làm bẩn màn duyệt Bài Post (nơi để duyệt trước
-- khi tốn tiền AI) mà không thêm thông tin nào.
--
-- Nội dung người dùng gõ vì thế nằm thẳng trên Voice:
--   * input_text   — đoạn text TTS đọc (mode C thì đây là đầu vào của LLM)
--   * collect_mode — B (đọc nguyên văn) hoặc C (LLM viết lại rồi đọc)
--   * prompt_id    — Prompt mẫu của mode C
--
-- ck_voice_origin giữ đúng 2 dạng Voice hợp lệ, không có dạng thứ ba: hoặc từ
-- Bài Post, hoặc từ text gõ tay.
-- ---------------------------------------------------------------------------

ALTER TABLE voice
  ALTER COLUMN source_post_id DROP NOT NULL,
  ADD COLUMN input_text   TEXT,
  ADD COLUMN collect_mode CHAR(1) CHECK (collect_mode IN ('B', 'C')),
  ADD COLUMN prompt_id    UUID REFERENCES prompt(id) ON DELETE SET NULL;

ALTER TABLE voice ADD CONSTRAINT ck_voice_origin CHECK (
  (source_post_id IS NOT NULL AND input_text IS NULL AND collect_mode IS NULL)
  OR
  (source_post_id IS NULL AND input_text IS NOT NULL AND collect_mode IS NOT NULL)
);

-- Mode C bắt buộc có prompt, giống ck_source_post_prompt bên Bài Post.
ALTER TABLE voice ADD CONSTRAINT ck_voice_prompt CHECK (
  collect_mode IS DISTINCT FROM 'C' OR prompt_id IS NOT NULL
);
