-- name: CreateLLMAPISet :one
INSERT INTO llm_api_set (name, note, visible_to_users, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLLMAPISet :one
SELECT s.*, u.email AS created_by_email
FROM llm_api_set s
JOIN app_user u ON u.id = s.created_by
WHERE s.id = $1;

-- name: ListLLMAPISets :many
-- `viewer` NULL = xem tất cả (admin). Người dùng thường được service truyền
-- chính id của họ vào, và chỉ thấy bộ mình tạo / được chia sẻ / admin đã bật
-- hiển thị — chặn ở SQL chứ không chỉ ẩn trên UI.
SELECT s.*, u.email AS created_by_email
FROM llm_api_set s
JOIN app_user u ON u.id = s.created_by
WHERE sqlc.narg('viewer')::uuid IS NULL
   OR s.visible_to_users
   OR s.created_by = sqlc.narg('viewer')
   OR EXISTS (
        SELECT 1 FROM llm_api_set_user su
        WHERE su.set_id = s.id AND su.user_id = sqlc.narg('viewer')
      )
ORDER BY s.name;

-- name: UpdateLLMAPISet :one
UPDATE llm_api_set
SET name             = COALESCE(sqlc.narg('name'), name),
    note             = COALESCE(sqlc.narg('note'), note),
    visible_to_users = COALESCE(sqlc.narg('visible_to_users'), visible_to_users)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteLLMAPISet :execrows
DELETE FROM llm_api_set WHERE id = $1;

-- name: TouchLLMAPISetUsed :exec
UPDATE llm_api_set SET last_used_at = now() WHERE id = $1;

-- ---------------------------------------------------------------------------
-- Key trong bộ
-- ---------------------------------------------------------------------------

-- name: CreateLLMAPIKey :one
INSERT INTO llm_api_key (set_id, provider, api_key_encrypted, label, priority)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetLLMAPIKey :one
SELECT * FROM llm_api_key WHERE id = $1;

-- name: ListLLMAPIKeys :many
-- Thứ tự ở đây chính là thứ tự router thử key trong cùng 1 nhà: priority nhỏ
-- trước, hoà thì theo id cho TẤT ĐỊNH — router không được xoay ngẫu nhiên.
SELECT * FROM llm_api_key
WHERE set_id = $1
ORDER BY provider, priority, id;

-- name: CountLLMAPIKeysBySet :many
-- Bảng danh sách bộ hiện "số key theo từng nhà" mà không phải tải hết key về.
SELECT set_id, provider, COUNT(*)::bigint AS total
FROM llm_api_key
WHERE set_id = ANY(sqlc.arg('set_ids')::uuid[])
GROUP BY set_id, provider;

-- name: UpdateLLMAPIKey :one
-- api_key_encrypted dùng COALESCE: bỏ trống ô API key ở form nghĩa là "giữ key
-- cũ" — key thật không bao giờ gửi về trình duyệt nên không có gì để gửi lại.
--
-- Sửa key cũng là RESET SỨC KHOẺ: người dùng vào đây vì key hỏng, dán key mới
-- mà vẫn còn disabled_at thì router tiếp tục bỏ qua nó và họ không hiểu vì sao.
UPDATE llm_api_key
SET provider          = COALESCE(sqlc.narg('provider'), provider),
    api_key_encrypted = COALESCE(sqlc.narg('api_key_encrypted'), api_key_encrypted),
    label             = COALESCE(sqlc.narg('label'), label),
    priority          = COALESCE(sqlc.narg('priority'), priority),
    disabled_at          = CASE WHEN sqlc.narg('api_key_encrypted') IS NULL THEN disabled_at          ELSE NULL END,
    cooldown_until       = CASE WHEN sqlc.narg('api_key_encrypted') IS NULL THEN cooldown_until       ELSE NULL END,
    consecutive_failures = CASE WHEN sqlc.narg('api_key_encrypted') IS NULL THEN consecutive_failures ELSE 0    END,
    last_error           = CASE WHEN sqlc.narg('api_key_encrypted') IS NULL THEN last_error           ELSE NULL END
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteLLMAPIKey :execrows
DELETE FROM llm_api_key WHERE id = $1;

-- name: MarkLLMAPIKeyOK :exec
-- Gọi sau mỗi lần gọi LLM THÀNH CÔNG: xoá sạch dấu vết hỏng hóc cũ. Không reset
-- consecutive_failures ở đây thì một key thỉnh thoảng lỗi sẽ tích dần số đếm
-- qua nhiều ngày rồi bị tắt oan.
UPDATE llm_api_key
SET last_used_at         = now(),
    consecutive_failures = 0,
    last_error           = NULL,
    cooldown_until       = NULL
WHERE id = $1;

-- name: MarkLLMAPIKeyCooldown :exec
-- Hết quota / bị rate-limit: key vẫn đúng, chỉ cần CHỜ. Không đụng disabled_at.
UPDATE llm_api_key
SET cooldown_until       = sqlc.arg('cooldown_until'),
    consecutive_failures = consecutive_failures + 1,
    last_error           = sqlc.narg('last_error')
WHERE id = sqlc.arg('id');

-- name: MarkLLMAPIKeyDisabled :exec
-- Key sai / bị thu hồi: chờ bao lâu cũng không tự khỏi, phải có người dán key
-- mới. Tách khỏi cooldown vì hai tình huống này xử lý khác hẳn nhau.
UPDATE llm_api_key
SET disabled_at          = now(),
    consecutive_failures = consecutive_failures + 1,
    last_error           = sqlc.narg('last_error')
WHERE id = sqlc.arg('id');

-- name: MarkLLMAPIKeyFailure :exec
-- Lỗi tạm thời (5xx, mạng): chỉ đếm và ghi lý do, không tắt cũng không bắt nghỉ.
UPDATE llm_api_key
SET consecutive_failures = consecutive_failures + 1,
    last_error           = sqlc.narg('last_error')
WHERE id = sqlc.arg('id');

-- ---------------------------------------------------------------------------
-- Chia sẻ bộ cho người dùng
-- ---------------------------------------------------------------------------

-- name: ListLLMAPISetUsers :many
SELECT su.set_id, su.user_id, u.email
FROM llm_api_set_user su
JOIN app_user u ON u.id = su.user_id
WHERE su.set_id = ANY(sqlc.arg('set_ids')::uuid[])
ORDER BY u.email;

-- name: AddLLMAPISetUser :exec
INSERT INTO llm_api_set_user (set_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ClearLLMAPISetUsers :exec
DELETE FROM llm_api_set_user WHERE set_id = $1;

-- name: CanUseLLMAPISet :one
-- Câu hỏi của WORKER, không phải của UI: bộ này có thật sự dùng được cho người
-- tạo voice không. Trả về bộ nếu được, không có dòng nào nếu không.
SELECT s.* FROM llm_api_set s
WHERE s.id = sqlc.arg('id')
  AND (s.visible_to_users
       OR s.created_by = sqlc.arg('user_id')
       OR EXISTS (
            SELECT 1 FROM llm_api_set_user su
            WHERE su.set_id = s.id AND su.user_id = sqlc.arg('user_id')
          ));

-- name: CountLLMAPIKeys :one
-- Câu hỏi lúc KHỞI ĐỘNG: hệ thống có đường nào chạy được mode C không.
-- Mode C cần LLM thật; trước đây điều đó chỉ đọc từ .env, nhưng giờ đường chính
-- là Bộ API key trong DB — chỉ nhìn .env thì tắt nhầm mode C của cả hệ thống.
SELECT COUNT(*)::bigint FROM llm_api_key;
