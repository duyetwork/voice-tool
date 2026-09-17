DROP INDEX IF EXISTS idx_scrape_proxy_owner;
DROP INDEX IF EXISTS idx_scrape_via_owner;

ALTER TABLE scrape_proxy DROP COLUMN IF EXISTS user_id;
ALTER TABLE scrape_via   DROP COLUMN IF EXISTS user_id;
