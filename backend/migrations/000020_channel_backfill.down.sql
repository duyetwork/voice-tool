ALTER TABLE list_breaking
  DROP COLUMN IF EXISTS max_posts_per_run,
  DROP COLUMN IF EXISTS backfill_excluded_ids,
  DROP COLUMN IF EXISTS backfill_done_at,
  DROP COLUMN IF EXISTS backfill_limit;

ALTER TABLE list_scheduled
  DROP COLUMN IF EXISTS backfill_done_at,
  DROP COLUMN IF EXISTS backfill_limit;
