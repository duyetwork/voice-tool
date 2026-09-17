-- ---------------------------------------------------------------------------
-- Hạ tầng via (tài khoản đăng nhập) + proxy để quét kênh Facebook / X /
-- Instagram.
--
-- VÌ SAO TỒN TẠI: ba nền tảng này không liệt kê được bài của một trang qua
-- yt-dlp (không có extractor), và kênh nguồn là trang CÔNG KHAI CỦA NGƯỜI KHÁC
-- nên không có đường API chính thức nào — Graph API cần page access token do
-- admin trang cấp. Quyết định đã chốt: tự lấy bằng phiên đăng nhập + proxy.
--
-- RỦI RO ĐI KÈM, ghi ở đây vì đây là nơi không ai đọc sót: cách làm này vi phạm
-- điều khoản sử dụng của cả ba nền tảng, mang rủi ro pháp lý về scraping, và
-- chi phí thật của nó là via CHẾT LIÊN TỤC phải thay — chi phí vận hành chứ
-- không phải chi phí một lần. Nặng nhất là rủi ro liên đới: nếu via dùng chung
-- hạ tầng/địa chỉ với Business account đang dùng để đăng bài thì Meta khoá luôn
-- tài khoản đó. Giữ hai thứ tách biệt hoàn toàn.
--
-- VÌ SAO KHÔNG CÓ CỘT `*_secret_ref`: pkg/secret là một hộp mã hoá AES-256-GCM,
-- KHÔNG phải kho khoá-giá trị có địa chỉ để trỏ tới. Cách đang dùng trong repo
-- là để ciphertext thẳng trong cột `*_encrypted` (ai_engine.api_key_encrypted,
-- llm_api_key.api_key_encrypted). Một cột "ref" ở đây sẽ trỏ vào hư không.
-- ---------------------------------------------------------------------------

-- ---------------------------------------------------------------------------
-- scrape_via — một phiên đăng nhập dùng để đọc trang công khai.
--
-- Máy trạng thái mượn nguyên hình dạng của llm_api_key (cooldown_until +
-- consecutive_*), vì đó là cùng một bài toán: một danh sách thứ có thể hỏng
-- tạm thời hoặc hỏng hẳn, cần xoay vòng và tự cách ly. Khác một điểm: ở đây có
-- cột `status` tường minh chứ không suy ra từ các cột kia, vì giao diện hiện
-- một nhãn màu và người vận hành TẮT/BẬT tay được — hai thứ đó cần một giá trị
-- lưu trữ chứ không phải một biểu thức.
-- ---------------------------------------------------------------------------
CREATE TABLE scrape_via (
  id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  platform VARCHAR(20) NOT NULL,
  -- label là tên gợi nhớ nội bộ. CHECK chặn người dùng dán thẳng thông tin
  -- đăng nhập vào đây: label hiện ra khắp giao diện và đi vào log.
  label TEXT NOT NULL CHECK (btrim(label) <> ''),

  -- Cookies của phiên, AES-256-GCM (TOKEN_ENCRYPTION_KEY). Không bao giờ trả
  -- về trình duyệt sau khi lưu.
  cookies_encrypted TEXT NOT NULL,

  status VARCHAR(10) NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'cooldown', 'dead', 'disabled')),

  -- Đếm lỗi `login_required` LIÊN TIẾP. Reset về 0 mỗi lần dùng thành công —
  -- không reset thì một via thỉnh thoảng lỗi sẽ tích dần số đếm qua nhiều ngày
  -- rồi bị khai tử oan (đúng bài học của llm_api_key.consecutive_failures).
  consecutive_login_errors INT NOT NULL DEFAULT 0,
  cooldown_until           TIMESTAMPTZ,

  -- Hạn mức lượt quét/ngày. 1 lượt = 1 lần FetchLatestPosts HOÀN TẤT cho 1
  -- kênh, kể cả khi bên trong phải tải nhiều trang nối tiếp để đủ scan_limit.
  daily_quota INT NOT NULL DEFAULT 50 CHECK (daily_quota > 0),
  daily_used  INT NOT NULL DEFAULT 0 CHECK (daily_used >= 0),
  -- Ngày mà daily_used đang đếm cho. Có cột này thì việc reset TỰ LIỀN: lượt
  -- dùng đầu tiên của một ngày mới thấy ngày lệch là tự đặt lại. Không có nó
  -- thì một lần cron lỡ nhịp sẽ khoá toàn bộ via ở "hết hạn mức" cho tới khi
  -- có người để ý — mà đó đúng là lúc không ai để ý.
  daily_used_date DATE NOT NULL DEFAULT CURRENT_DATE,

  last_used_at  TIMESTAMPTZ,
  last_error_at TIMESTAMPTZ,
  last_error    TEXT,

  created_by UUID        NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Chỉ mục phục vụ ĐÚNG câu truy vấn chọn via: lọc theo nền tảng + trạng thái,
-- rồi lấy cái nghỉ lâu nhất.
CREATE INDEX idx_scrape_via_pick ON scrape_via(platform, status, last_used_at NULLS FIRST);
-- Job hồi sinh via hết cooldown: chỉ mục bộ phận nên nó luôn nhỏ bằng số via
-- đang nghỉ, không phải toàn bảng.
CREATE INDEX idx_scrape_via_cooldown ON scrape_via(cooldown_until) WHERE status = 'cooldown';

-- ---------------------------------------------------------------------------
-- scrape_proxy — lối ra mạng cho các request ở trên.
--
-- Vòng đời KHÁC via một điểm cốt lõi: proxy `dead` KHÔNG tự hồi sinh theo thời
-- gian. Via bị đòi đăng nhập thường là phiên hết hạn và có thể sống lại; còn
-- một IP đã bị Meta/X liệt thì chờ bao lâu cũng vậy — phải có người thay proxy
-- mới. Tự hồi sinh nó chỉ tạo ra một vòng lặp hỏng đều đặn.
-- ---------------------------------------------------------------------------
CREATE TABLE scrape_proxy (
  id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  label TEXT NOT NULL CHECK (btrim(label) <> ''),
  -- NULL = dùng chung cho mọi nền tảng. Đúng với gateway residential xoay IP
  -- theo request: nó không thuộc về nền tảng nào.
  platform VARCHAR(20),

  -- URL đầy đủ kèm user/pass, AES-256-GCM. Giao diện chỉ hiện phần đã che.
  endpoint_encrypted TEXT NOT NULL,
  -- Phần hiện được cho người vận hành nhận ra proxy nào là proxy nào, vd
  -- "http://gw.example.net:8000". Service tự cắt từ endpoint, không nhận từ
  -- client — để không có đường nào lộ user/pass qua chính cột này.
  endpoint_masked TEXT NOT NULL DEFAULT '',

  kind VARCHAR(12) NOT NULL DEFAULT 'residential'
    CHECK (kind IN ('residential', 'mobile', 'datacenter')),
  status VARCHAR(10) NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'degraded', 'dead', 'disabled')),

  -- Đếm `bot_block` LIÊN TIẾP: đây là lỗi của lối ra mạng, không phải của via.
  consecutive_blocks INT NOT NULL DEFAULT 0,
  -- Số lỗi trong NGÀY, để giao diện hiện tỉ lệ hỏng. Tự liền theo ngày như
  -- daily_used của via.
  errors_today INT  NOT NULL DEFAULT 0 CHECK (errors_today >= 0),
  used_today   INT  NOT NULL DEFAULT 0 CHECK (used_today >= 0),
  today        DATE NOT NULL DEFAULT CURRENT_DATE,

  last_used_at  TIMESTAMPTZ,
  last_error_at TIMESTAMPTZ,
  last_error    TEXT,

  created_by UUID        NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_scrape_proxy_pick ON scrape_proxy(status, last_used_at NULLS FIRST);

-- ---------------------------------------------------------------------------
-- via_usage_log — từng lượt dùng via, để trả lời "via nào đang hỏng ở kênh nào".
--
-- ĐÃ KIỂM TRA fetch_error_stat (migration 000017) TRƯỚC KHI TẠO BẢNG NÀY, đúng
-- như yêu cầu. Bảng đó gộp theo (ngày, nền tảng, loại lỗi) và KHÔNG có cột nào
-- trỏ tới via hay kênh — nó được dựng để trả lời "có đáng mua proxy không", một
-- câu hỏi ở mức nền tảng. Nó không thể quy lỗi về một via cụ thể, mà đó lại là
-- toàn bộ lý do tồn tại của bảng này. Hai bảng không trùng nhau:
--
--   fetch_error_stat — số đếm gộp, giữ mãi, cho biểu đồ lỗi theo nền tảng;
--   via_usage_log    — từng lượt, giữ ngắn ngày, cho việc gỡ lỗi một via.
--
-- Vẫn ghi cả lượt THÀNH CÔNG chứ không chỉ lỗi: tỉ lệ hỏng cần mẫu số, và biểu
-- đồ "số lượt quét theo giờ trong ngày" (kiểm tra lịch có bị dồn cục không) chỉ
-- dựng được từ các lượt chạy được.
--
-- Hai khoá ngoại kênh nullable + CHECK: giống hệt scan_run và source_post — một
-- lượt quét thuộc về đúng một loại kênh, và lượt thử tay không thuộc kênh nào.
-- ---------------------------------------------------------------------------
CREATE TABLE via_usage_log (
  id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  via_id UUID NOT NULL REFERENCES scrape_via(id) ON DELETE CASCADE,
  -- Proxy có thể NULL: một lượt chạy không qua proxy vẫn là một lượt.
  proxy_id UUID REFERENCES scrape_proxy(id) ON DELETE SET NULL,

  list_breaking_id  UUID REFERENCES list_breaking(id)  ON DELETE CASCADE,
  list_scheduled_id UUID REFERENCES list_scheduled(id) ON DELETE CASCADE,

  platform VARCHAR(20) NOT NULL,
  -- Trùng bộ nhãn của domain.FetchBlockKind, cộng thêm 'success'. Dùng lại
  -- đúng bộ chữ đó để một lỗi chỉ có một tên trên toàn hệ thống.
  result VARCHAR(20) NOT NULL
    CHECK (result IN ('success', 'bot_block', 'login_required', 'rate_limit',
                      'geo_blocked', 'unavailable', 'timeout', 'other')),
  -- Số bài lấy được, chỉ có nghĩa khi result = 'success'.
  posts_found INT NOT NULL DEFAULT 0,
  detail      TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT ck_via_usage_log_owner CHECK (
    NOT (list_breaking_id IS NOT NULL AND list_scheduled_id IS NOT NULL)
  )
);

CREATE INDEX idx_via_usage_log_via ON via_usage_log(via_id, created_at DESC);
CREATE INDEX idx_via_usage_log_time ON via_usage_log(created_at DESC);
