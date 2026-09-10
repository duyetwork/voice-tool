-- name: CreateSourcePost :one
INSERT INTO source_post (
  source_type, list_breaking_id, list_scheduled_id, source_url, platform,
  content_type, post_id_extracted, extracted_text, collect_mode, prompt_id,
  language, status, created_by, title, hashtags, thumbnail_url,
  author_name, posted_at
) VALUES (
  sqlc.arg('source_type'), sqlc.narg('list_breaking_id'), sqlc.narg('list_scheduled_id'),
  sqlc.arg('source_url'), sqlc.arg('platform'), sqlc.narg('content_type'),
  sqlc.narg('post_id_extracted'), sqlc.narg('extracted_text'), sqlc.arg('collect_mode'),
  sqlc.narg('prompt_id'), sqlc.arg('language'), sqlc.arg('status'),
  sqlc.arg('created_by'), sqlc.narg('title'),
  COALESCE(sqlc.narg('hashtags')::text[], '{}'),
  sqlc.narg('thumbnail_url'), sqlc.narg('author_name'), sqlc.narg('posted_at')
)
RETURNING *;

-- name: GetSourcePost :one
SELECT * FROM source_post WHERE id = $1;

-- name: ListSourcePosts :many
SELECT sp.*, u.email AS created_by_email
FROM source_post sp
JOIN app_user u ON u.id = sp.created_by
WHERE (sqlc.narg('source_type')::varchar IS NULL OR sp.source_type = sqlc.narg('source_type'))
  AND (sqlc.narg('status')::varchar       IS NULL OR sp.status       = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar     IS NULL OR sp.platform     = sqlc.narg('platform'))
  AND (sqlc.narg('collect_mode')::varchar IS NULL OR sp.collect_mode = sqlc.narg('collect_mode'))
  AND (sqlc.narg('language')::varchar     IS NULL OR sp.language     = sqlc.narg('language'))
  AND (sqlc.narg('created_by')::uuid        IS NULL OR sp.created_by        = sqlc.narg('created_by'))
  AND (sqlc.narg('list_breaking_id')::uuid  IS NULL OR sp.list_breaking_id  = sqlc.narg('list_breaking_id'))
  AND (sqlc.narg('list_scheduled_id')::uuid IS NULL OR sp.list_scheduled_id = sqlc.narg('list_scheduled_id'))
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR sp.created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz   IS NULL OR sp.created_at <= sqlc.narg('created_to'))
-- Sắp xếp động: chỉ có 1 cột thời gian nên chỉ cần chiều. Nhánh CASE toàn
-- NULL khi dir='desc' -> rơi về mặc định mới nhất trước.
ORDER BY
  CASE WHEN sqlc.arg('dir')::text = 'asc' THEN sp.created_at END ASC,
  sp.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountSourcePosts :one
SELECT COUNT(*) FROM source_post sp
WHERE (sqlc.narg('source_type')::varchar IS NULL OR sp.source_type = sqlc.narg('source_type'))
  AND (sqlc.narg('status')::varchar       IS NULL OR sp.status       = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar     IS NULL OR sp.platform     = sqlc.narg('platform'))
  AND (sqlc.narg('collect_mode')::varchar IS NULL OR sp.collect_mode = sqlc.narg('collect_mode'))
  AND (sqlc.narg('language')::varchar     IS NULL OR sp.language     = sqlc.narg('language'))
  AND (sqlc.narg('created_by')::uuid        IS NULL OR sp.created_by        = sqlc.narg('created_by'))
  AND (sqlc.narg('list_breaking_id')::uuid  IS NULL OR sp.list_breaking_id  = sqlc.narg('list_breaking_id'))
  AND (sqlc.narg('list_scheduled_id')::uuid IS NULL OR sp.list_scheduled_id = sqlc.narg('list_scheduled_id'))
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR sp.created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz   IS NULL OR sp.created_at <= sqlc.narg('created_to'));

-- name: UpdateSourcePost :one
UPDATE source_post
SET collect_mode   = COALESCE(sqlc.narg('collect_mode'), collect_mode),
    prompt_id      = COALESCE(sqlc.narg('prompt_id'), prompt_id),
    language       = COALESCE(sqlc.narg('language'), language),
    extracted_text = COALESCE(sqlc.narg('extracted_text'), extracted_text)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: UpdateSourcePostMetadata :one
-- Worker ghi lại metadata gốc lấy được từ nền tảng để Voice và form đăng bài
-- auto-fill từ đây.
--
-- `title` là TOÀN BỘ nội dung bài (trừ hashtag) — hệ thống không còn trường mô
-- tả riêng. COALESCE để lần fetch không ra thì giữ nguyên phần đã có.
UPDATE source_post
SET title         = COALESCE(sqlc.narg('title'), title),
    hashtags      = COALESCE(sqlc.narg('hashtags')::text[], hashtags),
    thumbnail_url = COALESCE(sqlc.narg('thumbnail_url'), thumbnail_url),
    author_name   = COALESCE(sqlc.narg('author_name'), author_name),
    posted_at     = COALESCE(sqlc.narg('posted_at'), posted_at),
    content_type  = COALESCE(sqlc.narg('content_type'), content_type)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetSourcePostStatus :one
UPDATE source_post
SET status = sqlc.arg('status'), last_error = sqlc.narg('last_error')
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ClaimSourcePostForProcessing :one
-- Chỉ 1 worker được xử lý 1 bài tại 1 thời điểm (idempotent khi Asynq retry).
UPDATE source_post
SET status = 'processing', last_error = NULL
WHERE id = $1 AND status IN ('new', 'failed')
RETURNING *;

-- name: DeleteSourcePost :execrows
DELETE FROM source_post WHERE id = $1;

-- name: FindSourcePostByPostID :one
-- Tra cứu bài trùng theo ID bài đăng trên nền tảng (KHÔNG theo URL): cùng 1
-- bài có nhiều dạng URL khác nhau nhưng chỉ 1 id.
-- Trả bản CŨ NHẤT để thông báo trùng luôn trỏ về bài gốc.
SELECT * FROM source_post
WHERE platform = $1 AND post_id_extracted = $2
ORDER BY created_at ASC
LIMIT 1;
