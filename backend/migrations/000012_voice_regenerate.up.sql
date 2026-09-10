-- ---------------------------------------------------------------------------
-- Sửa nội dung rồi tạo lại chính Voice đó.
--
-- Trước migration này `input_text` chỉ tồn tại trên Voice gõ tay, và
-- ck_voice_origin cấm Voice-từ-Bài-Post mang text riêng. Nhưng khi người dùng
-- nghe thử rồi muốn sửa lại lời đọc (bài gốc viết lủng củng, tên riêng đọc
-- sai, muốn cắt ngắn), thứ họ sửa là NỘI DUNG SẼ ĐỌC chứ không phải bài gốc —
-- bài gốc trên Facebook/YouTube là dữ liệu của người khác, không được đụng vào.
--
-- Nên `input_text` đổi nghĩa thành: "lời đọc do người dùng chốt". Có nó thì
-- worker đọc đúng nó và không fetch lại nguồn; không có thì Voice sinh ra từ
-- Bài Post như cũ.
--
-- ck_voice_origin nới theo: mỗi Voice vẫn phải neo vào MỘT trong hai gốc —
-- Bài Post, hoặc text người dùng tự gõ (kèm hình thức đọc).
-- ---------------------------------------------------------------------------

ALTER TABLE voice DROP CONSTRAINT ck_voice_origin;

ALTER TABLE voice ADD CONSTRAINT ck_voice_origin CHECK (
  source_post_id IS NOT NULL
  OR (input_text IS NOT NULL AND collect_mode IS NOT NULL)
);
