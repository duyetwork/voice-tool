-- Khôi phục hai ràng buộc cũ.
--
-- NOT VALID: dữ liệu hiện có có thể đã chứa đúng những dòng mà bản cũ từ chối
-- (bài của một kênh đã xoá, voice của một prompt đã xoá). Kiểm tra lại toàn bảng
-- sẽ làm lệnh này hỏng, và một migration down không chạy được thì không phải
-- đường lùi. NOT VALID áp luật cũ cho dòng MỚI và tha cho dòng đã có.
ALTER TABLE voice ADD CONSTRAINT ck_voice_prompt
  CHECK (collect_mode IS DISTINCT FROM 'C' OR prompt_id IS NOT NULL) NOT VALID;

ALTER TABLE source_post DROP CONSTRAINT ck_source_post_origin;
ALTER TABLE source_post ADD CONSTRAINT ck_source_post_origin CHECK (
  (source_type = 'F1'        AND list_breaking_id IS NULL AND list_scheduled_id IS NULL)
  OR (source_type = 'BREAKING'  AND list_breaking_id  IS NOT NULL AND list_scheduled_id IS NULL)
  OR (source_type = 'SCHEDULED' AND list_breaking_id  IS NULL AND list_scheduled_id IS NOT NULL)
) NOT VALID;
