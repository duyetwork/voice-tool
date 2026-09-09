-- name: UpsertUserFromSSO :one
-- Đăng nhập bằng strongbody: tạo user nếu chưa có, cập nhật thông tin + token
-- nếu đã có. Role của lần tạo đầu do $5 quyết định, các lần sau KHÔNG ghi đè
-- (admin đã cấp quyền thì giữ nguyên).
INSERT INTO app_user (
  strongbody_user_id, email, full_name, avatar_url, role,
  multime_access_token, multime_refresh_token, multime_token_at, last_login_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, now(), now()
)
ON CONFLICT (email) DO UPDATE SET
  strongbody_user_id    = EXCLUDED.strongbody_user_id,
  full_name             = COALESCE(EXCLUDED.full_name, app_user.full_name),
  avatar_url            = COALESCE(EXCLUDED.avatar_url, app_user.avatar_url),
  multime_access_token  = EXCLUDED.multime_access_token,
  multime_refresh_token = EXCLUDED.multime_refresh_token,
  multime_token_at      = now(),
  last_login_at         = now(),
  is_active             = TRUE
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM app_user WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM app_user WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM app_user ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: SetUserRole :one
UPDATE app_user SET role = $2 WHERE id = $1 RETURNING *;

-- name: SetUserActive :one
UPDATE app_user SET is_active = $2 WHERE id = $1 RETURNING *;

-- name: CountUsers :one
SELECT COUNT(*) FROM app_user;

-- name: CountAdmins :one
SELECT COUNT(*) FROM app_user WHERE role = 'admin' AND is_active = TRUE;

-- name: SetUserMultimeToken :exec
-- Cập nhật access token sau khi refresh với strongbody.
UPDATE app_user
SET multime_access_token = $2, multime_token_at = now()
WHERE id = $1;

-- name: ClearUserMultimeToken :exec
-- Token hết hiệu lực và refresh cũng thất bại -> buộc user đăng nhập lại.
UPDATE app_user
SET multime_access_token = NULL, multime_refresh_token = NULL
WHERE id = $1;
