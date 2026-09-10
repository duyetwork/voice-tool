-- ---------------------------------------------------------------------------
-- Chống trùng theo ID BÀI ĐĂNG, không theo URL.
--
-- Cùng 1 bài có nhiều URL khác nhau, ví dụ:
--   facebook.com/watch/?ref=saved&v=2112466516312058
--   facebook.com/reel/2112466516312058
-- đều là id 2112466516312058. Dedup theo URL để lọt cả hai, ra 2 Bài Post và
-- 2 Voice cho cùng 1 nội dung.
--
-- Trước: dedup RIÊNG theo từng danh sách, và F1 (nhập tay) không dedup gì cả.
-- Sau: dedup theo (platform, post_id_extracted) trên TOÀN hệ thống.
--
-- Index chỉ phủ bài do quét tự động (F2/F3) — nơi trùng lặp luôn là lỗi và
-- không ai xác nhận được. Bài F1 nhập tay được kiểm tra ở tầng service để còn
-- hiện thông báo và cho người dùng chọn "vẫn tạo mới"; index cứng sẽ chặn mất
-- lựa chọn đó.
-- ---------------------------------------------------------------------------

DROP INDEX IF EXISTS uq_source_post_scheduled_dedup;
DROP INDEX IF EXISTS uq_source_post_breaking_dedup;

-- Dọn bài trùng đã lọt vào từ trước, giữ bản CŨ NHẤT (voice sinh ra từ bản đó
-- cũng theo nó nhờ ON DELETE CASCADE).
DELETE FROM source_post sp
WHERE sp.source_type <> 'F1'
  AND sp.post_id_extracted IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM source_post older
    WHERE older.platform = sp.platform
      AND older.post_id_extracted = sp.post_id_extracted
      AND older.source_type <> 'F1'
      AND (older.created_at, older.id) < (sp.created_at, sp.id)
  );

CREATE UNIQUE INDEX uq_source_post_dedup
  ON source_post(platform, post_id_extracted)
  WHERE post_id_extracted IS NOT NULL AND source_type <> 'F1';

-- Tra cứu trùng lúc tạo bài F1: cần cả bài F1 lẫn bài do quét.
CREATE INDEX idx_source_post_dedup_lookup
  ON source_post(platform, post_id_extracted)
  WHERE post_id_extracted IS NOT NULL;
