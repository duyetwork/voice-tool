ALTER TABLE voice          DROP COLUMN IF EXISTS llm_model_used;
ALTER TABLE voice          DROP COLUMN IF EXISTS llm_api_set_id;
ALTER TABLE list_scheduled DROP COLUMN IF EXISTS llm_api_set_id;
ALTER TABLE list_breaking  DROP COLUMN IF EXISTS llm_api_set_id;

DROP TABLE IF EXISTS app_setting;
DROP TABLE IF EXISTS llm_api_set_user;
DROP TABLE IF EXISTS llm_api_key;
DROP TABLE IF EXISTS llm_api_set;
