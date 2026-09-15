-- name: GetHealthCounters :one
-- Ba con số trả lời "hệ thống có đang hỏng âm thầm không", gộp trong 1 lượt đọc
-- vì chúng luôn được hỏi cùng nhau (thanh cảnh báo trên giao diện).
SELECT
  -- Voice kẹt ở trạng thái lỗi. Đây là TỒN ĐỌNG chứ không phải tốc độ: voice
  -- lỗi nằm lại cho tới khi có người xử lý, nên đếm tất cả chứ không cắt theo
  -- cửa sổ thời gian.
  (SELECT COUNT(*) FROM voice WHERE publish_status = 'failed')::bigint
    AS failed_voices,

  -- Người bị xoá token multime (refresh thất bại) MÀ đang đứng tên kênh đang
  -- bật. Chỉ nhóm này mới đáng báo: token của họ là thứ worker dùng để
  -- auto-publish, và hỏng thì mọi voice của các kênh đó fail âm thầm.
  (SELECT COUNT(*) FROM app_user u
    WHERE u.multime_refresh_token IS NULL
      AND (EXISTS (SELECT 1 FROM list_breaking b
                    WHERE b.created_by = u.id AND b.status = 'active')
        OR EXISTS (SELECT 1 FROM list_scheduled s
                    WHERE s.created_by = u.id AND s.status = 'active')))::bigint
    AS users_need_relogin,

  -- Kênh đang bật nhưng vòng quét gần nhất lỗi.
  ((SELECT COUNT(*) FROM list_breaking
     WHERE status = 'active' AND last_error IS NOT NULL)
   + (SELECT COUNT(*) FROM list_scheduled
       WHERE status = 'active' AND last_error IS NOT NULL))::bigint
    AS channels_with_error;
