-- name: CreateVoice :one
INSERT INTO voice (
  source_post_id, ai_engine_id, voice_file_url, duration_seconds, description,
  hashtag, language, image_url, publish_status, title, mime_type, size_bytes,
  sample_rate, created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetVoice :one
SELECT * FROM voice WHERE id = $1;

-- name: ListVoices :many
-- Trả kèm nền tảng nguồn + email người tạo để bảng Voice hiển thị và lọc được
-- mà không phải gọi thêm API (prompt.md mục 3, 4, 8).
SELECT v.*,
       sp.platform    AS platform,
       sp.source_url  AS source_url,
       sp.title       AS source_title,
       u.email        AS created_by_email
FROM voice v
JOIN source_post sp ON sp.id = v.source_post_id
JOIN app_user u     ON u.id = v.created_by
WHERE (sqlc.narg('publish_status')::varchar IS NULL OR v.publish_status = sqlc.narg('publish_status'))
  AND (sqlc.narg('source_post_id')::uuid    IS NULL OR v.source_post_id = sqlc.narg('source_post_id'))
  AND (sqlc.narg('platform')::varchar       IS NULL OR sp.platform      = sqlc.narg('platform'))
  AND (sqlc.narg('language')::varchar       IS NULL OR v.language       = sqlc.narg('language'))
  AND (sqlc.narg('created_by')::uuid        IS NULL OR v.created_by     = sqlc.narg('created_by'))
  AND (sqlc.narg('created_from')::timestamptz   IS NULL OR v.created_at   >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz     IS NULL OR v.created_at   <= sqlc.narg('created_to'))
  AND (sqlc.narg('published_from')::timestamptz IS NULL OR v.published_at >= sqlc.narg('published_from'))
  AND (sqlc.narg('published_to')::timestamptz   IS NULL OR v.published_at <= sqlc.narg('published_to'))
ORDER BY v.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountVoices :one
SELECT COUNT(*)
FROM voice v
JOIN source_post sp ON sp.id = v.source_post_id
WHERE (sqlc.narg('publish_status')::varchar IS NULL OR v.publish_status = sqlc.narg('publish_status'))
  AND (sqlc.narg('source_post_id')::uuid    IS NULL OR v.source_post_id = sqlc.narg('source_post_id'))
  AND (sqlc.narg('platform')::varchar       IS NULL OR sp.platform      = sqlc.narg('platform'))
  AND (sqlc.narg('language')::varchar       IS NULL OR v.language       = sqlc.narg('language'))
  AND (sqlc.narg('created_by')::uuid        IS NULL OR v.created_by     = sqlc.narg('created_by'))
  AND (sqlc.narg('created_from')::timestamptz   IS NULL OR v.created_at   >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz     IS NULL OR v.created_at   <= sqlc.narg('created_to'))
  AND (sqlc.narg('published_from')::timestamptz IS NULL OR v.published_at >= sqlc.narg('published_from'))
  AND (sqlc.narg('published_to')::timestamptz   IS NULL OR v.published_at <= sqlc.narg('published_to'));

-- name: ListVoicesBySourcePost :many
SELECT * FROM voice WHERE source_post_id = $1 ORDER BY created_at DESC;

-- name: UpdateVoiceMetadata :one
UPDATE voice
SET description = COALESCE(sqlc.narg('description'), description),
    hashtag     = COALESCE(sqlc.narg('hashtag'), hashtag),
    language    = COALESCE(sqlc.narg('language'), language),
    image_url   = COALESCE(sqlc.narg('image_url'), image_url),
    title       = COALESCE(sqlc.narg('title'), title)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetVoicePublishStatus :one
UPDATE voice
SET publish_status = sqlc.arg('publish_status'), last_error = sqlc.narg('last_error')
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: MarkVoicePublished :one
-- Business rule #2: publish thành công -> xoá file S3 và set voice_file_url = NULL,
-- chỉ giữ multime_post_url làm nguồn tham chiếu duy nhất.
UPDATE voice
SET publish_status   = 'published',
    multime_post_url = sqlc.arg('multime_post_url'),
    voice_file_url   = NULL,
    last_error       = NULL,
    published_at     = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteVoice :execrows
DELETE FROM voice WHERE id = $1;
