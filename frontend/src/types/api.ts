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
  /**
   * Mở nhóm "Vận hành" trên sidebar (nhật ký thao tác, cài đặt, via/proxy).
   * Admin và editor.
   *
   * Đi từ server chứ không suy từ `role` ở đây: luật phân quyền có đúng một
   * nơi định nghĩa, và khi nó đổi thì giao diện không phải sửa theo.
   */
  can_operate: boolean;
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

/**
 * Điều kiện multime.ai đòi hỏi ở 1 bài đăng.
 *
 * Không còn `default_hashtags`: hashtag mặc định đã bị bỏ khỏi cấu hình, mỗi
 * voice phải có hashtag của riêng nó thì mới đăng được.
 */
export interface PublishRequirements {
  category_ids: number[];
  min_duration_seconds: number;
}

/** Giới tính tài khoản Strongbody — đúng 3 giá trị strongbody-api nhận. */
export type Gender = "male" | "female" | "other";

/**
 * VoiceStyle — cấu hình giọng đọc gửi cho TTS.
 *
 * Giá trị là bộ từ khoá tiếng Anh của nhà cung cấp, KHÔNG phải nhãn hiển thị:
 * sai một chữ là hỏng cả request. Danh sách hợp lệ lấy từ `/meta/voice-style`
 * chứ không chép cứng ở đây — một danh sách nằm ở hai nơi thì sớm muộn cũng
 * lệch, và bên lệch sẽ là bên không validate.
 *
 * Mọi trường tuỳ chọn; để trống = theo mặc định của nhà cung cấp.
 */
export interface VoiceStyle {
  gender?: string;
  age?: string;
  pitch?: string;
  accent?: string;
  /** Tốc độ đọc, 1.0 là bình thường. */
  speed?: number;
}

/** Bộ giá trị hợp lệ cho VoiceStyle, do backend công bố. */
export interface VoiceStyleOptions {
  genders: string[];
  ages: string[];
  pitches: string[];
  accents: string[];
  speed: { min: number; max: number };
  /**
   * API key của người đang đăng nhập có khai sẵn một giọng đã lưu hay không.
   *
   * Có thì form phải nói trước: chỉnh cấu hình = bỏ giọng đó, sinh giọng mới
   * theo mô tả.
   */
  has_saved_voice: boolean;
}

/** Quốc gia trong danh mục Strongbody — dùng để lọc author. */
export interface Country {
  id: number;
  name: string;
  code?: string;
}

/**
 * 1 hashtag trong danh mục đồng bộ từ MultiMe.
 *
 * Bên MultiMe hashtag KHÔNG phải thực thể riêng: nó là `category` có
 * `type = 'voice'`. Vì thế mới có `slug`, `is_featured` — và vì thế **không có
 * ngôn ngữ**: entity Category không hề mang trường ngôn ngữ nào, nên không tồn
 * tại quan hệ hashtag ↔ ngôn ngữ để lọc theo.
 */
export interface Hashtag {
  /** id category bên MultiMe. */
  id: number;
  tag: string;
  slug: string;
  /** system | user | campaign — `system` là tag MultiMe tuyển chọn. */
  kind: string;
  is_featured: boolean;
}

/**
 * Catalog — 3 danh mục modal Tạo Voice cần, lấy 1 lần rồi cache.
 *
 * Gộp làm một response vì modal cần cả ba cùng lúc; và vì đã cache nên mở modal
 * lần sau không gọi API nữa, search chạy hoàn toàn trên dữ liệu này.
 */
export interface Catalog {
  countries: Country[];
  /**
   * PHẦN ĐẦU danh mục hashtag (tag MultiMe tuyển chọn), không phải toàn bộ —
   * danh mục bên đó có ~94.000 mục. Phần còn lại tìm qua `/meta/hashtags?q=`.
   */
  hashtags: Hashtag[];
  /** Mã ngôn ngữ theo thứ tự ưu tiên (suy ra từ thứ tự quốc gia ở backend). */
  language_order: string[];
}

/**
 * Author là tài khoản Strongbody đứng tên bài đăng trên multime.
 *
 * Không chọn đích danh: người dùng chỉ chọn giới tính, hệ thống bốc ngẫu nhiên
 * một tài khoản khớp (`/meta/authors/random`). Đây là tài khoản bên Strongbody,
 * không phải tài khoản của tool — người đăng và người đứng tên bài khác nhau.
 */
export interface Author {
  /** Chính là `author_id` gửi kèm khi đăng voice. */
  id: number;
  email: string;
  gender: Gender;
  full_name: string;
  avatar_url?: string;
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
  /**
   * URL của kênh đã lấy bài này về. Chỉ có trong danh sách (JOIN hai bảng
   * kênh). Rỗng = bài F1 nhập tay, không thuộc kênh nào. Kênh không có cột
   * tên nên URL chính là thứ nhận diện nó.
   */
  list_source_url?: string;
}

export interface Voice {
  id: string;
  /** null = Voice gõ tay (không qua Bài Post) — xem `input_text`. */
  source_post_id: string | null;
  /** Đoạn text người dùng gõ; chỉ có ở Voice tạo thẳng từ text. */
  input_text: string | null;
  /**
   * ĐÚNG đoạn chữ TTS đã đọc ra file audio này.
   *
   * Khác `input_text` ở hình thức C: ở đó `input_text` là ĐẦU VÀO của prompt,
   * còn cái được đọc là bản LLM viết lại. null với hình thức A (không đọc chữ
   * nào) và với voice tạo trước khi có cột này.
   */
  spoken_text: string | null;
  /** B/C của Voice gõ tay; null với Voice sinh từ Bài Post. */
  collect_mode: CollectMode | null;
  prompt_id: string | null;
  ai_engine_id: string | null;
  /** Bộ API key đã dùng để viết lại nội dung (hình thức C). */
  llm_api_set_id: string | null;
  /**
   * Model THẬT đã viết lại nội dung, vd "gemini:gemini-2.5-flash-lite".
   *
   * Không suy lại được từ `llm_api_set_id`: cùng một bộ, hôm nay chạy model rẻ
   * nhất, mai hết hạn mức thì chạy mắt xích sau.
   */
  llm_model_used: string | null;
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
  /** true = ảnh bìa nằm trong storage của tool (tải từ máy lên) và sẽ bị xoá sau khi đăng. */
  image_uploaded: boolean;
  /** Tài khoản Strongbody đứng tên bài đăng — bắt buộc trước khi đăng. */
  author_id: number | null;
  author_email: string | null;
  /** Giới tính đã bốc ra tài khoản đó, để hiện lại "Female - a@b.com". */
  author_gender: Gender | null;
  /**
   * Quốc gia lọc danh bạ lúc bốc author.
   *
   * Còn nguyên giá trị sau khi bài đã đăng: modal Sửa voice phải hiện lại đúng
   * nước người dùng đã chọn, không thì mỗi lần mở ra lại thành "tất cả".
   */
  author_country_id: number | null;
  /** Tạo xong audio thì tự đăng — bảng dựa vào đây để biết còn việc đang chạy. */
  publish_when_ready: boolean;
  /**
   * Cấu hình giọng đọc đã chốt cho voice này (mục "Cấu hình giọng đọc").
   *
   * null = giọng mặc định của nhà cung cấp. Modal Tạo lại đọc lại từ đây để
   * hiện đúng thứ người dùng đã chọn, thay vì mở ra là về mặc định.
   */
  tts_config: VoiceStyle | null;
  /** Người dùng chủ động chọn "không có ảnh" (khác với chưa có ảnh). */
  no_image: boolean;
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
  /**
   * Text worker đã đưa cho TTS đọc (với hình thức C là bản gốc trước khi LLM
   * viết lại). Là thứ điền sẵn vào ô "Nội dung đọc" — nghe thấy gì thì ô hiện
   * đúng cái đó.
   */
  source_extracted_text?: string | null;
  created_by_email?: string;
}

/**
 * Khung giờ quét của 1 kênh.
 *
 * Giờ đi qua API dưới dạng SỐ PHÚT TÍNH TỪ NỬA ĐÊM (0–1439) — khớp thẳng với
 * <input type="time"> sau khi tách, và khớp thẳng với cột trong DB, nên không
 * có chỗ nào phải parse chuỗi giờ.
 *
 * Bỏ trống cả `active_from_min` lẫn `active_to_min` = quét 24/7.
 */
export interface ChannelSchedule {
  /** Tên IANA, vd "Asia/Ho_Chi_Minh". */
  timezone: string;
  active_from_min: number | null;
  active_to_min: number | null;
  /** 0 = Chủ nhật … 6 = Thứ bảy. Rỗng = mọi ngày. */
  active_weekdays: number[] | null;
  /** Giờ chạy cố định — THAY THẾ "mỗi N phút". Chỉ Danh sách Định kỳ có. */
  fixed_times_min?: number[] | null;
  /** Xoá khung giờ đang đặt (chỉ gửi lên khi sửa). */
  clear_window?: boolean;
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
  /**
   * Tự đăng lên multime. KHÔNG còn ô riêng trên giao diện: nó đi theo
   * auto_process — tạo voice tự động mà không đăng thì bài nằm lại ở nháp và
   * vẫn phải vào bấm tay từng cái, tức là không tự động.
   */
  auto_publish: boolean;
  /**
   * Bốc tài khoản đứng tên bài đăng cho từng voice của kênh, lọc theo quốc gia
   * suy ra từ ngôn ngữ của kênh.
   *
   * Không bật thì voice của kênh không có author và hỏng ở bước đăng: không có
   * ai ngồi chọn tài khoản cho chúng, mà multime bắt buộc phải có.
   */
  random_author: boolean;
  status: string;
  scan_limit: number;
  scan_interval: PgInterval;
  /**
   * Số bài CŨ lấy về ở vòng quét đầu tiên. 0 = chỉ lấy bài đăng sau khi thêm
   * kênh — đây là mặc định.
   */
  backfill_limit: number;
  /** Mốc vòng quét đầu đã chạy xong. null = kênh chưa quét lần nào. */
  backfill_done_at: string | null;
  /** Trần Bài Post tạo ra mỗi vòng quét. null = không giới hạn. */
  max_posts_per_run: number | null;
  last_scanned_at: string | null;
  created_by: string;
  created_at: string;
  created_by_email?: string;
  /**
   * Lỗi của vòng quét gần nhất. null = vòng quét gần nhất chạy sạch.
   *
   * Cần vì quét chạy trong worker: không có cột này thì "kênh hỏng" và "kênh
   * khoẻ nhưng chưa có bài mới" trông giống hệt nhau trên bảng.
   */
  last_error: string | null;
  /** Số Bài Post kênh này đã lấy về. Chỉ có trong danh sách (subquery đếm). */
  post_count?: number;
  /** Số Voice đã tạo từ các Bài Post của kênh này. Chỉ có trong danh sách. */
  voice_count?: number;
  /** Bộ API key dùng cho hình thức C của kênh này. */
  llm_api_set_id: string | null;
  timezone: string;
  active_from_min: number | null;
  active_to_min: number | null;
  active_weekdays: number[] | null;
  /**
   * Quốc gia của kênh. Mọi Bài Post và Voice của kênh thuộc về quốc gia này;
   * null = suy từ ngôn ngữ như trước.
   */
  country_id: number | null;
  /**
   * Trạng thái vòng quét GẦN NHẤT: `running` = đang quét, `success` = vừa quét
   * xong, `error` = vòng vừa rồi hỏng, rỗng = chưa quét lần nào.
   *
   * Đọc từ bảng scan_run chứ không phải một cột trên kênh: cột như vậy phải
   * được xoá bởi chính tiến trình vừa chết, nên nó sẽ kẹt ở "đang quét" đúng
   * lúc cần tin nó nhất.
   */
  last_run_status?: string;
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
  /**
   * Tự đăng lên multime. KHÔNG còn ô riêng trên giao diện: nó đi theo
   * auto_process — tạo voice tự động mà không đăng thì bài nằm lại ở nháp và
   * vẫn phải vào bấm tay từng cái, tức là không tự động.
   */
  auto_publish: boolean;
  /**
   * Bốc tài khoản đứng tên bài đăng cho từng voice của kênh, lọc theo quốc gia
   * suy ra từ ngôn ngữ của kênh.
   *
   * Không bật thì voice của kênh không có author và hỏng ở bước đăng: không có
   * ai ngồi chọn tài khoản cho chúng, mà multime bắt buộc phải có.
   */
  random_author: boolean;
  status: string;
  scan_limit: number;
  max_posts_per_run: number | null;
  /**
   * Số bài CŨ lấy về ở vòng quét đầu tiên. 0 = chỉ lấy bài đăng sau khi thêm
   * kênh — đây là mặc định.
   */
  backfill_limit: number;
  /** Mốc vòng quét đầu đã chạy xong. null = kênh chưa quét lần nào. */
  backfill_done_at: string | null;
  last_scanned_at: string | null;
  created_by: string;
  created_at: string;
  created_by_email?: string;
  /**
   * Lỗi của vòng quét gần nhất. null = vòng quét gần nhất chạy sạch.
   *
   * Cần vì quét chạy trong worker: không có cột này thì "kênh hỏng" và "kênh
   * khoẻ nhưng chưa có bài mới" trông giống hệt nhau trên bảng.
   */
  last_error: string | null;
  /** Số Bài Post kênh này đã lấy về. Chỉ có trong danh sách (subquery đếm). */
  post_count?: number;
  /** Số Voice đã tạo từ các Bài Post của kênh này. Chỉ có trong danh sách. */
  voice_count?: number;
  llm_api_set_id: string | null;
  timezone: string;
  active_from_min: number | null;
  active_to_min: number | null;
  active_weekdays: number[] | null;
  fixed_times_min: number[] | null;
  /**
   * Quốc gia của kênh. Mọi Bài Post và Voice của kênh thuộc về quốc gia này;
   * null = suy từ ngôn ngữ như trước.
   */
  country_id: number | null;
  /**
   * Trạng thái vòng quét GẦN NHẤT: `running` = đang quét, `success` = vừa quét
   * xong, `error` = vòng vừa rồi hỏng, rỗng = chưa quét lần nào.
   *
   * Đọc từ bảng scan_run chứ không phải một cột trên kênh: cột như vậy phải
   * được xoá bởi chính tiến trình vừa chết, nên nó sẽ kẹt ở "đang quét" đúng
   * lúc cần tin nó nhất.
   */
  last_run_status?: string;
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
  /**
   * Key này có phải key ĐANG CHẠY của chủ sở hữu không.
   *
   * Mỗi người nhiều nhất 1 key bật — bật key này là tắt các key khác của cùng
   * người, backend ép bằng unique index chứ không chỉ bằng UI.
   */
  is_active: boolean;
  /**
   * Điều học được từ LẦN GỌI TTS GẦN NHẤT bằng key này. Xem KEY_STATUS_LABELS.
   *
   * Luôn là thông tin QUÁ KHỨ: hệ thống không thăm dò 3voices định kỳ (mỗi lần
   * hỏi là một request tính tiền), chỉ ghi lại những gì lần đọc thật nói ra.
   * Vì thế luôn đọc kèm `key_status_at`.
   */
  key_status: "unknown" | "ok" | "invalid" | "no_credit" | "rate_limited";
  /** Nguyên văn câu 3voices trả về, để đối chiếu khi mở ticket với họ. */
  key_status_detail: string | null;
  /** Lúc học được trạng thái đó; null = chưa chạy lần nào. */
  key_status_at: string | null;
  created_at: string;
  /** Lần gần nhất key thật sự đọc ra audio; null = khai xong chưa dùng. */
  last_used_at: string | null;
}

/** Nhà cung cấp LLM — đúng 3 giá trị backend chấp nhận. */
export type LLMProvider = "gemini" | "openai" | "anthropic";

/**
 * Sức khoẻ 1 API key LLM, do router ghi:
 *
 *   ok       — dùng được.
 *   cooldown — hết hạn mức, ĐANG NGHỈ và tự khỏi khi tới giờ.
 *   disabled — key sai hoặc bị thu hồi; chờ bao lâu cũng không tự khỏi, phải
 *              dán key mới.
 */
export type LLMKeyHealth = "ok" | "cooldown" | "disabled";

/** 1 API key trong bộ. API không bao giờ trả key thật — chỉ 4 ký tự cuối. */
export interface LLMAPIKey {
  id: string;
  set_id: string;
  provider: LLMProvider;
  label: string | null;
  /** Nhỏ hơn = router thử trước, trong cùng 1 nhà. */
  priority: number;
  api_key_masked: string;

  health: LLMKeyHealth;
  disabled_at: string | null;
  cooldown_until: string | null;
  consecutive_failures: number;
  last_used_at: string | null;
  last_error: string | null;
  created_at: string;
}

/** 1 người được dùng chung bộ API. */
export interface LLMAPISetUser {
  user_id: string;
  email: string;
}

/**
 * Bộ API key LLM: 1 túi key của NHIỀU nhà, dùng chung cho NHIỀU người.
 *
 * Khác hẳn AIEngine (1 key TTS của 1 người): chuỗi dự phòng chỉ có ý nghĩa khi
 * trong tay có key của nhiều nhà cùng lúc.
 */
export interface LLMAPISet {
  id: string;
  name: string;
  note: string | null;
  /** Toggle của admin: bộ này có hiện ra cho người khác chọn không. */
  visible_to_users: boolean;

  created_by: string;
  created_by_email: string;
  created_at: string;
  last_used_at: string | null;

  /** Số key theo từng nhà, vd { gemini: 2, openai: 1 }. */
  key_counts: Record<string, number>;
  users: LLMAPISetUser[];
  /** Bộ `visible_to_users` thì ai cũng THẤY nhưng chỉ chủ/admin mới SỬA. */
  can_manage: boolean;

  /** Chỉ có khi mở 1 bộ ra (GET /llm-api-sets/:id). */
  keys?: LLMAPIKey[];
}

/** 1 mắt xích của chuỗi dự phòng: gọi model này, của nhà này. */
export interface LLMChainStep {
  provider: LLMProvider;
  model: string;
}

/** Gộp N mẩu text vào 1 request để giảm chi phí và số lần gọi. */
export interface LLMBatchConfig {
  enabled: boolean;
  /** Số mẩu mỗi request. */
  size: number;
  /** Chặn trên theo ký tự — 5 bài dài vẫn có thể quá ngưỡng dù đúng `size`. */
  max_chars: number;
  /** Chờ bao lâu để gom đủ mẩu trước khi gửi đi. */
  wait_ms: number;
}

/**
 * Cấu hình chung (màn Cài đặt, chỉ admin).
 *
 * `allowed_models` do backend trả về chứ không phải hằng số trong code FE: giữ
 * một bản sao ở đây thì nó lệch với backend ngay ở lần thêm model tiếp theo.
 */
export interface Settings {
  llm_chain: LLMChainStep[];
  llm_batch: LLMBatchConfig;
  allowed_models: Record<LLMProvider, string[]>;
  providers: LLMProvider[];
  ai_prices: AIPrice[];
}

/** Loại dịch vụ AI bị tính tiền — ba loại, ba đơn vị tính khác nhau. */
export type AIKind = "llm" | "tts" | "stt";

/**
 * Đơn giá của 1 (loại, nhà cung cấp, model), do admin tự khai theo hoá đơn.
 *
 * Không có bảng giá mặc định trong code: giá đổi vài lần một năm và khác nhau
 * theo hợp đồng, nên một con số ghim sẵn sẽ sai mà không báo lỗi.
 */
export interface AIPrice {
  kind: AIKind;
  provider: string;
  model: string;
  /** LLM: USD cho 1 triệu token. */
  input_per_mtok: number;
  output_per_mtok: number;
  /** TTS: USD cho 1 triệu ký tự. */
  per_mchars: number;
  /** STT: USD cho 1 phút audio. */
  per_minute: number;
}

export interface AIUsageRow {
  day: string;
  kind: AIKind;
  provider: string;
  model: string;
  calls: number;
  failed: number;
  input_tokens: number;
  output_tokens: number;
  characters: number;
  audio_seconds: number;
  cost_usd: number;
  /** false = chưa khai đơn giá, `cost_usd` KHÔNG có ý nghĩa (khác "miễn phí"). */
  has_price: boolean;
}

export type AIUsageModel = Omit<AIUsageRow, "day">;

export interface AIUsageReport {
  days: number;
  daily: AIUsageRow[];
  models: AIUsageModel[];
  /** Chỉ cộng phần ĐÃ có đơn giá — xem `missing_prices`. */
  total_usd: number;
  /** Đã dùng thật nhưng chưa khai giá. Dùng để dựng sẵn form khai giá. */
  missing_prices: AIPrice[];
  prices: AIPrice[];
}

/**
 * Những thứ đang hỏng âm thầm.
 *
 * `users_need_relogin` là rủi ro nặng nhất: token multime của người tạo kênh
 * hết hạn thì mọi auto-publish của các kênh đó fail, và chỉ chính họ đăng nhập
 * lại mới sửa được.
 */
export interface Health {
  failed_voices: number;
  users_need_relogin: number;
  channels_with_error: number;
  ok: boolean;
}

/**
 * Số lần 1 nền tảng chặn ta trong 1 ngày.
 *
 * Đây là dữ liệu để trả lời đúng một câu hỏi: có đáng mua proxy không. Chỉ
 * `bot_block` và `rate_limit` là hai loại proxy giải quyết được; `login_required`
 * thì phải có cookies chứ proxy không giúp gì.
 */
export interface FetchErrorStat {
  day: string;
  platform: string;
  kind:
    | "bot_block"
    | "login_required"
    | "rate_limit"
    | "geo_blocked"
    | "unavailable"
    | "timeout"
    | "other";
  count: number;
  last_at: string;
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

/**
 * 1 bài mà vòng quét Breaking đã xét rồi BỎ.
 *
 * `text_excerpt` là chính đoạn text đã đem so với regex — không có nó thì log
 * chỉ là danh sách ID và không ai kết luận được regex quá chặt hay bài thật sự
 * không liên quan. Bản ghi cũ (trước migration 000024) không có URL/excerpt.
 */
export interface SkippedLog {
  post_id_external: string;
  post_url: string;
  text_excerpt: string;
  reason: string;
  checked_at: string;
}

export interface SkippedLogPage {
  items: SkippedLog[];
  total: number;
  /** Số bài bỏ qua trong 7 ngày gần nhất — cửa sổ cố định để so giữa các kênh. */
  last_7_days: number;
  limit: number;
  offset: number;
}

/**
 * ScanRun — MỘT vòng quét đã chạy của một kênh.
 *
 * Kênh chỉ mang được trạng thái của vòng gần nhất (`last_scanned_at`,
 * `last_error`), và vòng sau ghi đè vòng trước. Bảng này là chỗ duy nhất trả
 * lời được "kênh này quét bao giờ, ai cho chạy, mấy vòng vừa rồi có ra gì".
 */
export interface ScanRun {
  id: string;
  started_at: string;
  /** null = vòng vẫn đang chạy. */
  finished_at: string | null;
  /** `running` | `success` | `error`. */
  status: string;
  /** `auto` = lịch chạy, `manual` = có người bấm nút. */
  trigger_kind: string;
  /**
   * Rỗng ở vòng tự động, và cũng rỗng khi tài khoản đã bị xoá — `trigger_kind`
   * mới là thứ phân biệt hai trường hợp đó.
   */
  triggered_by_email: string;
  /**
   * Số bài vòng này XIN nền tảng, sau khi đã ép về trần của nền tảng đó.
   * 0 = dòng lịch sử có từ trước khi cột này tồn tại.
   */
  requested_limit: number;
  /** Số bài vòng này thật sự đem ra xét. */
  fetched: number;
  posts_created: number;
  voices_created: number;
  skipped: number;
  error: string;
}

export interface ScanHistory {
  items: ScanRun[];
  total: number;
  /** Tổng kết 7 ngày qua — cửa sổ cố định để so được giữa các kênh. */
  runs_7d: number;
  posts_created_7d: number;
  voices_created_7d: number;
  failed_7d: number;
  limit: number;
  offset: number;
}

// ---------------------------------------------------------------------------
// Hạ tầng via/proxy quét Facebook / X / Instagram
// ---------------------------------------------------------------------------
//
// VÌ SAO CÓ: ba nền tảng này không liệt kê được bài của một trang qua yt-dlp, và
// kênh nguồn là trang của NGƯỜI KHÁC nên không có API chính thức nào dùng được.
//
// Rủi ro đi kèm, người vận hành cần biết: cách làm này vi phạm điều khoản sử
// dụng của cả ba nền tảng, mang rủi ro pháp lý về scraping, và chi phí thật của
// nó là via chết liên tục phải thay.

/**
 * Trạng thái một via.
 *
 * `cooldown` và `dead` là KẾT LUẬN của hệ thống, không đặt tay được; `disabled`
 * mới là công tắc của người vận hành. Gộp `dead` với `disabled` thì một lần tắt
 * tay trông giống một lần via chết, và bảng tổng quan đếm sai.
 */
export type ViaStatus = "active" | "cooldown" | "dead" | "disabled";

/** Không có `cooldown`: IP bị nền tảng liệt thì chờ không khỏi. */
export type ProxyStatus = "active" | "degraded" | "dead" | "disabled";

export type ProxyKind = "residential" | "mobile" | "datacenter";

/** KHÔNG có trường nào chứa cookies — chúng không bao giờ rời khỏi server. */
export interface Via {
  id: string;
  platform: string;
  label: string;
  status: ViaStatus;
  /** CHỦ SỞ HỮU — người thấy và sửa được via này. Admin thấy của mọi người. */
  user_id: string;
  user_email: string;
  /** NGƯỜI KHAI — khác chủ sở hữu khi admin thêm hộ. */
  created_by: string;
  created_by_email: string;
  /** Số lỗi "đòi đăng nhập" liên tiếp; đủ ngưỡng thì via vào cooldown. */
  consecutive_login_errors: number;
  cooldown_until: string | null;
  daily_quota: number;
  daily_used: number;
  /** Tính ở server: bộ đếm tự liền theo ngày nên trình duyệt trừ tay sẽ sai. */
  quota_left: number;
  last_used_at: string | null;
  last_error_at: string | null;
  last_error: string;
  created_at: string;
}

/** `endpoint` đã che user/pass — server cắt, client không bao giờ thấy bản đầy đủ. */
export interface ScrapeProxy {
  id: string;
  label: string;
  /** Rỗng = dùng chung mọi nền tảng (gateway residential xoay IP). */
  platform: string;
  endpoint: string;
  kind: ProxyKind;
  status: ProxyStatus;
  user_id: string;
  user_email: string;
  created_by: string;
  created_by_email: string;
  consecutive_blocks: number;
  used_today: number;
  errors_today: number;
  last_used_at: string | null;
  last_error: string;
  created_at: string;
}

/** Số via theo trạng thái, cho mỗi nền tảng cần via. */
export interface ViaHealth {
  platform: string;
  active: number;
  cooldown: number;
  dead: number;
  disabled: number;
  total: number;
  /**
   * Tỉ lệ via còn dùng được (0..1). Mẫu số KHÔNG tính via bị tắt tay — tắt bớt
   * vài via không phải là dấu hiệu hệ thống đang hỏng.
   */
  healthy: number;
}

/**
 * Hướng dẫn dán cookies cho một nền tảng.
 *
 * Đi từ backend chứ không chép cứng ở đây: server mới là bên từ chối khi thiếu
 * cookie, nên form phải nói đúng cái server kiểm. Hai nơi giữ hai bản thì sớm
 * muộn form hướng dẫn một đằng, API chặn một nẻo.
 */
export interface ViaCookieSpec {
  platform: string;
  /** Cookie mang danh tính — thiếu là bị từ chối. */
  required: string[];
  /** Mẫu để dán: đúng hình dạng thật, giá trị là giả. */
  example: string;
  hint: string;
}

/** Một cột của biểu đồ "số lượt quét theo giờ" — để xem lịch có bị dồn cục không. */
export interface ScrapeHourRow {
  hour: number;
  platform: string;
  total: number;
  failed: number;
}
