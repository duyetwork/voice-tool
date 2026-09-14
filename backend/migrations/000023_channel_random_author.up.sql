-- ---------------------------------------------------------------------------
-- Kênh tự chọn tài khoản đứng tên bài đăng, và bỏ "giờ chạy cố định".
--
-- 1. random_author — vá một lỗ hổng thật, không phải thêm tuỳ chọn cho vui.
--
-- Voice sinh ra từ kênh không có ai ngồi chọn author, nên `author_gender` của
-- chúng luôn NULL. Đến bước đăng, Engine.ensureAuthor gặp voice không có
-- author_id lẫn author_gender và trả lỗi vĩnh viễn "chưa chọn giới tính tài
-- khoản đứng tên bài đăng". Nghĩa là auto_publish của kênh CHƯA BAO GIỜ đăng
-- được bài nào — nó chỉ tạo ra voice hỏng ở bước cuối.
--
-- Bật cờ này thì mỗi voice của kênh được bốc một tài khoản ngẫu nhiên, lọc theo
-- quốc gia suy ra từ ngôn ngữ của kênh (xem domain.CountriesForLanguage).
--
-- 2. auto_publish không còn ô riêng trên giao diện: "tạo voice tự động" mà
-- không đăng thì bài nằm lại ở trạng thái nháp và vẫn phải vào bấm tay từng
-- cái — tức là không tự động. Cột vẫn giữ vì nó là thứ worker đọc; API giờ
-- gán auto_publish = auto_process.
--
-- 3. fixed_times_min bị bỏ khỏi form. Xoá luôn dữ liệu cũ: để lại giá trị mà
-- không còn ô nào sửa nghĩa là kênh chạy theo một lịch người dùng không nhìn
-- thấy và không gỡ được.
-- ---------------------------------------------------------------------------

ALTER TABLE list_scheduled ADD COLUMN random_author BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE list_breaking  ADD COLUMN random_author BOOLEAN NOT NULL DEFAULT FALSE;

-- Kênh đang bật auto_publish là kênh đã nói rõ "tôi muốn bài tự lên multime" —
-- bật sẵn random_author cho chúng, nếu không thì sau migration này chúng vẫn
-- hỏng ở đúng chỗ cũ.
UPDATE list_scheduled SET random_author = TRUE WHERE auto_publish;
UPDATE list_breaking  SET random_author = TRUE WHERE auto_publish;

UPDATE list_scheduled SET fixed_times_min = NULL WHERE fixed_times_min IS NOT NULL;
