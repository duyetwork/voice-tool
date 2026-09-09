-- ---------------------------------------------------------------------------
-- 1. Nhiều Regex Pattern cho 1 kênh Breaking (kết hợp OR)
-- ---------------------------------------------------------------------------
ALTER TABLE list_breaking ADD COLUMN regex_patterns TEXT[] NOT NULL DEFAULT '{}';

UPDATE list_breaking SET regex_patterns = ARRAY[regex_pattern] WHERE regex_pattern <> '';

ALTER TABLE list_breaking ALTER COLUMN regex_patterns DROP DEFAULT;
ALTER TABLE list_breaking DROP COLUMN regex_pattern;
ALTER TABLE list_breaking
  ADD CONSTRAINT ck_list_breaking_patterns CHECK (cardinality(regex_patterns) > 0);

-- ---------------------------------------------------------------------------
-- 2. Tham số điều chỉnh việc quét, cấu hình theo từng kênh.
--    NULL/mặc định = dùng chỉ số tối ưu của hệ thống (biến môi trường).
-- ---------------------------------------------------------------------------
ALTER TABLE list_breaking
  -- Số bài mới nhất lấy về mỗi vòng quét. 20 đủ bắt kịp kênh đăng dày mà
  -- không tốn quota vô ích.
  ADD COLUMN scan_limit INT NOT NULL DEFAULT 20
    CHECK (scan_limit BETWEEN 1 AND 200),
  -- Khoảng nghỉ riêng của kênh này giữa 2 vòng quét. NULL = theo
  -- BREAKING_SCAN_INTERVAL của hệ thống.
  ADD COLUMN scan_interval INTERVAL
    CHECK (scan_interval IS NULL OR scan_interval >= INTERVAL '15 seconds'),
  ADD COLUMN last_scanned_at TIMESTAMPTZ;

ALTER TABLE list_scheduled
  ADD COLUMN scan_limit INT NOT NULL DEFAULT 20
    CHECK (scan_limit BETWEEN 1 AND 200),
  -- Trần số Bài Post tạo ra trong 1 vòng quét — chặn trường hợp kênh đăng ồ ạt
  -- làm nổ chi phí AI. NULL = không giới hạn.
  ADD COLUMN max_posts_per_run INT
    CHECK (max_posts_per_run IS NULL OR max_posts_per_run > 0),
  ADD COLUMN last_scanned_at TIMESTAMPTZ;

-- ---------------------------------------------------------------------------
-- 3. Phân quyền: admin / editor / viewer. Đăng ký mở, mặc định viewer.
-- ---------------------------------------------------------------------------
ALTER TABLE app_user
  ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'viewer'
    CHECK (role IN ('admin', 'editor', 'viewer'));

-- ---------------------------------------------------------------------------
-- 4. Metadata audio — multime.ai cần duration/size/mime để tạo audio asset.
-- ---------------------------------------------------------------------------
ALTER TABLE voice
  ADD COLUMN title       TEXT,
  ADD COLUMN mime_type   VARCHAR(60),
  ADD COLUMN size_bytes  BIGINT,
  ADD COLUMN sample_rate INT;

CREATE INDEX idx_app_user_role ON app_user(role);
