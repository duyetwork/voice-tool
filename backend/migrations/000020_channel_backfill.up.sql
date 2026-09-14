-- ---------------------------------------------------------------------------
-- Lấy bài cũ khi thêm kênh, và trần bài mỗi vòng quét cho cả hai loại kênh.
--
-- Vấn đề đang có: thêm một kênh mới thì VÒNG QUÉT ĐẦU TIÊN lặng lẽ nuốt trọn
-- `scan_limit` bài mới nhất (mặc định 20) — toàn bài đã đăng từ trước, và nếu
-- kênh bật auto_process thì đó là 20 voice được tạo ngay lập tức. Người thêm
-- kênh không hề chọn điều đó, cũng không có chỗ nào để chọn khác đi.
--
-- `backfill_limit` biến việc ấy thành một quyết định tường minh:
--
--     0  (mặc định) = chỉ lấy bài đăng SAU khi thêm kênh
--     N            = lấy thêm N bài cũ nhất-định, tính lùi từ lúc thêm kênh
--
-- Đếm theo SỐ BÀI chứ không theo khoảng thời gian: danh sách nền tảng trả về
-- vốn đã là "N bài mới nhất", nên đếm bài là đếm đúng thứ đang cầm trên tay.
-- Lọc theo ngày đăng thì phải tin vào `posted_at`, mà yt-dlp không trả trường
-- này ổn định trên mọi nền tảng — kênh nào thiếu ngày sẽ bị lọc sai im lặng.
--
-- `backfill_done_at` đánh dấu vòng quét đầu đã chạy xong. Cần một cột riêng
-- chứ không suy ra từ `last_synced_post_id`/`last_scanned_at`: cả hai đều có
-- thể còn NULL sau một vòng quét hợp lệ (kênh chưa có bài nào), và lúc đó hạn
-- mức lấy bài cũ sẽ được áp dụng lại lần nữa.
-- ---------------------------------------------------------------------------

ALTER TABLE list_scheduled
  ADD COLUMN backfill_limit   INT NOT NULL DEFAULT 0 CHECK (backfill_limit >= 0),
  ADD COLUMN backfill_done_at TIMESTAMPTZ;

ALTER TABLE list_breaking
  ADD COLUMN backfill_limit   INT NOT NULL DEFAULT 0 CHECK (backfill_limit >= 0),
  ADD COLUMN backfill_done_at TIMESTAMPTZ,
  -- Kênh Breaking KHÔNG có mốc đồng bộ: mỗi vòng nó xét lại đúng cửa sổ
  -- `scan_limit` bài mới nhất và dựa vào dedup để không tạo trùng. Nghĩa là
  -- bài cũ bị bỏ qua ở vòng đầu sẽ quay lại ở vòng thứ hai như thể vừa mới
  -- đăng. Nên phải NHỚ những id đã cố tình bỏ.
  --
  -- Mảng chứ không phải bảng riêng vì nó bị chặn trên bởi chính `scan_limit`
  -- (tối đa 200 id), ghi đúng một lần ở vòng quét đầu rồi không đổi nữa.
  ADD COLUMN backfill_excluded_ids TEXT[] NOT NULL DEFAULT '{}',
  -- Trần số Bài Post tạo ra trong 1 vòng quét. Trước đây chỉ list_scheduled có;
  -- kênh Breaking khớp regex ồ ạt cũng nổ chi phí AI y hệt. NULL = không giới
  -- hạn (xem MAX_POSTS_PER_RUN_DEFAULT).
  ADD COLUMN max_posts_per_run INT
    CHECK (max_posts_per_run IS NULL OR max_posts_per_run > 0);

-- Kênh đã tồn tại đều đã đi qua vòng quét đầu của nó từ lâu. Không đánh dấu ở
-- đây thì migration này biến chúng thành "chưa backfill" và, với mặc định 0,
-- vòng quét kế tiếp sẽ bỏ qua toàn bộ bài đang chờ trong cửa sổ quét.
UPDATE list_scheduled SET backfill_done_at = now();
UPDATE list_breaking  SET backfill_done_at = now();
