-- Voice sinh từ Bài Post không được mang text riêng nữa -> bỏ phần đã sửa tay.
UPDATE voice
SET input_text = NULL, collect_mode = NULL, prompt_id = NULL
WHERE source_post_id IS NOT NULL;

ALTER TABLE voice DROP CONSTRAINT ck_voice_origin;

ALTER TABLE voice ADD CONSTRAINT ck_voice_origin CHECK (
  (source_post_id IS NOT NULL AND input_text IS NULL AND collect_mode IS NULL)
  OR
  (source_post_id IS NULL AND input_text IS NOT NULL AND collect_mode IS NOT NULL)
);
