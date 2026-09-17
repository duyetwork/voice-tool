-- ---------------------------------------------------------------------------
-- Via (phiên đăng nhập dùng để quét trang công khai)
-- ---------------------------------------------------------------------------

-- name: CreateScrapeVia :one
INSERT INTO scrape_via (platform, label, cookies_encrypted, daily_quota, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetScrapeVia :one
SELECT * FROM scrape_via WHERE id = $1;

-- name: ListScrapeVias :many
-- Bảng quản lý trên màn Cài đặt. Sắp theo nền tảng rồi nhãn để danh sách đứng
-- yên giữa các lần tải — sắp theo trạng thái thì dòng nhảy chỗ mỗi lần một via
-- vào cooldown, đúng lúc người ta đang nhìn nó.
SELECT * FROM scrape_via
WHERE sqlc.narg('platform')::varchar IS NULL OR platform = sqlc.narg('platform')
ORDER BY platform, label;

-- name: ClaimScrapeVia :one
-- Nhận via cho MỘT lượt quét: nghỉ lâu nhất trước (round-robin theo thời gian
-- nghỉ), trong nhóm còn khoẻ và còn hạn mức ngày.
--
-- CHỌN VÀ ĐÁNH DẤU TRONG MỘT CÂU, không phải SELECT rồi UPDATE. Hai worker quét
-- song song mà đọc rồi mới ghi thì cả hai cùng thấy một via "nghỉ lâu nhất" và
-- cùng dùng nó — vòng xoay đứng im, một via gánh hết tải, đúng thứ cần tránh
-- nhất khi tài sản quý là số via còn sống. Một câu UPDATE thì Postgres tự khoá
-- dòng, worker thứ hai thấy last_used_at đã đổi và nhận via kế tiếp.
--
-- SKIP LOCKED ở subquery: worker thứ hai nhảy sang via kế thay vì xếp hàng chờ
-- — có vài trăm kênh cần quét thì chờ nhau là tự dồn burst về cuối hàng.
--
-- Chỉ đặt last_used_at ở đây, KHÔNG trừ hạn mức: hạn mức trừ khi lượt quét
-- thành công (xem MarkScrapeViaUsed). Một request bị proxy làm hỏng không phải
-- lỗi của via và không đáng lấy mất một suất của nó.
--
-- `daily_used_date < CURRENT_DATE` cũng được coi là CÒN hạn mức: bộ đếm của
-- ngày hôm qua không phải hạn mức của hôm nay. Nhờ vậy việc reset tự liền ngay
-- cả khi job cron lỡ nhịp.
UPDATE scrape_via
SET last_used_at = now()
WHERE id = (
  -- Bí danh `c`: không có nó thì `platform` trong subquery vừa trỏ tới cột của
  -- bảng đang UPDATE vừa trỏ tới cột của chính subquery, và Postgres từ chối.
  SELECT c.id FROM scrape_via c
  WHERE c.platform = sqlc.arg('platform')
    AND c.status = 'active'
    AND (c.daily_used_date < CURRENT_DATE OR c.daily_used < c.daily_quota)
  ORDER BY c.last_used_at ASC NULLS FIRST, c.id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: UpdateScrapeVia :one
-- Sửa via. Dán cookies mới cũng là RESET SỨC KHOẺ: người vào đây là vì via
-- hỏng, dán cookies mới mà vẫn còn `dead` thì bộ chọn tiếp tục bỏ qua nó và họ
-- không hiểu vì sao (đúng bài học của UpdateLLMAPIKey).
UPDATE scrape_via
SET label             = COALESCE(sqlc.narg('label'), label),
    daily_quota       = COALESCE(sqlc.narg('daily_quota'), daily_quota),
    cookies_encrypted = COALESCE(sqlc.narg('cookies_encrypted'), cookies_encrypted),
    status = CASE
               WHEN sqlc.narg('cookies_encrypted') IS NOT NULL THEN 'active'
               ELSE COALESCE(sqlc.narg('status'), status)
             END,
    consecutive_login_errors =
      CASE WHEN sqlc.narg('cookies_encrypted') IS NULL THEN consecutive_login_errors ELSE 0 END,
    cooldown_until =
      CASE WHEN sqlc.narg('cookies_encrypted') IS NULL THEN cooldown_until ELSE NULL END,
    last_error =
      CASE WHEN sqlc.narg('cookies_encrypted') IS NULL THEN last_error ELSE NULL END,
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteScrapeVia :execrows
DELETE FROM scrape_via WHERE id = $1;

-- name: MarkScrapeViaUsed :exec
-- Sau một lượt quét THÀNH CÔNG: trừ hạn mức, xoá sạch dấu vết hỏng hóc cũ.
--
-- Bộ đếm ngày tự liền ngay tại đây: gặp ngày lệch thì đặt lại về 1 thay vì cộng
-- tiếp vào con số của hôm qua.
UPDATE scrape_via
SET last_used_at = now(),
    daily_used = CASE WHEN daily_used_date < CURRENT_DATE THEN 1 ELSE daily_used + 1 END,
    daily_used_date          = CURRENT_DATE,
    consecutive_login_errors = 0,
    last_error               = NULL,
    updated_at               = now()
WHERE id = $1;

-- name: MarkScrapeViaLoginError :one
-- Nền tảng đòi đăng nhập — phiên của via này có vấn đề.
--
-- Một câu UPDATE lo trọn máy trạng thái, thay vì đọc rồi tính rồi ghi: hai
-- worker cùng gặp lỗi trên một via sẽ chạy hai lượt, và đọc-tính-ghi thì lượt
-- sau ghi đè lượt trước, số đếm đứng yên và via không bao giờ chết.
--
--   chưa đủ ngưỡng          -> vẫn active, chỉ tăng số đếm
--   đủ ngưỡng, lần đầu      -> cooldown, hẹn giờ thử lại
--   lại hỏng sau cooldown   -> dead (đã cho một cơ hội rồi)
--
-- Nhận ra "lại hỏng sau cooldown" bằng cooldown_until: cột này chỉ được đặt khi
-- via đã từng đi qua cooldown, và MarkScrapeViaUsed xoá nó khi via sống lại
-- thật. Còn giá trị nghĩa là lần cuối cùng ta cho nó cơ hội, nó vẫn hỏng.
UPDATE scrape_via
SET consecutive_login_errors = consecutive_login_errors + 1,
    last_error_at            = now(),
    last_error               = sqlc.narg('last_error'),
    -- Nhánh `cooldown_until IS NOT NULL` đứng TRƯỚC nhánh ngưỡng, và thứ tự đó
    -- là toàn bộ ý nghĩa của bước "chết": via đã đi qua một vòng cooldown rồi
    -- mà vẫn bị đòi đăng nhập thì không cần đếm lại đủ N lần nữa — N lần đầu đã
    -- là bằng chứng, vòng nghỉ là cơ hội, và lần này là câu trả lời.
    --
    -- Đảo hai nhánh thì via hỏng sống thêm N lượt quét sau mỗi lần hồi sinh, và
    -- mỗi lượt đó là một lần đem một phiên đã chết ra trước mặt nền tảng.
    status = CASE
               WHEN cooldown_until IS NOT NULL THEN 'dead'
               WHEN consecutive_login_errors + 1 < sqlc.arg('threshold')::int THEN status
               ELSE 'cooldown'
             END,
    cooldown_until = CASE
               WHEN cooldown_until IS NOT NULL THEN cooldown_until
               WHEN consecutive_login_errors + 1 < sqlc.arg('threshold')::int THEN cooldown_until
               ELSE now() + sqlc.arg('cooldown')::interval
             END,
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: NoteScrapeViaError :exec
-- Lỗi KHÔNG phải của via (bot_block, rate_limit, mạng đứt): ghi lại để đối
-- chiếu, nhưng không đụng tới máy trạng thái.
--
-- Vì sao tách khỏi câu trên: đánh via chết vì một IP bị chặn là thay nhầm thứ
-- đang hỏng — via vẫn tốt, và ta vừa vứt đi một tài khoản còn dùng được.
UPDATE scrape_via
SET last_error_at = now(), last_error = sqlc.narg('last_error'), updated_at = now()
WHERE id = sqlc.arg('id');

-- name: ReviveScrapeVias :execrows
-- Job định kỳ: via hết cooldown thì cho thử lại.
--
-- GIỮ NGUYÊN cooldown_until (không xoá về NULL) vì nó chính là thứ đánh dấu
-- "via này đã được cho một cơ hội" — xem MarkScrapeViaLoginError. Xoá nó ở đây
-- thì via hỏng lại sẽ vào cooldown lần nữa, và lặp mãi không bao giờ chết.
UPDATE scrape_via
SET status = 'active', consecutive_login_errors = 0, updated_at = now()
WHERE status = 'cooldown' AND cooldown_until IS NOT NULL AND cooldown_until <= now();

-- name: ResetScrapeViaDailyUsed :execrows
-- Job hằng ngày. Bộ đếm đã tự liền theo ngày ở MarkScrapeViaUsed, nên câu này
-- chỉ để bảng nhìn đúng trên giao diện trước lượt dùng đầu tiên của ngày mới.
UPDATE scrape_via
SET daily_used = 0, daily_used_date = CURRENT_DATE, updated_at = now()
WHERE daily_used_date < CURRENT_DATE;

-- name: CountScrapeViasByStatus :many
-- Bảng tổng quan trên màn Cài đặt: bao nhiêu via sống/chết theo từng nền tảng.
SELECT platform, status, COUNT(*)::bigint AS total
FROM scrape_via
GROUP BY platform, status;
