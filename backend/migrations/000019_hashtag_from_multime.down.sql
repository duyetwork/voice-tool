DROP TABLE IF EXISTS hashtag;

-- Dựng lại bảng cũ (hashtag suy ra từ dữ liệu nội bộ) để rollback không mất cấu trúc.
CREATE TABLE hashtag (
  tag          TEXT NOT NULL,
  language     TEXT NOT NULL DEFAULT '',
  usage_count  BIGINT NOT NULL DEFAULT 0,
  last_used_at TIMESTAMPTZ,
  refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tag, language)
);
CREATE INDEX idx_hashtag_lang ON hashtag(language, usage_count DESC);
