-- ---------------------------------------------------------------------------
-- Danh mục Quốc gia + Hashtag lưu trong DB, để modal Tạo Voice không phải hỏi
-- API mỗi lần mở.
--
-- Quốc gia: trước đây mỗi lần mở modal là một lần gọi sang Strongbody. Danh mục
-- này đổi vài năm một lần, nên hỏi lại liên tục vừa chậm vừa làm modal phụ
-- thuộc vào việc token Strongbody còn hạn hay không. Đồng bộ 1 lần rồi đọc từ
-- đây; `synced_at` quyết định khi nào hỏi lại.
--
-- `sort_order` tồn tại vì thứ tự hiển thị là THỨ TỰ NGHIỆP VỤ (Việt Nam trước,
-- rồi các thị trường chính), không phải alphabet và cũng không phải thứ tự
-- Strongbody trả về. Để frontend tự sắp thì thứ tự đó thành một bản sao thứ hai
-- của cùng một quy tắc.
-- ---------------------------------------------------------------------------

CREATE TABLE country (
  -- id của Strongbody, dùng thẳng làm khoá chính: đây là bản sao có chủ, không
  -- phải thực thể riêng của tool.
  id         BIGINT PRIMARY KEY,
  name       TEXT   NOT NULL,
  code       TEXT,
  sort_order INT    NOT NULL DEFAULT 9999,
  synced_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_country_order ON country(sort_order, name);

-- ---------------------------------------------------------------------------
-- Hashtag: KHÔNG có API danh mục hashtag nào bên Strongbody (client hiện tại
-- chỉ gửi hashtags lúc đăng bài, không đọc về). Nên nguồn duy nhất có thật là
-- chính dữ liệu hệ thống đã tích: hashtag của Bài Post đã lấy về và hashtag của
-- Voice đã tạo.
--
-- Quan hệ hashtag ↔ ngôn ngữ cũng lấy từ đó chứ không bịa ra: mỗi Voice đã có
-- sẵn cả hashtag lẫn ngôn ngữ, nên cặp (tag, language) là quan hệ QUAN SÁT ĐƯỢC.
-- Ngôn ngữ rỗng = tag đến từ nguồn chưa biết ngôn ngữ.
-- ---------------------------------------------------------------------------

CREATE TABLE hashtag (
  tag          TEXT NOT NULL,
  language     TEXT NOT NULL DEFAULT '',
  usage_count  BIGINT NOT NULL DEFAULT 0,
  -- last_used_at là mốc của DỮ LIỆU (voice mới nhất mang tag này);
  -- refreshed_at là mốc của LẦN DỰNG LẠI. Hai thứ khác nhau, và lấy nhầm cái
  -- đầu làm TTL thì hệ thống dựng lại danh mục ở mỗi lần mở modal.
  last_used_at TIMESTAMPTZ,
  refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tag, language)
);

CREATE INDEX idx_hashtag_lang ON hashtag(language, usage_count DESC);

-- Quốc gia đã chọn khi tạo voice. Cần lưu vì việc BỐC tài khoản author giờ lùi
-- tới lúc bấm Đăng: lúc đó không còn form nào để hỏi lại người dùng đã lọc theo
-- quốc gia nào.
ALTER TABLE voice ADD COLUMN author_country_id BIGINT;
