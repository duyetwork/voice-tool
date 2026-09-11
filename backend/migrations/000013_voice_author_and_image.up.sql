-- ---------------------------------------------------------------------------
-- Author của bài đăng + ảnh bìa tải từ máy.
--
-- 1) author_id: trước đây bài luôn đăng dưới tài khoản của người tạo Voice
--    (creds.AuthorID). Nay người đăng phải CHỌN tác giả trong danh sách user
--    bên Strongbody — voice của một biên tập viên có thể phải lên dưới tên một
--    kênh/tài khoản khác. Lưu kèm author_email để bảng Voice hiện được email mà
--    không phải gọi lại API Strongbody cho từng dòng (email cũng là thứ duy
--    nhất người dùng nhận ra, id chỉ là con số).
--
-- 2) image_uploaded: ảnh bìa có 2 nguồn — URL của bài gốc (ảnh của người khác,
--    không nằm trong storage của mình) và ảnh người dùng tải lên từ máy (nằm
--    trong bucket của mình). Chỉ ảnh loại sau mới được xoá sau khi đăng, nên
--    phải phân biệt được bằng một cột chứ không đoán từ hình dạng URL.
-- ---------------------------------------------------------------------------

ALTER TABLE voice
  ADD COLUMN author_id      BIGINT,
  ADD COLUMN author_email   TEXT,
  ADD COLUMN image_uploaded BOOLEAN NOT NULL DEFAULT FALSE;
