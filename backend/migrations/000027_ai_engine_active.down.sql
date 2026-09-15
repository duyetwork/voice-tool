DROP INDEX IF EXISTS uq_ai_engine_active_per_user;
ALTER TABLE ai_engine DROP COLUMN IF EXISTS is_active;
