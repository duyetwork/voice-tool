-- ---------------------------------------------------------------------------
-- Đăng ngay sau khi tạo xong, và "bài này không có ảnh".
--
-- Màn tạo Voice giờ điền metadata TRƯỚC khi worker chạy: người dùng bấm "Đăng"
-- là đóng hộp thoại, worker tạo audio xong thì tự đăng lên multime. Hai thứ đó
-- phải đi theo bản ghi voice chứ không giữ trong bộ nhớ của trình duyệt:
--
--   publish_when_ready  worker tạo xong thì tự đưa vào hàng đợi đăng.
--   no_image            người dùng chủ động chọn "không có ảnh". Khác với
--                       image_url IS NULL (chưa biết), vì khi chưa biết thì
--                       worker điền ảnh bìa của bài gốc vào.
-- ---------------------------------------------------------------------------

ALTER TABLE voice
  ADD COLUMN publish_when_ready BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN no_image           BOOLEAN NOT NULL DEFAULT FALSE;
