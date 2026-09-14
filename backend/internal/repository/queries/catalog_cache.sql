-- name: ListCountries :many
-- Thứ tự nghiệp vụ trước, rồi alphabet cho phần còn lại — xem cột sort_order.
SELECT * FROM country ORDER BY sort_order, name;

-- name: CountriesSyncedAt :one
-- Mốc đồng bộ cũ nhất trong bảng: chỉ cần 1 bản ghi quá hạn là đồng bộ lại cả
-- danh mục, vì chúng luôn được ghi cùng một lượt.
SELECT COALESCE(MIN(synced_at), 'epoch'::timestamptz)::timestamptz AS synced_at,
       COUNT(*)::bigint AS total
FROM country;

-- name: UpsertCountry :exec
INSERT INTO country (id, name, code, sort_order, synced_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name, code = EXCLUDED.code,
    sort_order = EXCLUDED.sort_order, synced_at = now();

-- name: ListHashtags :many
-- Danh sách cho ô chọn: tag MultiMe tuyển chọn (is_featured, rồi `system`) lên
-- trước, phần còn lại theo ordering/tên.
--
-- `q` rỗng = lấy phần đầu danh mục. Có `q` thì lọc ngay trong DB: danh mục có
-- ~94.000 tag nên không thể đẩy hết về trình duyệt để lọc tại chỗ.
SELECT id, name, normalized_name, slug, voice_tag_kind, is_featured
FROM hashtag
WHERE sqlc.arg('q')::text = ''
   OR normalized_name LIKE sqlc.arg('q')::text || '%'
   OR lower(name) LIKE '%' || sqlc.arg('q')::text || '%'
ORDER BY
  -- Khớp từ đầu chuỗi đứng trước khớp ở giữa: gõ "new" thì "News" phải lên
  -- trước "Renewable".
  (normalized_name LIKE sqlc.arg('q')::text || '%') DESC,
  is_featured DESC,
  voice_tag_kind,
  ordering,
  name
LIMIT sqlc.arg('lim');

-- name: HashtagsSyncedAt :one
SELECT COALESCE(MIN(synced_at), 'epoch'::timestamptz)::timestamptz AS synced_at,
       COUNT(*)::bigint AS total
FROM hashtag;

-- name: UpsertHashtag :exec
INSERT INTO hashtag (id, name, normalized_name, slug, voice_tag_kind, is_featured, ordering, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (id) DO UPDATE
SET name            = EXCLUDED.name,
    normalized_name = EXCLUDED.normalized_name,
    slug            = EXCLUDED.slug,
    voice_tag_kind  = EXCLUDED.voice_tag_kind,
    is_featured     = EXCLUDED.is_featured,
    ordering        = EXCLUDED.ordering,
    synced_at       = now();

-- name: SetVoiceAuthor :exec
-- Ghi lại tài khoản đã BỐC lúc đăng. Không dùng UpdateVoice vì đây là worker
-- chốt kết quả của một lần bốc, không phải người dùng sửa metadata.
UPDATE voice
SET author_id = sqlc.arg('author_id'), author_email = sqlc.narg('author_email')
WHERE id = sqlc.arg('id');
