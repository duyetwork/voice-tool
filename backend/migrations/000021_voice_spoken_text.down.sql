DROP INDEX IF EXISTS idx_source_post_list_breaking;
DROP INDEX IF EXISTS idx_source_post_list_scheduled;
ALTER TABLE voice DROP COLUMN IF EXISTS spoken_text;
