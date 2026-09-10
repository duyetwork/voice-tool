DROP INDEX IF EXISTS uq_source_post_dedup;
DROP INDEX IF EXISTS idx_source_post_dedup_lookup;

CREATE UNIQUE INDEX uq_source_post_scheduled_dedup
  ON source_post(list_scheduled_id, post_id_extracted)
  WHERE list_scheduled_id IS NOT NULL AND post_id_extracted IS NOT NULL;
CREATE UNIQUE INDEX uq_source_post_breaking_dedup
  ON source_post(list_breaking_id, post_id_extracted)
  WHERE list_breaking_id IS NOT NULL AND post_id_extracted IS NOT NULL;
