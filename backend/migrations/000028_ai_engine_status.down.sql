ALTER TABLE ai_engine
  DROP COLUMN IF EXISTS key_status,
  DROP COLUMN IF EXISTS key_status_detail,
  DROP COLUMN IF EXISTS key_status_at;
