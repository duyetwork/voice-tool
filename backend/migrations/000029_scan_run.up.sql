-- ---------------------------------------------------------------------------
-- Lịch sử quét: mỗi vòng quét của mỗi kênh là 1 dòng.
--
-- Vấn đề đang có: kênh chỉ mang được DUY NHẤT trạng thái của vòng gần nhất —
-- `last_scanned_at` và `last_error`. Cả hai đều bị vòng sau ghi đè, nên không
-- có cách nào trả lời những câu hỏi thường gặp nhất khi một kênh "trông vẫn
-- chạy mà chẳng ra bài":
--
--   * kênh này quét lúc nào, bao lâu một lần thật sự (khác với cấu hình)?
--   * vòng vừa rồi do lịch chạy hay do ai đó bấm "Quét thử"?
--   * mấy vòng trước có ra bài không, hay im lặng suốt từ hôm qua?
--
-- Đếm sẵn 4 con số ngay trên dòng chứ không suy ra từ source_post/voice: bài
-- có thể bị xoá sau đó, và một vòng quét không tạo bài nào vẫn là một sự kiện
-- có thật cần nhìn thấy.
--
-- `status='running'` vừa là dòng lịch sử vừa là TRẠNG THÁI ĐANG QUÉT hiện lên
-- bảng kênh. Một chỗ ghi, một chỗ đọc: không có cột "đang quét" nào trên kênh
-- để lệch với sự thật khi worker chết giữa chừng.
-- ---------------------------------------------------------------------------

CREATE TABLE scan_run (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  -- Đúng MỘT trong hai, giống ck_source_post_origin: một vòng quét thuộc về
  -- đúng một kênh, và kiểu kênh quyết định cách đọc mấy con số bên dưới.
  list_breaking_id  UUID REFERENCES list_breaking(id)  ON DELETE CASCADE,
  list_scheduled_id UUID REFERENCES list_scheduled(id) ON DELETE CASCADE,

  started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  status      VARCHAR(10) NOT NULL DEFAULT 'running'
              CHECK (status IN ('running', 'success', 'error')),

  -- AI QUÉT. Vòng theo lịch không có người đứng sau nên triggered_by NULL và
  -- trigger_kind='auto'; nút "Quét thử" trên giao diện ghi đúng người đã bấm.
  -- Hai cột chứ không chỉ một: triggered_by NULL còn xảy ra khi tài khoản bị
  -- xoá sau đó, và lúc đó vẫn phải phân biệt được "hệ thống chạy" với "một
  -- người đã bấm, giờ không còn tài khoản".
  trigger_kind VARCHAR(10) NOT NULL DEFAULT 'auto'
               CHECK (trigger_kind IN ('auto', 'manual')),
  triggered_by UUID REFERENCES app_user(id) ON DELETE SET NULL,

  -- fetched: số bài vòng này thật sự đem ra xét (đã trừ phần backfill bỏ qua,
  -- phần cũ hơn mốc đồng bộ, phần ngoài cửa sổ thời gian).
  fetched        INT NOT NULL DEFAULT 0,
  posts_created  INT NOT NULL DEFAULT 0,
  voices_created INT NOT NULL DEFAULT 0,
  skipped        INT NOT NULL DEFAULT 0,

  error TEXT,

  CONSTRAINT ck_scan_run_owner CHECK (
    (list_breaking_id IS NOT NULL AND list_scheduled_id IS NULL) OR
    (list_breaking_id IS NULL     AND list_scheduled_id IS NOT NULL)
  )
);

-- Hai chỉ mục riêng, mỗi loại kênh một cái: mọi truy vấn đều hỏi "lịch sử của
-- kênh này", và dòng mới nhất được đọc nhiều nhất (bảng kênh lấy vòng gần nhất
-- của từng kênh).
CREATE INDEX idx_scan_run_breaking ON scan_run(list_breaking_id, started_at DESC)
  WHERE list_breaking_id IS NOT NULL;
CREATE INDEX idx_scan_run_scheduled ON scan_run(list_scheduled_id, started_at DESC)
  WHERE list_scheduled_id IS NOT NULL;
-- Quét dọn vòng treo: chỉ mục bộ phận nên nó luôn nhỏ bằng số vòng ĐANG chạy,
-- không phải toàn bộ lịch sử.
CREATE INDEX idx_scan_run_running ON scan_run(started_at) WHERE status = 'running';
