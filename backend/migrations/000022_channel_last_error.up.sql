-- ---------------------------------------------------------------------------
-- Lỗi của vòng quét gần nhất, ghi lên chính kênh.
--
-- Vấn đề: vòng quét chạy trong worker, nên khi nó hỏng thì câu trả lời "vì sao
-- kênh này không ra bài nào" chỉ nằm trong log container. Trên giao diện kênh
-- vẫn "Đang bật", vẫn có tần suất quét, và trông y hệt một kênh khoẻ mạnh chưa
-- có bài mới. Người dùng không có đường nào phân biệt hai trạng thái đó.
--
-- Ví dụ thật đang có: kênh X không quét được (yt-dlp không đọc được dòng thời
-- gian của X) — mỗi vòng một lần hỏng, ba lần retry, và bảng vẫn im lặng.
--
-- Xoá về NULL mỗi khi một vòng quét chạy xong không lỗi: lỗi cũ nằm lại sau khi
-- đã tự khỏi thì còn dẫn sai hơn là không hiện gì.
-- ---------------------------------------------------------------------------

ALTER TABLE list_scheduled ADD COLUMN last_error TEXT;
ALTER TABLE list_breaking  ADD COLUMN last_error TEXT;
