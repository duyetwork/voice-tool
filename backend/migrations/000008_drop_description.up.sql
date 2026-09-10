-- ---------------------------------------------------------------------------
-- Bỏ trường mô tả khỏi Bài Post và Voice.
--
-- Lý do: nơi đăng không dùng tới nó. Form đăng voice của multime luôn gửi
-- `caption` rỗng và giao diện chỉ hiển thị `title`, nên mô tả chỉ là dữ liệu
-- chết — vừa tốn công điền/sửa, vừa dễ hiểu nhầm là sẽ hiện trên bài đăng.
--
-- Thay vào đó tiêu đề gánh toàn bộ phần chữ:
--   * Tiêu đề Bài Post = TOÀN BỘ nội dung bài, trừ hashtag.
--   * Tiêu đề Voice    = tiêu đề Bài Post, gộp 1 dòng và cắt 200 ký tự — giới
--                        hạn của ô tiêu đề bên multime.
--
-- Dữ liệu cũ: bản ghi nào chưa có tiêu đề thì lấy mô tả làm tiêu đề trước khi
-- xoá cột, để không mất trắng phần chữ. Bản ghi đã có tiêu đề thì giữ nguyên
-- tiêu đề cũ (ngắn hơn định nghĩa mới); chạy lại bài đó sẽ lấy nội dung đầy đủ.
-- ---------------------------------------------------------------------------

UPDATE source_post
SET title = left(btrim(description), 5000)
WHERE title IS NULL AND btrim(coalesce(description, '')) <> '';

-- Tiêu đề Voice là 1 dòng, tối đa 200 ký tự.
UPDATE voice
SET title = left(regexp_replace(btrim(description), '\s+', ' ', 'g'), 200)
WHERE title IS NULL AND btrim(coalesce(description, '')) <> '';

ALTER TABLE source_post DROP COLUMN IF EXISTS description;
ALTER TABLE voice       DROP COLUMN IF EXISTS description;
