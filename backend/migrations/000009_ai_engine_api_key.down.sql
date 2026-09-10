DROP INDEX IF EXISTS idx_ai_engine_created_by;

ALTER TABLE ai_engine
  DROP CONSTRAINT IF EXISTS ck_ai_engine_provider,
  ALTER COLUMN provider DROP DEFAULT;

ALTER TABLE ai_engine
  DROP COLUMN IF EXISTS api_key_encrypted,
  DROP COLUMN IF EXISTS voice_id,
  DROP COLUMN IF EXISTS created_by;
