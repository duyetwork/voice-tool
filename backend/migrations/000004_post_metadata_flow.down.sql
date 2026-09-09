DROP INDEX IF EXISTS idx_voice_published_at;
DROP INDEX IF EXISTS idx_voice_created_at;
DROP INDEX IF EXISTS idx_voice_created_by;
DROP INDEX IF EXISTS idx_source_post_created_by;
DROP INDEX IF EXISTS idx_source_post_platform;

ALTER TABLE source_post
  DROP COLUMN IF EXISTS posted_at,
  DROP COLUMN IF EXISTS author_name,
  DROP COLUMN IF EXISTS thumbnail_url,
  DROP COLUMN IF EXISTS hashtags,
  DROP COLUMN IF EXISTS description,
  DROP COLUMN IF EXISTS title;
