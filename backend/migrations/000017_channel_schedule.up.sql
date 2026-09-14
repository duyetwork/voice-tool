-- ---------------------------------------------------------------------------
-- Lập lịch theo từng kênh + đếm lỗi bị chặn theo nền tảng.
--
-- Trước migration này, "lịch theo kênh" mới chỉ là scan_frequency: chạy đều
-- 24/7 theo giờ container (UTC). Hai hệ quả:
--
--   * Kênh tin tức không đăng lúc 3h sáng, nhưng hệ thống vẫn quét — đốt quota
--     và mời gọi rate-limit đúng vào lúc chẳng thu được gì.
--   * "Chỉ quét 6h–23h" mà tính theo UTC thì lệch 7 tiếng so với ý người dùng.
--     Nên khung giờ phải đi kèm MÚI GIỜ của chính kênh đó.
--
-- Giờ lưu bằng SỐ PHÚT TÍNH TỪ NỬA ĐÊM (0–1439) chứ không phải kiểu TIME: giá
-- trị đi qua JSON, qua pgx và qua ô nhập <input type="time"> của trình duyệt mà
-- không phải đổi kiểu ở ba nơi, và "06:00" luôn là 360 dù đọc ở tầng nào.
--
-- fetch_error_stat là dữ liệu để QUYẾT ĐỊNH có cần proxy hay không. Bộ phân
-- loại lỗi yt-dlp đã gắn nhãn sẵn từng loại (xem infra/platform/ytdlperr.go);
-- ở đây chỉ lưu số đếm. Mua proxy trước khi có số đếm là trả tiền cho một
-- phỏng đoán.
-- ---------------------------------------------------------------------------

ALTER TABLE list_scheduled
  -- Múi giờ diễn giải khung giờ bên dưới. Tên IANA, không phải offset: offset
  -- không biết tới giờ mùa hè.
  ADD COLUMN timezone        TEXT NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
  -- Khung giờ hoạt động, phút tính từ nửa đêm. Cả hai NULL = quét 24/7 (giữ
  -- nguyên hành vi cũ). from > to nghĩa là khung vắt qua nửa đêm (22:00–06:00).
  ADD COLUMN active_from_min SMALLINT CHECK (active_from_min BETWEEN 0 AND 1439),
  ADD COLUMN active_to_min   SMALLINT CHECK (active_to_min   BETWEEN 0 AND 1439),
  -- Thứ trong tuần được quét, theo quy ước của Postgres EXTRACT(DOW) và của
  -- Go time.Weekday: 0 = Chủ nhật … 6 = Thứ bảy. NULL hoặc rỗng = mọi ngày.
  ADD COLUMN active_weekdays SMALLINT[],
  -- Giờ chạy cố định (phút từ nửa đêm), là LỰA CHỌN THAY THẾ cho "mỗi N phút":
  -- có giá trị thì scan_frequency bị bỏ qua. Với kênh đăng theo giờ cố định
  -- (08:00/12:00/18:00) đây vừa đúng hơn vừa rẻ hơn hẳn.
  ADD COLUMN fixed_times_min SMALLINT[];

ALTER TABLE list_breaking
  ADD COLUMN timezone        TEXT NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
  ADD COLUMN active_from_min SMALLINT CHECK (active_from_min BETWEEN 0 AND 1439),
  ADD COLUMN active_to_min   SMALLINT CHECK (active_to_min   BETWEEN 0 AND 1439),
  ADD COLUMN active_weekdays SMALLINT[];

-- Số lần bị nền tảng chặn, gộp theo ngày để bảng không phình theo từng lỗi.
CREATE TABLE fetch_error_stat (
  day      DATE        NOT NULL,
  platform VARCHAR(20) NOT NULL,
  -- kind là nhãn của bộ phân loại lỗi: bot_block, login_required, rate_limit,
  -- geo_blocked, unavailable, timeout, other.
  kind     VARCHAR(30) NOT NULL,
  count    BIGINT      NOT NULL DEFAULT 0,
  last_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (day, platform, kind)
);

CREATE INDEX idx_fetch_error_stat_day ON fetch_error_stat(day DESC);
