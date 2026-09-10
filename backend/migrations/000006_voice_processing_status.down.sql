ALTER TABLE voice DROP CONSTRAINT IF EXISTS voice_publish_status_check;

-- Record đang xử lý không có bản tương ứng ở sơ đồ trước -> coi như thất bại.
UPDATE voice SET publish_status = 'failed' WHERE publish_status = 'processing';

ALTER TABLE voice
  ADD CONSTRAINT voice_publish_status_check
  CHECK (publish_status IN ('draft', 'ready', 'published', 'failed'));
