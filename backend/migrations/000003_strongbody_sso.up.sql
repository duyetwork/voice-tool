-- ---------------------------------------------------------------------------
-- Chuyển sang đăng nhập bằng tài khoản strongbody/multime (SSO).
--
-- Hệ thống không còn tự quản lý mật khẩu: app_user trở thành bản chiếu của user
-- bên strongbody. Tài khoản đăng nhập cũng chính là tài khoản đăng voice, nên
-- phải giữ token của họ để worker publish thay họ.
-- ---------------------------------------------------------------------------

ALTER TABLE app_user
  -- id của user bên strongbody, đồng thời là author_id khi đăng voice.
  ADD COLUMN strongbody_user_id BIGINT,
  ADD COLUMN avatar_url         TEXT,
  -- Token của strongbody, mã hoá AES-GCM bằng TOKEN_ENCRYPTION_KEY.
  ADD COLUMN multime_access_token  TEXT,
  ADD COLUMN multime_refresh_token TEXT,
  ADD COLUMN multime_token_at      TIMESTAMPTZ,
  ADD COLUMN last_login_at         TIMESTAMPTZ;

-- Không còn xác thực cục bộ -> không lưu mật khẩu nữa.
ALTER TABLE app_user ALTER COLUMN password_hash DROP NOT NULL;
UPDATE app_user SET password_hash = NULL;

-- Các bản ghi cũ (nếu có) chưa có strongbody_user_id -> vô hiệu hoá, người dùng
-- đăng nhập lại bằng SSO là khớp lại theo email.
UPDATE app_user SET is_active = FALSE WHERE strongbody_user_id IS NULL;

CREATE UNIQUE INDEX uq_app_user_strongbody_id
  ON app_user(strongbody_user_id)
  WHERE strongbody_user_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Role: editor -> user.
--   viewer : chỉ xem
--   user   : xem tất cả + tạo/chạy/đăng voice, KHÔNG được xoá
--   admin  : toàn quyền + cấp quyền cho người khác
-- ---------------------------------------------------------------------------
ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_role_check;
UPDATE app_user SET role = 'user' WHERE role = 'editor';
ALTER TABLE app_user
  ADD CONSTRAINT app_user_role_check CHECK (role IN ('admin', 'user', 'viewer'));

ALTER TABLE app_user ALTER COLUMN role SET DEFAULT 'user';
