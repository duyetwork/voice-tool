-- ---------------------------------------------------------------------------
-- Nới hai ràng buộc CHECK đang mâu thuẫn với chính khoá ngoại của chúng.
--
-- TRIỆU CHỨNG: xoá một kênh Định kỳ (hoặc Breaking) thất bại với
--
--   ERROR: new row for relation "source_post" violates check constraint
--          "ck_source_post_origin" (SQLSTATE 23514)
--
-- và xoá một Prompt mẫu đang được voice hình thức C dùng thì thất bại với
-- "ck_voice_prompt". Cả hai là CÙNG MỘT lỗi thiết kế.
--
-- VÌ SAO: khoá ngoại khai ON DELETE SET NULL — đó là quyết định có chủ ý, "xoá
-- danh mục là việc thường ngày, còn thứ đã sinh ra thì phải còn" (xem
-- voice.ai_engine_id ở migration 000010). Nhưng CHECK lại đòi cột đó KHÁC NULL.
-- Hai điều đó không thể cùng đúng: Postgres đặt NULL theo lệnh của khoá ngoại,
-- rồi CHECK từ chối chính dòng vừa được đặt, và cả lệnh DELETE bị huỷ.
--
-- Không sửa được bằng cách đổi khoá ngoại sang CASCADE: xoá kênh sẽ kéo theo
-- mọi Bài Post và Voice của nó, mà chúng là kết quả công việc đã hoàn thành.
--
-- Nên phần phải nhường là CHECK. Cụ thể là nhường ĐÚNG phần mà SET NULL làm cho
-- không giữ nổi, giữ lại toàn bộ phần còn lại.
-- ---------------------------------------------------------------------------

-- ---------------------------------------------------------------------------
-- source_post: một bài thuộc về ĐÚNG MỘT loại kênh — hoặc không kênh nào.
--
-- Giữ nguyên hai bất biến thật sự có nghĩa:
--   1. Bài F1 (nhập tay) không thuộc kênh nào.
--   2. Không bài nào vừa thuộc kênh Breaking vừa thuộc kênh Định kỳ.
--
-- Bỏ đúng một vế: "kênh phải còn tồn tại". `source_type` vẫn nói đúng bài này
-- SINH RA TỪ ĐÂU, kể cả khi kênh đã bị xoá — đó mới là thứ cột ấy trả lời, và
-- nó không đổi khi người ta dọn dẹp danh sách kênh.
-- ---------------------------------------------------------------------------
ALTER TABLE source_post DROP CONSTRAINT ck_source_post_origin;
ALTER TABLE source_post ADD CONSTRAINT ck_source_post_origin CHECK (
  (source_type = 'F1'        AND list_breaking_id IS NULL AND list_scheduled_id IS NULL)
  OR (source_type = 'BREAKING'  AND list_scheduled_id IS NULL)
  OR (source_type = 'SCHEDULED' AND list_breaking_id IS NULL)
);

-- ---------------------------------------------------------------------------
-- voice: hình thức C phải CHỌN prompt lúc tạo, nhưng prompt có thể bị xoá sau.
--
-- CHECK này không phân biệt được "chưa từng chọn prompt" với "đã chọn, prompt
-- ấy sau này bị xoá" — cả hai đều là prompt_id NULL. Mà chỉ trường hợp đầu mới
-- là lỗi.
--
-- Bỏ hẳn CHECK chứ không nới: nới thành "cho phép NULL" thì nó không còn chặn
-- được gì, tức là một ràng buộc chỉ còn tác dụng làm người đọc tưởng có ràng
-- buộc. Vế cần giữ đã được service chặn ngay lúc tạo (domain.ErrPromptRequired,
-- xem VoiceService) — đó mới là chỗ phân biệt được hai trường hợp trên.
-- ---------------------------------------------------------------------------
ALTER TABLE voice DROP CONSTRAINT ck_voice_prompt;
