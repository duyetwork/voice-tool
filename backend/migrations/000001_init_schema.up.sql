CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- app_user: cần cho JWT auth + cột created_by của các entity khác.
-- ---------------------------------------------------------------------------
CREATE TABLE app_user (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  full_name     TEXT,
  is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Danh mục: Prompt mẫu (Mode C) + AI Engine (TTS)
-- ---------------------------------------------------------------------------
CREATE TABLE prompt (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name       TEXT NOT NULL,
  content    TEXT NOT NULL,
  created_by UUID        NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_engine (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                TEXT        NOT NULL,
  provider            VARCHAR(50) NOT NULL,
  supported_languages TEXT[]      NOT NULL DEFAULT '{}',
  is_active           BOOLEAN     NOT NULL DEFAULT TRUE,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Tầng 1: Danh sách kênh — Breaking (F2) và Định kỳ (F3) tách biệt (specs 0.1)
-- ---------------------------------------------------------------------------
CREATE TABLE list_breaking (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_url       TEXT        NOT NULL,
  platform         VARCHAR(50) NOT NULL,
  content_type     VARCHAR(50),
  collect_mode     CHAR(1)     NOT NULL CHECK (collect_mode IN ('A','B','C')),
  prompt_id        UUID REFERENCES prompt(id),
  -- Regex là cơ chế nhận diện duy nhất (business rule #3).
  regex_pattern    TEXT        NOT NULL,
  language_default VARCHAR(10) NOT NULL,
  auto_process     BOOLEAN     NOT NULL DEFAULT TRUE,
  auto_publish     BOOLEAN     NOT NULL DEFAULT FALSE,
  status           VARCHAR(20) NOT NULL DEFAULT 'active',
  created_by       UUID        NOT NULL REFERENCES app_user(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Mode C bắt buộc có prompt.
  CONSTRAINT ck_list_breaking_prompt CHECK (collect_mode <> 'C' OR prompt_id IS NOT NULL)
);

CREATE TABLE list_scheduled (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_url          TEXT        NOT NULL,
  platform            VARCHAR(50) NOT NULL,
  content_type        VARCHAR(50),
  collect_mode        CHAR(1)     NOT NULL CHECK (collect_mode IN ('A','B','C')),
  prompt_id           UUID REFERENCES prompt(id),
  -- Tần suất quét riêng từng kênh (business rule #5).
  scan_frequency      INTERVAL    NOT NULL,
  language_default    VARCHAR(10) NOT NULL,
  last_synced_post_id TEXT,
  auto_process        BOOLEAN     NOT NULL DEFAULT TRUE,
  auto_publish        BOOLEAN     NOT NULL DEFAULT FALSE,
  status              VARCHAR(20) NOT NULL DEFAULT 'active',
  created_by          UUID        NOT NULL REFERENCES app_user(id),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ck_list_scheduled_prompt CHECK (collect_mode <> 'C' OR prompt_id IS NOT NULL)
);

-- ---------------------------------------------------------------------------
-- Tầng 2: Bài Post — điểm neo bắt buộc của mọi Voice (business rule #1)
-- ---------------------------------------------------------------------------
CREATE TABLE source_post (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_type       VARCHAR(20) NOT NULL CHECK (source_type IN ('F1','BREAKING','SCHEDULED')),
  list_breaking_id  UUID REFERENCES list_breaking(id) ON DELETE SET NULL,
  list_scheduled_id UUID REFERENCES list_scheduled(id) ON DELETE SET NULL,
  source_url        TEXT        NOT NULL,
  platform          VARCHAR(50) NOT NULL,
  content_type      VARCHAR(50),
  post_id_extracted TEXT,
  extracted_text    TEXT,
  collect_mode      CHAR(1)     NOT NULL CHECK (collect_mode IN ('A','B','C')),
  prompt_id         UUID REFERENCES prompt(id),
  -- Ngôn ngữ đã resolve theo cascade Post > List > hệ thống (business rule #9).
  language          VARCHAR(10) NOT NULL,
  status            VARCHAR(20) NOT NULL DEFAULT 'new'
                    CHECK (status IN ('new','processing','processed','failed')),
  last_error        TEXT,
  created_by        UUID        NOT NULL REFERENCES app_user(id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ck_source_post_prompt CHECK (collect_mode <> 'C' OR prompt_id IS NOT NULL),
  -- F1 không thuộc list nào; BREAKING/SCHEDULED phải trỏ đúng 1 list.
  CONSTRAINT ck_source_post_origin CHECK (
    (source_type = 'F1'        AND list_breaking_id IS NULL     AND list_scheduled_id IS NULL) OR
    (source_type = 'BREAKING'  AND list_breaking_id IS NOT NULL AND list_scheduled_id IS NULL) OR
    (source_type = 'SCHEDULED' AND list_breaking_id IS NULL     AND list_scheduled_id IS NOT NULL)
  )
);

-- ---------------------------------------------------------------------------
-- Tầng 3: Voice
-- ---------------------------------------------------------------------------
CREATE TABLE voice (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_post_id UUID NOT NULL REFERENCES source_post(id) ON DELETE CASCADE,
  ai_engine_id   UUID REFERENCES ai_engine(id),
  -- Chỉ tồn tại khi chưa publish; xoá sau khi publish thành công (business rule #2).
  voice_file_url   TEXT,
  duration_seconds INT,
  description      TEXT,
  hashtag          TEXT,
  language         VARCHAR(10) NOT NULL,
  image_url        TEXT,
  publish_status   VARCHAR(20) NOT NULL DEFAULT 'draft'
                   CHECK (publish_status IN ('draft','ready','published','failed')),
  multime_post_url TEXT,
  last_error       TEXT,
  created_by       UUID        NOT NULL REFERENCES app_user(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at     TIMESTAMPTZ,
  -- Đã publish thì bắt buộc có URL multime và không còn file nội bộ.
  CONSTRAINT ck_voice_published CHECK (
    publish_status <> 'published'
    OR (multime_post_url IS NOT NULL AND voice_file_url IS NULL)
  )
);

-- ---------------------------------------------------------------------------
-- Log
-- ---------------------------------------------------------------------------
CREATE TABLE skipped_log (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  list_breaking_id UUID        NOT NULL REFERENCES list_breaking(id) ON DELETE CASCADE,
  post_id_external TEXT        NOT NULL,
  reason           TEXT        NOT NULL,
  checked_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only (specs 1.6): không UPDATE/DELETE bản ghi audit.
CREATE TABLE audit_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID        NOT NULL REFERENCES app_user(id),
  action      VARCHAR(20) NOT NULL CHECK (action IN ('create','update','delete','run','publish')),
  object_type VARCHAR(30) NOT NULL,
  object_id   UUID        NOT NULL,
  changes     JSONB,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_source_post_status     ON source_post(status);
CREATE INDEX idx_source_post_created_at ON source_post(created_at DESC);
CREATE INDEX idx_voice_publish_status   ON voice(publish_status);
CREATE INDEX idx_voice_source_post      ON voice(source_post_id);
CREATE INDEX idx_audit_object           ON audit_log(object_type, object_id);
CREATE INDEX idx_skipped_log_list       ON skipped_log(list_breaking_id, checked_at DESC);

-- Chống lấy lặp (business rule #6): 1 bài ngoài chỉ vào hệ thống 1 lần / mỗi list.
CREATE UNIQUE INDEX uq_source_post_scheduled_dedup
  ON source_post(list_scheduled_id, post_id_extracted)
  WHERE list_scheduled_id IS NOT NULL AND post_id_extracted IS NOT NULL;
CREATE UNIQUE INDEX uq_source_post_breaking_dedup
  ON source_post(list_breaking_id, post_id_extracted)
  WHERE list_breaking_id IS NOT NULL AND post_id_extracted IS NOT NULL;
