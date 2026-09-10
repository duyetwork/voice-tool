-- ---------------------------------------------------------------------------
-- Phân quyền: bỏ `viewer`, thêm `editor`.
--
--   admin  : toàn quyền, kể cả phân quyền
--   editor : toàn quyền nghiệp vụ (kể cả xoá), TRỪ phân quyền
--   user   : tạo/chạy/đăng voice, KHÔNG được xoá
--
-- Bỏ vai trò chỉ-xem: hệ thống không có đăng ký, đăng nhập được bằng tài khoản
-- multime nghĩa là dùng được — nên `viewer` cũ chuyển thành `user`.
-- ---------------------------------------------------------------------------

ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_role_check;

UPDATE app_user SET role = 'user' WHERE role = 'viewer';

ALTER TABLE app_user
  ALTER COLUMN role SET DEFAULT 'user',
  ADD CONSTRAINT app_user_role_check CHECK (role IN ('admin', 'editor', 'user'));
