-- ---------------------------------------------------------------------------
-- Bài bị bỏ qua: ghi thêm ĐỦ NGỮ CẢNH để người vận hành phán được.
--
-- Bảng skipped_log có từ migration đầu tiên nhưng chỉ lưu `post_id_external` —
-- một chuỗi ID trần. Câu hỏi mà bảng này sinh ra để trả lời là "regex của tôi
-- có quá chặt không", và không ai trả lời được câu đó khi chỉ nhìn thấy
-- "7300000000000000000".
--
-- Hai cột thêm vào:
--   post_url     mở được bài gốc để đối chiếu bằng mắt.
--   text_excerpt chính đoạn text đã đem so với regex — thứ duy nhất giải thích
--                được vì sao nó trượt. Cắt ngắn khi ghi (xem service.Scan):
--                một vòng quét bỏ qua vài trăm bài, lưu nguyên nội dung là làm
--                phình bảng log để đọc đúng dòng đầu.
--
-- Cả hai NULL được: bản ghi cũ không có, và một số nền tảng không trả URL.
-- ---------------------------------------------------------------------------

ALTER TABLE skipped_log ADD COLUMN post_url     TEXT;
ALTER TABLE skipped_log ADD COLUMN text_excerpt TEXT;
