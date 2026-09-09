DROP INDEX IF EXISTS idx_app_user_role;

ALTER TABLE voice
  DROP COLUMN IF EXISTS sample_rate,
  DROP COLUMN IF EXISTS size_bytes,
  DROP COLUMN IF EXISTS mime_type,
  DROP COLUMN IF EXISTS title;

ALTER TABLE app_user DROP COLUMN IF EXISTS role;

ALTER TABLE list_scheduled
  DROP COLUMN IF EXISTS last_scanned_at,
  DROP COLUMN IF EXISTS max_posts_per_run,
  DROP COLUMN IF EXISTS scan_limit;

ALTER TABLE list_breaking
  DROP COLUMN IF EXISTS last_scanned_at,
  DROP COLUMN IF EXISTS scan_interval,
  DROP COLUMN IF EXISTS scan_limit;

ALTER TABLE list_breaking DROP CONSTRAINT IF EXISTS ck_list_breaking_patterns;
ALTER TABLE list_breaking ADD COLUMN regex_pattern TEXT NOT NULL DEFAULT '';
UPDATE list_breaking SET regex_pattern = COALESCE(regex_patterns[1], '');
ALTER TABLE list_breaking ALTER COLUMN regex_pattern DROP DEFAULT;
ALTER TABLE list_breaking DROP COLUMN regex_patterns;
