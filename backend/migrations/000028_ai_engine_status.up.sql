-- ---------------------------------------------------------------------------
-- Tình trạng API key TTS: còn hạn, hết credit, hay đã bị thu hồi.
--
-- Trước migration này, ba cái key ở ba tình trạng khác hẳn nhau trông y hệt
-- nhau trên màn AI Engine — cùng 4 ký tự cuối, cùng một dòng. Người dùng chỉ
-- biết key hết credit khi một voice chết, mà lúc đó đã mất một lần chạy và một
-- job phải làm lại.
--
-- Nguồn dữ liệu là LẦN GỌI TTS GẦN NHẤT, không phải một API thăm dò chạy nền:
-- 3voices tính tiền theo request, nên hỏi họ "key này còn sống không" mỗi vài
-- phút là tiêu tiền của người dùng để lấy một thông tin mà lần đọc thật sẽ nói
-- ra miễn phí. Đổi lại, cột này luôn mô tả QUÁ KHỨ — kèm key_status_at để
-- người đọc biết thông tin cũ tới mức nào.
--
--   key_status        — xem domain.KeyStatus cho ý nghĩa từng giá trị.
--   key_status_detail — nguyên văn câu 3voices trả về, để đối chiếu khi mở
--                       ticket với họ.
--   key_status_at     — lúc học được điều đó. NULL = chưa chạy lần nào.
--
-- Lỗi KHÔNG nói gì về key (mạng chập chờn, 3voices 5xx, text quá dài) không
-- đụng vào ba cột này: một lần rớt mạng không được phép xoá dấu vết của lần
-- 402 trước đó.
-- ---------------------------------------------------------------------------

ALTER TABLE ai_engine
  ADD COLUMN key_status        TEXT NOT NULL DEFAULT 'unknown'
             CHECK (key_status IN ('unknown','ok','invalid','no_credit','rate_limited')),
  ADD COLUMN key_status_detail TEXT,
  ADD COLUMN key_status_at     TIMESTAMPTZ;

-- Key đã từng đọc ra audio thì tại thời điểm đó nó còn sống — đó là tất cả
-- những gì ta biết về nó, và biết bấy nhiêu vẫn hơn hiện "chưa rõ" cho một key
-- vừa chạy hôm qua.
UPDATE ai_engine
SET key_status = 'ok', key_status_at = last_used_at
WHERE last_used_at IS NOT NULL;
