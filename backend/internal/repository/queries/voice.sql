-- name: CreateVoice :one
-- Metadata người dùng điền sẵn ở màn tạo Voice đi vào ngay từ đây (title,
-- hashtag, author, ảnh): worker sau đó chỉ ĐIỀN VÀO CHỖ TRỐNG chứ không ghi đè,
-- nên giá trị người dùng gõ luôn thắng giá trị lấy từ bài gốc.
INSERT INTO voice (
  source_post_id, ai_engine_id, voice_file_url, duration_seconds,
  hashtag, language, image_url, publish_status, title, mime_type, size_bytes,
  sample_rate, created_by,
  author_id, author_email, author_gender,
  image_uploaded, no_image, publish_when_ready, llm_api_set_id, author_country_id,
  spoken_text, tts_config
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
  sqlc.narg('author_id'), sqlc.narg('author_email'), sqlc.narg('author_gender'),
  sqlc.arg('image_uploaded'), sqlc.arg('no_image'), sqlc.arg('publish_when_ready'),
  sqlc.narg('llm_api_set_id'), sqlc.narg('author_country_id'),
  sqlc.narg('spoken_text'), sqlc.narg('tts_config')
)
RETURNING *;

-- name: CreateTextVoice :one
-- Voice gõ tay: không có Bài Post nào đứng sau, nội dung nằm thẳng trên Voice.
-- Tạo ở trạng thái `processing` để người dùng thấy ngay dòng voice đang chạy,
-- worker điền file + metadata vào đúng dòng đó (FinishVoice).
INSERT INTO voice (
  input_text, collect_mode, prompt_id, language, publish_status, title, created_by,
  llm_api_set_id,
  -- Metadata điền sẵn ở form, giống hệt voice tạo từ URL: form tạo voice giờ là
  -- MỘT form cho cả ba hình thức, nên hai đường không được nhận hai bộ trường
  -- khác nhau — lệch một trường là một thứ người dùng điền rồi mà biến mất.
  hashtag, image_url, image_uploaded, no_image,
  author_gender, author_country_id, publish_when_ready, tts_config
) VALUES (
  sqlc.arg('input_text'), sqlc.arg('collect_mode'), sqlc.narg('prompt_id'),
  sqlc.arg('language'), 'processing', sqlc.narg('title'), sqlc.arg('created_by'),
  sqlc.narg('llm_api_set_id'),
  sqlc.narg('hashtag'), sqlc.narg('image_url'), sqlc.arg('image_uploaded'),
  sqlc.arg('no_image'), sqlc.narg('author_gender'), sqlc.narg('author_country_id'),
  sqlc.arg('publish_when_ready'), sqlc.narg('tts_config')
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
    llm_api_set_id = COALESCE(sqlc.narg('llm_api_set_id'), llm_api_set_id),
    -- Lời đọc người dùng tự chốt ở tab Nội dung. Chỉ có giá trị khi họ sửa
    -- thẳng vào ô lời đọc; NULL = để LLM (hoặc chính input_text) quyết định như
    -- thường, nên COALESCE giữ nguyên bản cũ cho tới khi FinishVoice ghi đè.
    spoken_text    = COALESCE(sqlc.narg('spoken_text'), spoken_text),
    -- Cấu hình giọng đọc: cờ set_tts_config phân biệt "không đụng tới" với
    -- "trả về mặc định". COALESCE không làm được việc đó — gửi NULL để xoá
    -- cấu hình sẽ bị hiểu thành giữ nguyên, và người dùng bấm tạo lại sau khi
    -- thu mục cấu hình về mặc định vẫn nghe đúng giọng cũ.
    tts_config     = CASE WHEN sqlc.arg('set_tts_config')::bool
                          THEN sqlc.narg('tts_config')
                          ELSE tts_config END,
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
       sp.extracted_text AS source_extracted_text,
       u.email         AS created_by_email
FROM voice v
LEFT JOIN source_post sp ON sp.id = v.source_post_id
JOIN app_user u     ON u.id = v.created_by
-- Lọc theo trạng thái NGƯỜI DÙNG THẤY, không phải cột thô: voice còn thiếu điều
-- kiện đăng (chưa chọn author, chưa có hashtag/tiêu đề, audio ngắn hơn mức
-- multime nhận) hiện badge "Chưa đủ điều kiện", nên chọn trạng thái đó phải ra
-- đúng những dòng ấy — và "Nháp"/"Chờ đăng" thì không được lẫn chúng.
--
-- 'incomplete' KHÔNG phải giá trị có trong cột publish_status: nó là trạng thái
-- suy ra lúc đọc. Thiếu hashtag là việc người dùng chưa điền xong, khác hẳn
-- 'failed' (đã gửi lên multime và bị từ chối), nên không gộp chung.
WHERE (sqlc.narg('publish_status')::varchar IS NULL
       OR CASE
            WHEN v.publish_status IN ('draft', 'ready')
             AND (v.author_id IS NULL OR v.author_id <= 0
                  OR COALESCE(btrim(v.hashtag), '') = ''
                  OR COALESCE(btrim(v.title), '') = ''
                  OR (v.duration_seconds IS NOT NULL
                      AND v.duration_seconds < sqlc.arg('min_duration')::int))
            THEN 'incomplete'
            ELSE v.publish_status
          END = sqlc.narg('publish_status'))
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
-- Lọc theo trạng thái NGƯỜI DÙNG THẤY, không phải cột thô: voice còn thiếu điều
-- kiện đăng (chưa chọn author, chưa có hashtag/tiêu đề, audio ngắn hơn mức
-- multime nhận) hiện badge "Chưa đủ điều kiện", nên chọn trạng thái đó phải ra
-- đúng những dòng ấy — và "Nháp"/"Chờ đăng" thì không được lẫn chúng.
--
-- 'incomplete' KHÔNG phải giá trị có trong cột publish_status: nó là trạng thái
-- suy ra lúc đọc. Thiếu hashtag là việc người dùng chưa điền xong, khác hẳn
-- 'failed' (đã gửi lên multime và bị từ chối), nên không gộp chung.
WHERE (sqlc.narg('publish_status')::varchar IS NULL
       OR CASE
            WHEN v.publish_status IN ('draft', 'ready')
             AND (v.author_id IS NULL OR v.author_id <= 0
                  OR COALESCE(btrim(v.hashtag), '') = ''
                  OR COALESCE(btrim(v.title), '') = ''
                  OR (v.duration_seconds IS NOT NULL
                      AND v.duration_seconds < sqlc.arg('min_duration')::int))
            THEN 'incomplete'
            ELSE v.publish_status
          END = sqlc.narg('publish_status'))
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
    -- Model THẬT đã viết lại nội dung. COALESCE vì mode B không qua LLM: ghi
    -- thẳng NULL sẽ xoá mất model của lần chạy trước trên chính voice đó.
    llm_model_used   = COALESCE(sqlc.narg('llm_model_used'), llm_model_used),
    -- Đoạn chữ TTS vừa đọc. KHÔNG dùng COALESCE: đây là kết quả của đúng lần
    -- chạy này, giữ lại giá trị cũ nghĩa là mô tả sai file audio vừa ghi đè.
    spoken_text      = sqlc.narg('spoken_text'),
    publish_status   = 'draft',
    last_error       = NULL
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListVoicesBySourcePost :many
SELECT * FROM voice WHERE source_post_id = $1 ORDER BY created_at DESC;

-- name: UpdateVoiceMetadata :one
-- Chọn giới tính KHÔNG còn kéo theo việc bốc tài khoản — việc đó lùi xuống bước
-- đăng. Vì thế author_gender giờ sửa được độc lập với author_id.
--
-- `reset_author` là hệ quả bắt buộc của điều đó: đổi giới tính hoặc quốc gia mà
-- vẫn giữ tài khoản đã bốc trước đó nghĩa là bài lên multime dưới tên một người
-- không khớp thứ người dùng vừa chọn.
UPDATE voice
SET hashtag           = COALESCE(sqlc.narg('hashtag'), hashtag),
    language          = COALESCE(sqlc.narg('language'), language),
    image_url         = COALESCE(sqlc.narg('image_url'), image_url),
    title             = COALESCE(sqlc.narg('title'), title),
    author_gender     = COALESCE(sqlc.narg('author_gender'), author_gender),
    author_country_id = CASE WHEN sqlc.arg('set_country')::bool
                             THEN sqlc.narg('author_country_id')
                             ELSE author_country_id END,
    author_id         = CASE WHEN sqlc.arg('reset_author')::bool THEN NULL
                             ELSE COALESCE(sqlc.narg('author_id'), author_id) END,
    author_email      = CASE WHEN sqlc.arg('reset_author')::bool THEN NULL
                             WHEN sqlc.narg('author_id')::bigint IS NULL THEN author_email
                             ELSE sqlc.narg('author_email') END
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetVoiceImage :one
-- Ảnh bìa tải từ máy: image_uploaded = TRUE đánh dấu file nằm trong storage của
-- mình, để sau khi đăng lên multime thì xoá đi cho đỡ tốn dung lượng.
UPDATE voice
SET image_url      = sqlc.narg('image_url'),
    image_uploaded = sqlc.arg('image_uploaded')
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
-- Ảnh bìa tải từ máy cũng bị xoá theo cùng lý do: multime đã giữ bản của nó,
-- bản trong bucket của mình không còn ai đọc nữa. Ảnh lấy từ URL bài gốc không
-- nằm trong bucket nên giữ nguyên link.
UPDATE voice
SET publish_status   = 'published',
    multime_post_url = sqlc.arg('multime_post_url'),
    voice_file_url   = NULL,
    image_url        = CASE WHEN image_uploaded THEN NULL ELSE image_url END,
    image_uploaded   = FALSE,
    last_error       = NULL,
    published_at     = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteVoice :execrows
DELETE FROM voice WHERE id = $1;

-- name: LastUsedPromptAndSet :one
-- Prompt mẫu + Bộ API mà CHÍNH người này dùng gần đây nhất ở hình thức C.
--
-- Để form tạo voice chọn sẵn thay vì bắt chọn lại mỗi lần: gần như ai cũng chạy
-- đi chạy lại cùng một prompt, và hai ô bắt buộc phải tự tay chọn ở mỗi lần tạo
-- là hai lần bấm thừa cộng một lần quên.
--
-- Hai cột lấy ĐỘC LẬP, mỗi cột từ voice gần nhất có giá trị: chúng được chọn
-- riêng, nên voice gần nhất có prompt chưa chắc là voice gần nhất có bộ API.
SELECT
  (SELECT p.prompt_id FROM voice p
    WHERE p.created_by = sqlc.arg('user_id')::uuid AND p.prompt_id IS NOT NULL
    ORDER BY p.created_at DESC LIMIT 1) AS prompt_id,
  (SELECT k.llm_api_set_id FROM voice k
    WHERE k.created_by = sqlc.arg('user_id')::uuid AND k.llm_api_set_id IS NOT NULL
    ORDER BY k.created_at DESC LIMIT 1) AS llm_api_set_id;
