export type CollectMode = "A" | "B" | "C";
export type SourceType = "F1" | "BREAKING" | "SCHEDULED";
export type PostStatus = "new" | "processing" | "processed" | "failed";
/**
 * `processing`: record đã tạo, worker đang tải/tạo audio. Chưa có file nên
 * chưa sửa/đăng được — tồn tại để thấy voice ngay khi bấm tạo.
 */
export type PublishStatus = "processing" | "draft" | "ready" | "published" | "failed";

export interface Page<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

/**
 * Vai trò: admin (toàn quyền), editor (toàn quyền nghiệp vụ trừ phân quyền),
 * user (tạo/chạy/đăng voice, không xoá).
 */
export type Role = "admin" | "editor" | "user";

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

/** Hình thức thu thập kèm trạng thái bật/tắt và lý do nếu đang tắt. */
export interface CollectModeMeta {
  mode: CollectMode;
  enabled: boolean;
  /** Chỉ có khi `enabled` = false: vì sao mode này chưa dùng được. */
  reason?: string;
}

/** Điều kiện multime.ai đòi hỏi ở 1 bài đăng. */
export interface PublishRequirements {
  default_hashtags: string[];
  category_ids: number[];
  min_duration_seconds: number;
}

/**
 * Bài Post đã có trong hệ thống, trả kèm lỗi `duplicate_post` (HTTP 409) khi
 * tạo bài trùng ID bài đăng.
 */
export interface DuplicatePost {
  id: string;
  source_url: string;
  title?: string;
  status: PostStatus;
  collect_mode: CollectMode;
  created_at: string;
  post_id_extracted?: string;
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

  /**
   * Metadata gốc của bài, dùng auto-fill khi tạo Voice. `title` là TOÀN BỘ nội
   * dung bài (trừ hashtag) — hệ thống không còn trường mô tả riêng.
   */
  title: string | null;
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
  /** null = Voice gõ tay (không qua Bài Post) — xem `input_text`. */
  source_post_id: string | null;
  /** Đoạn text người dùng gõ; chỉ có ở Voice tạo thẳng từ text. */
  input_text: string | null;
  /** B/C của Voice gõ tay; null với Voice sinh từ Bài Post. */
  collect_mode: CollectMode | null;
  prompt_id: string | null;
  ai_engine_id: string | null;
  voice_file_url: string | null;
  duration_seconds: number | null;
  /** Nội dung Bài Post gộp về 1 dòng, cắt 200 ký tự — giới hạn của multime. */
  title: string | null;
  mime_type: string | null;
  size_bytes: number | null;
  sample_rate: number | null;
  hashtag: string | null;
  language: string;
  image_url: string | null;
  publish_status: PublishStatus;
  multime_post_url: string | null;
  last_error: string | null;
  created_by: string;
  created_at: string;
  published_at: string | null;

  /**
   * Các field dưới đây chỉ có trong danh sách (JOIN source_post + app_user).
   * Voice gõ tay không có Bài Post nên `platform`/`source_url` là null.
   */
  platform?: string | null;
  source_url?: string | null;
  source_title?: string | null;
  /** Hình thức/prompt của Bài Post nguồn — dùng làm mặc định khi tạo lại voice. */
  source_collect_mode?: CollectMode | null;
  source_prompt_id?: string | null;
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

/**
 * AIEngine thực chất là API key TTS của một người: chỉ còn 1 nhà cung cấp
 * (3voices) nên thứ cần quản lý là key của ai, không phải chọn engine nào.
 *
 * API không bao giờ trả key thật — chỉ `api_key_masked` (4 ký tự cuối).
 */
export interface AIEngine {
  id: string;
  /** Chủ sở hữu: voice của người này được đọc bằng key này. */
  user_id: string;
  user_email: string;
  /** Người khai key — khác chủ sở hữu khi admin khai hộ. */
  created_by: string;
  created_by_email: string;
  api_key_masked: string;
  created_at: string;
  /** Lần gần nhất key thật sự đọc ra audio; null = khai xong chưa dùng. */
  last_used_at: string | null;
}

export interface AuditLog {
  id: string;
  user_id: string;
  /** Email người thao tác (JOIN app_user); null nếu tài khoản không còn. */
  user_email: string | null;
  action: "create" | "update" | "delete" | "run" | "publish";
  object_type: string;
  object_id: string;
  changes: unknown;
  created_at: string;
}
