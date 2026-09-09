export type CollectMode = "A" | "B" | "C";
export type SourceType = "F1" | "BREAKING" | "SCHEDULED";
export type PostStatus = "new" | "processing" | "processed" | "failed";
export type PublishStatus = "draft" | "ready" | "published" | "failed";

export interface Page<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface ItemList<T> {
  items: T[];
  limit: number;
  offset: number;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export type Role = "admin" | "user" | "viewer";

export interface User {
  id: string;
  email: string;
  full_name: string | null;
  avatar?: string | null;
  role: Role;
  is_active?: boolean;
  created_at?: string;
}

export interface Permissions {
  can_write: boolean;
  can_delete: boolean;
  can_manage_users: boolean;
}

export interface Me {
  id: string;
  role: Role;
  permissions: Permissions;
}

/** Nền tảng nguồn hệ thống nhận diện được. */
export type Platform = "youtube" | "facebook" | "tiktok" | "instagram" | "x";

/** Hình thức thu thập kèm trạng thái bật/tắt (B, C chưa hỗ trợ). */
export interface CollectModeMeta {
  mode: CollectMode;
  enabled: boolean;
}

/** Điều kiện multime.ai đòi hỏi ở 1 bài đăng. */
export interface PublishRequirements {
  default_hashtags: string[];
  category_ids: number[];
  min_duration_seconds: number;
}

export interface SourcePost {
  id: string;
  source_type: SourceType;
  list_breaking_id: string | null;
  list_scheduled_id: string | null;
  source_url: string;
  platform: string;
  content_type: string | null;
  post_id_extracted: string | null;
  extracted_text: string | null;
  collect_mode: CollectMode;
  prompt_id: string | null;
  language: string;
  status: PostStatus;
  last_error: string | null;
  created_by: string;
  created_at: string;

  /** Metadata gốc của bài, dùng auto-fill khi tạo Voice. */
  title: string | null;
  description: string | null;
  hashtags: string[];
  thumbnail_url: string | null;
  author_name: string | null;
  /** Ngày đăng của bài GỐC trên nền tảng (khác created_at). */
  posted_at: string | null;
  /** Chỉ có trong danh sách (JOIN app_user). */
  created_by_email?: string;
}

export interface Voice {
  id: string;
  source_post_id: string;
  ai_engine_id: string | null;
  voice_file_url: string | null;
  duration_seconds: number | null;
  title: string | null;
  mime_type: string | null;
  size_bytes: number | null;
  sample_rate: number | null;
  description: string | null;
  hashtag: string | null;
  language: string;
  image_url: string | null;
  publish_status: PublishStatus;
  multime_post_url: string | null;
  last_error: string | null;
  created_by: string;
  created_at: string;
  published_at: string | null;

  /** Các field dưới đây chỉ có trong danh sách (JOIN source_post + app_user). */
  platform?: string;
  source_url?: string;
  source_title?: string | null;
  created_by_email?: string;
}

export interface ListBreaking {
  id: string;
  source_url: string;
  platform: string;
  content_type: string | null;
  collect_mode: CollectMode;
  prompt_id: string | null;
  regex_patterns: string[];
  language_default: string;
  auto_process: boolean;
  auto_publish: boolean;
  status: string;
  scan_limit: number;
  scan_interval: PgInterval;
  last_scanned_at: string | null;
  created_by: string;
  created_at: string;
  created_by_email?: string;
}

/** Postgres INTERVAL do pgx trả về. */
export interface PgInterval {
  Microseconds: number;
  Days: number;
  Months: number;
  Valid: boolean;
}

export interface ListScheduled {
  id: string;
  source_url: string;
  platform: string;
  content_type: string | null;
  collect_mode: CollectMode;
  prompt_id: string | null;
  scan_frequency: PgInterval;
  language_default: string;
  last_synced_post_id: string | null;
  auto_process: boolean;
  auto_publish: boolean;
  status: string;
  scan_limit: number;
  max_posts_per_run: number | null;
  last_scanned_at: string | null;
  created_by: string;
  created_at: string;
  created_by_email?: string;
}

export interface Prompt {
  id: string;
  name: string;
  content: string;
  created_at: string;
}

export interface AIEngine {
  id: string;
  name: string;
  provider: string;
  supported_languages: string[];
  is_active: boolean;
}

export interface AuditLog {
  id: string;
  user_id: string;
  action: "create" | "update" | "delete" | "run" | "publish";
  object_type: string;
  object_id: string;
  changes: unknown;
  created_at: string;
}
