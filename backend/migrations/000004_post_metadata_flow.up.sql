-- ---------------------------------------------------------------------------
-- Nối luồng rời rạc thành 1 flow xuyên suốt (prompt.md).
--
-- 1. Bài Post giữ đủ metadata gốc (tiêu đề, mô tả, hashtag, ảnh bìa, tác giả)
--    để Voice sinh ra được auto-fill sẵn — không phải gõ tay trước khi đăng.
-- 2. Ngôn ngữ 'auto': để nền tảng/multime tự nhận diện thay vì đoán bừa.
-- ---------------------------------------------------------------------------

ALTER TABLE source_post
  ADD COLUMN title         TEXT,
  ADD COLUMN description   TEXT,
  ADD COLUMN hashtags      TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN thumbnail_url TEXT,
  ADD COLUMN author_name   TEXT,
  -- Ngày đăng của bài GỐC trên nền tảng (khác created_at = ngày vào hệ thống).
  ADD COLUMN posted_at     TIMESTAMPTZ;

-- Bộ lọc mới ở các bảng: nền tảng, người tạo, ngày tạo, ngày đăng.
CREATE INDEX idx_source_post_platform   ON source_post(platform);
CREATE INDEX idx_source_post_created_by ON source_post(created_by);
CREATE INDEX idx_voice_created_by       ON voice(created_by);
CREATE INDEX idx_voice_created_at       ON voice(created_at DESC);
CREATE INDEX idx_voice_published_at     ON voice(published_at DESC);
