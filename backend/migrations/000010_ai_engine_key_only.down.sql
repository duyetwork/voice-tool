DROP INDEX IF EXISTS idx_ai_engine_user;
CREATE INDEX idx_ai_engine_created_by ON ai_engine(created_by);

ALTER TABLE voice
  DROP CONSTRAINT voice_ai_engine_id_fkey,
  ADD CONSTRAINT voice_ai_engine_id_fkey
    FOREIGN KEY (ai_engine_id) REFERENCES ai_engine(id);

ALTER TABLE ai_engine
  ADD COLUMN name                TEXT        NOT NULL DEFAULT '',
  ADD COLUMN supported_languages TEXT[]      NOT NULL DEFAULT '{}',
  ADD COLUMN is_active           BOOLEAN     NOT NULL DEFAULT TRUE;

ALTER TABLE ai_engine ALTER COLUMN name DROP DEFAULT;

ALTER TABLE ai_engine
  ALTER COLUMN api_key_encrypted DROP NOT NULL,
  DROP COLUMN user_id,
  DROP COLUMN last_used_at;
