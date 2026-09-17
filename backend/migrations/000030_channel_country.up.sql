-- ---------------------------------------------------------------------------
-- Quốc gia của kênh, lan xuống mọi Bài Post và Voice do kênh đó đẻ ra.
--
-- Đang có: quốc gia chỉ được SUY RA từ ngôn ngữ của kênh, lúc bốc tài khoản
-- đứng tên bài (Scan.countryForLanguage). Suy ra thì hỏng ở đúng những chỗ hay
-- gặp nhất: tiếng Anh ra cả chục nước, tiếng Việt thì đúng nhưng kênh nói
-- tiếng Việt cho thị trường khác thì sai, và ngôn ngữ 'auto' không suy ra được
-- gì cả — lúc đó tài khoản được bốc không lọc nước, tức là bốc bừa.
--
-- Đặt thẳng lên kênh biến nó thành một quyết định người dùng chốt một lần lúc
-- thêm kênh, và mọi bài của kênh thừa hưởng. NULL = giữ nguyên hành vi cũ
-- (suy từ ngôn ngữ), nên kênh đang chạy không đổi gì.
--
-- Vì sao source_post cũng có cột này chứ không chỉ đọc ngược lên kênh: bài F1
-- nhập tay không thuộc kênh nào, và kênh có thể bị xoá (list_*_id ON DELETE
-- SET NULL) trong khi bài vẫn còn. Quốc gia là thuộc tính của BÀI kể từ lúc nó
-- được tạo — chốt tại đó thì một lần sửa kênh về sau không viết lại lịch sử.
--
-- BIGINT chứ không FK kiểu chuỗi: country.id là id của Strongbody, bảng country
-- chỉ là bản sao có chủ của danh mục bên đó.
-- ---------------------------------------------------------------------------

ALTER TABLE list_scheduled ADD COLUMN country_id BIGINT REFERENCES country(id);
ALTER TABLE list_breaking  ADD COLUMN country_id BIGINT REFERENCES country(id);
ALTER TABLE source_post    ADD COLUMN country_id BIGINT REFERENCES country(id);
