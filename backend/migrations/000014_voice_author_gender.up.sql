-- ---------------------------------------------------------------------------
-- Giới tính của tài khoản đứng tên bài đăng.
--
-- Author giờ được chọn bằng cách bốc ngẫu nhiên một tài khoản theo giới tính
-- (Male/Female/Other) chứ không tìm theo email nữa. Bảng Voice phải hiện lại
-- được "Female - solr@example.com" sau khi tải lại trang, mà giới tính thì
-- không suy ra được từ id hay email — nên lưu kèm.
-- ---------------------------------------------------------------------------

ALTER TABLE voice ADD COLUMN author_gender TEXT;
