-- Voice gõ tay không có Bài Post để trỏ về nên không giữ lại được khi quay đầu.
DELETE FROM voice WHERE source_post_id IS NULL;

ALTER TABLE voice
  DROP CONSTRAINT IF EXISTS ck_voice_prompt,
  DROP CONSTRAINT IF EXISTS ck_voice_origin;

ALTER TABLE voice
  DROP COLUMN IF EXISTS input_text,
  DROP COLUMN IF EXISTS collect_mode,
  DROP COLUMN IF EXISTS prompt_id;

ALTER TABLE voice ALTER COLUMN source_post_id SET NOT NULL;
