DROP TABLE IF EXISTS fetch_error_stat;

ALTER TABLE list_breaking
  DROP COLUMN IF EXISTS active_weekdays,
  DROP COLUMN IF EXISTS active_to_min,
  DROP COLUMN IF EXISTS active_from_min,
  DROP COLUMN IF EXISTS timezone;

ALTER TABLE list_scheduled
  DROP COLUMN IF EXISTS fixed_times_min,
  DROP COLUMN IF EXISTS active_weekdays,
  DROP COLUMN IF EXISTS active_to_min,
  DROP COLUMN IF EXISTS active_from_min,
  DROP COLUMN IF EXISTS timezone;
