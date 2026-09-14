-- ---------------------------------------------------------------------------
-- Lời đọc THẬT của voice, và chỉ mục cho phần thống kê theo kênh.
--
-- Vấn đề: hình thức C đi qua LLM (text nguồn + prompt -> text mới -> TTS),
-- nhưng chỉ có file audio giữ lại kết quả. `input_text` vẫn là đoạn người dùng
-- gõ, `title` vẫn dựng từ đoạn đó — nên giao diện hiển thị đúng cái KHÔNG được
-- đọc, và người dùng kết luận là LLM không chạy. Nó có chạy; chỉ là không có
-- chỗ nào lưu kết quả.
--
-- Tách cột riêng chứ không ghi đè `input_text`: `input_text` là ĐẦU VÀO của
-- prompt. Ghi đè nó nghĩa là lần "tạo lại" kế tiếp sẽ đưa bản đã viết lại vào
-- prompt lần nữa — mỗi lần chạy lại là một lần tam sao thất bản, và không có
-- đường nào lấy lại đoạn gốc.
--
-- Mode B cũng ghi cột này (spoken_text = input_text sau chuẩn hoá): "đoạn TTS
-- đã đọc" phải trả lời được cho mọi voice, không chỉ mode C.
-- ---------------------------------------------------------------------------

ALTER TABLE voice ADD COLUMN spoken_text TEXT;

-- Bảng Danh sách kênh đếm số Bài Post và số Voice của từng kênh trên mỗi lần
-- tải trang. Không có chỉ mục thì mỗi ô đếm là một lần quét toàn bảng
-- source_post.
CREATE INDEX idx_source_post_list_scheduled ON source_post(list_scheduled_id)
  WHERE list_scheduled_id IS NOT NULL;
CREATE INDEX idx_source_post_list_breaking  ON source_post(list_breaking_id)
  WHERE list_breaking_id IS NOT NULL;
