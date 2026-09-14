-- ---------------------------------------------------------------------------
-- Hashtag lấy từ MultiMe, không còn suy ra từ dữ liệu nội bộ.
--
-- Trên MultiMe, hashtag của voice CHÍNH LÀ `category` có `type = 'voice'`
-- (strongbody-api: entity.CategoryTypeVoice). Endpoint:
--
--   GET {auth_base}/v1/admin/category/list?type=voice&page=&limit=
--                                         &order_by=id&order_dir=ASC
--
-- Bảng cũ (dựng từ hashtag của Voice/Bài Post trong chính tool này) bị bỏ: nó
-- chỉ biết những tag hệ thống mình đã dùng, trong khi danh mục thật bên MultiMe
-- có ~94.000 tag.
--
-- KHÔNG CÓ QUAN HỆ HASHTAG ↔ NGÔN NGỮ. Đã kiểm tra tận entity: `Category`
-- không có trường ngôn ngữ nào, và category_translation_service.go ghi rõ
-- "Voice hashtags are not translated by this cron". Nên ô Hashtag hiển thị toàn
-- bộ danh mục, không lọc theo ngôn ngữ đang chọn — không bịa ra một quan hệ mà
-- nguồn dữ liệu không có.
-- ---------------------------------------------------------------------------

DROP TABLE IF EXISTS hashtag;

CREATE TABLE hashtag (
  -- id của category bên MultiMe: đây là bản sao có chủ, không phải thực thể
  -- riêng của tool.
  id              BIGINT PRIMARY KEY,
  name            TEXT NOT NULL,
  -- normalized_name là dạng đã chuẩn hoá của MultiMe, dùng để tìm kiếm.
  normalized_name TEXT NOT NULL DEFAULT '',
  slug            TEXT NOT NULL DEFAULT '',
  -- voice_tag_kind: system | user | campaign. `system` là tag do MultiMe tuyển
  -- chọn — đó là thứ nên hiện trước trong ô chọn, còn `user` là đuôi dài do
  -- người dùng bên đó tự đặt.
  voice_tag_kind  TEXT NOT NULL DEFAULT 'user',
  is_featured     BOOLEAN NOT NULL DEFAULT FALSE,
  ordering        BIGINT NOT NULL DEFAULT 0,
  synced_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Thứ tự hiển thị: tag tuyển chọn trước, rồi tới phần còn lại.
CREATE INDEX idx_hashtag_rank ON hashtag(is_featured DESC, voice_tag_kind, ordering, name);
-- Tìm kiếm gõ tới đâu lọc tới đó trên ~94k dòng: cần index cho LIKE 'abc%'.
CREATE INDEX idx_hashtag_search ON hashtag(normalized_name text_pattern_ops);
CREATE INDEX idx_hashtag_name ON hashtag(lower(name) text_pattern_ops);
