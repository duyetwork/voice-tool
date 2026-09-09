ALTER TABLE app_user ALTER COLUMN role SET DEFAULT 'viewer';

ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_role_check;
UPDATE app_user SET role = 'editor' WHERE role = 'user';
ALTER TABLE app_user
  ADD CONSTRAINT app_user_role_check CHECK (role IN ('admin', 'editor', 'viewer'));

DROP INDEX IF EXISTS uq_app_user_strongbody_id;

-- password_hash không thể phục hồi (đã bị xoá) -> đặt placeholder để giữ NOT NULL.
UPDATE app_user SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE app_user ALTER COLUMN password_hash SET NOT NULL;

ALTER TABLE app_user
  DROP COLUMN IF EXISTS last_login_at,
  DROP COLUMN IF EXISTS multime_token_at,
  DROP COLUMN IF EXISTS multime_refresh_token,
  DROP COLUMN IF EXISTS multime_access_token,
  DROP COLUMN IF EXISTS avatar_url,
  DROP COLUMN IF EXISTS strongbody_user_id;
