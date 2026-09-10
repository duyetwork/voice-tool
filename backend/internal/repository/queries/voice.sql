-- name: CreateVoice :one
INSERT INTO voice (
  source_post_id, ai_engine_id, voice_file_url, duration_seconds,
  hashtag, language, image_url, publish_status, title, mime_type, size_bytes,
  sample_rate, created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: CreateTextVoice :one
-- Voice gõ tay: không có Bài Post nào đứng sau, nội dung nằm thẳng trên Voice.
-- Tạo ở trạng thái `processing` để người dùng thấy ngay dòng voice đang chạy,
-- worker điền file + metadata vào đúng dòng đó (FinishVoice).
INSERT INTO voice (
  input_text, collect_mode, prompt_id, language, publish_status, title, created_by
) VALUES (
  sqlc.arg('input_text'), sqlc.arg('collect_mode'), sqlc.narg('prompt_id'),
  sqlc.arg('language'), 'processing', sqlc.narg('title'), sqlc.arg('created_by')
)
RETURNING *;

-- name: SetVoiceContent :one
-- Chốt lời đọc mới cho 1 Voice rồi đưa lại vào hàng đợi.
--
-- Đặt luôn publish_status='processing' trong cùng câu lệnh: người dùng bấm
-- "Tạo lại" là thấy dòng voice chuyển sang đang xử lý ngay, không có khoảng
-- giữa mà bảng vẫn hiện voice cũ như chưa có gì xảy ra.
UPDATE voice
SET input_text     = sqlc.arg('input_text'),
    collect_mode   = sqlc.arg('collect_mode'),
    prompt_id      = sqlc.narg('prompt_id'),
    language       = COALESCE(sqlc.narg('language'), language),
    publish_status = 'processing',
    last_error     = NULL
WHERE id = sqlc.arg('id') AND publish_status <> 'published'
RETURNING *;

-- name: ClaimVoiceForProcessing :one
-- Nhận Voice về để đọc. Chỉ nhận khi voice đang `processing` hoặc đã `failed`:
-- Asynq retry lần sau vẫn nhặt lại được, còn voice đã ra file (draft/ready) thì
-- không đọc đè lên — muốn đọc lại phải đi qua SetVoiceContent, nơi người dùng
-- chốt lại lời đọc. Voice đã publish thì không bao giờ đụng vào.
UPDATE voice
SET publish_status = 'processing', last_error = NULL
WHERE id = $1 AND publish_status IN ('processing', 'failed')
RETURNING *;

-- name: GetVoice :one
SELECT * FROM voice WHERE id = $1;

-- name: ListVoices :many
-- Trả kèm nền tảng nguồn + email người tạo để bảng Voice hiển thị và lọc được
-- mà không phải gọi thêm API (prompt.md mục 3, 4, 8).
SELECT v.*,
       sp.platform     AS platform,
       sp.source_url   AS source_url,
       sp.title        AS source_title,
       sp.collect_mode AS source_collect_mode,
       sp.prompt_id    AS source_prompt_id,
       u.email         AS created_by_email
FROM voice v
LEFT JOIN source_post sp ON sp.id = v.source_post_id
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
-- Sắp xếp động: mỗi nhánh CASE chỉ có giá trị khi đúng cột + đúng chiều đang
-- chọn, các nhánh còn lại toàn NULL nên không ảnh hưởng thứ tự. Dòng cuối là
-- mặc định (mới nhất trước) và cũng là nhánh sort=created_at + dir=desc.
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'published_at' AND sqlc.arg('dir')::text = 'asc'
       THEN v.published_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'published_at' AND sqlc.arg('dir')::text = 'desc'
       THEN v.published_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('dir')::text = 'asc' AND sqlc.arg('sort')::text <> 'published_at'
       THEN v.created_at END ASC,
  v.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountVoices :one
SELECT COUNT(*)
FROM voice v
LEFT JOIN source_post sp ON sp.id = v.source_post_id
WHERE (sqlc.narg('publish_status')::varchar IS NULL OR v.publish_status = sqlc.narg('publish_status'))
  AND (sqlc.narg('source_post_id')::uuid    IS NULL OR v.source_post_id = sqlc.narg('source_post_id'))
  AND (sqlc.narg('platform')::varchar       IS NULL OR sp.platform      = sqlc.narg('platform'))
  AND (sqlc.narg('language')::varchar       IS NULL OR v.language       = sqlc.narg('language'))
  AND (sqlc.narg('created_by')::uuid        IS NULL OR v.created_by     = sqlc.narg('created_by'))
  AND (sqlc.narg('created_from')::timestamptz   IS NULL OR v.created_at   >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz     IS NULL OR v.created_at   <= sqlc.narg('created_to'))
  AND (sqlc.narg('published_from')::timestamptz IS NULL OR v.published_at >= sqlc.narg('published_from'))
  AND (sqlc.narg('published_to')::timestamptz   IS NULL OR v.published_at <= sqlc.narg('published_to'));

-- name: FinishVoice :one
-- Worker điền kết quả vào record `processing` đã tạo sẵn lúc enqueue.
-- Metadata dùng COALESCE: fetch không ra tiêu đề thì giữ nguyên phần đã điền
-- sẵn từ Bài Post, không xoá trắng.
UPDATE voice
SET ai_engine_id     = sqlc.narg('ai_engine_id'),
    voice_file_url   = sqlc.arg('voice_file_url'),
    duration_seconds = sqlc.narg('duration_seconds'),
    title            = COALESCE(sqlc.narg('title'), title),
    hashtag          = COALESCE(sqlc.narg('hashtag'), hashtag),
    image_url        = COALESCE(sqlc.narg('image_url'), image_url),
    language         = sqlc.arg('language'),
    mime_type        = sqlc.narg('mime_type'),
    size_bytes       = sqlc.narg('size_bytes'),
    sample_rate      = sqlc.narg('sample_rate'),
    publish_status   = 'draft',
    last_error       = NULL
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListVoicesBySourcePost :many
SELECT * FROM voice WHERE source_post_id = $1 ORDER BY created_at DESC;

-- name: UpdateVoiceMetadata :one
UPDATE voice
SET hashtag     = COALESCE(sqlc.narg('hashtag'), hashtag),
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
