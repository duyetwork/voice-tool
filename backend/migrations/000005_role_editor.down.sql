ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_role_check;

-- `editor` cũ không có bản tương ứng ở sơ đồ trước, hạ về `user`.
UPDATE app_user SET role = 'user' WHERE role = 'editor';

ALTER TABLE app_user
  ALTER COLUMN role SET DEFAULT 'user',
  ADD CONSTRAINT app_user_role_check CHECK (role IN ('admin', 'user', 'viewer'));
