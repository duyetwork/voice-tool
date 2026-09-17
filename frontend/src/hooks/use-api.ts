"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ApiError, api } from "@/lib/api";
import type {
  Author,
  Country,
  Gender,
  DuplicatePost,
  AIEngine,
  AuditLog,
  Catalog,
  ChannelSchedule,
  Hashtag,
  FetchErrorStat,
  LLMAPIKey,
  LLMAPISet,
  LLMProvider,
  Settings,
  CollectMode,
  CollectModeMeta,
  Platform,
  PublishRequirements,
  Me,
  ListBreaking,
  ListScheduled,
  Page,
  Prompt,
  AIUsageReport,
  Health,
  Role,
  ScanHistory,
  ScrapeHourRow,
  ScrapeProxy,
  Via,
  ViaCookieSpec,
  ViaHealth,
  SkippedLogPage,
  SourcePost,
  User,
  Voice,
  VoiceStyle,
  VoiceStyleOptions,
} from "@/types/api";

export const keys = {
  me: ["me"] as const,
  users: (filters: Record<string, unknown>) => ["users", filters] as const,
  sourcePosts: (filters: Record<string, unknown>) => ["source-posts", filters] as const,
  voices: (filters: Record<string, unknown>) => ["voices", filters] as const,
  breaking: (filters: Record<string, unknown>) => ["lists", "breaking", filters] as const,
  skippedLogs: (id: string, limit: number, offset: number) =>
    ["lists", "breaking", id, "skipped", limit, offset] as const,
  scanRuns: (kind: ChannelKind, id: string, limit: number, offset: number) =>
    ["lists", kind, id, "scans", limit, offset] as const,
  scheduled: (filters: Record<string, unknown>) => ["lists", "scheduled", filters] as const,
  prompts: (filters: Record<string, unknown>) => ["prompts", filters] as const,
  aiEngines: ["ai-engines"] as const,
  llmApiSets: ["llm-api-sets"] as const,
  llmApiSet: (id: string) => ["llm-api-sets", id] as const,
  settings: ["settings"] as const,
  fetchStats: (days: number) => ["settings", "fetch-stats", days] as const,
  vias: (platform: string) => ["settings", "vias", platform] as const,
  proxies: (platform: string) => ["settings", "proxies", platform] as const,
  scrapeHealth: ["settings", "scrape-health"] as const,
  viaCookieSpecs: ["settings", "via-cookie-specs"] as const,
  scrapeLoad: (days: number) => ["settings", "scrape-load", days] as const,
  aiUsage: (days: number) => ["settings", "ai-usage", days] as const,
  health: ["meta", "health"] as const,
  auditLog: (filters: Record<string, unknown>) => ["audit-log", filters] as const,
  platforms: ["meta", "platforms"] as const,
  publishMeta: ["meta", "publish"] as const,
  catalog: ["meta", "catalog"] as const,
  hashtags: (q: string) => ["meta", "hashtags", q] as const,

  collectModes: ["meta", "collect-modes"] as const,
  voiceStyleOptions: ["meta", "voice-style"] as const,
};

// ---------------------------------------------------------------------------
// Bài Post
// ---------------------------------------------------------------------------

/** Giá trị hợp lệ của query string. */
type QueryValue = string | number | boolean | undefined | null;

/** Chu kỳ tự làm mới khi còn việc đang chạy (ms). */
const POLL_MS = 4_000;

/**
 * pollWhile bật tự làm mới chỉ khi `pending` đúng với dữ liệu vừa nhận — hết
 * việc đang chạy thì dừng hẳn, không gọi API vô ích.
 */
function pollWhile<T>(pending: (data: T) => boolean) {
  return (query: { state: { data?: T } }) => {
    const data = query.state.data;
    return data !== undefined && pending(data) ? POLL_MS : false;
  };
}

export interface SourcePostFilters {
  [key: string]: QueryValue;
  source_type?: string;
  status?: string;
  platform?: string;
  collect_mode?: string;
  language?: string;
  created_by?: string;
  created_from?: string;
  created_to?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  dir?: string;
}

export function useSourcePosts(filters: SourcePostFilters = {}) {
  return useQuery({
    queryKey: keys.sourcePosts(filters),
    queryFn: () => api.get<Page<SourcePost>>("/source-posts", filters),
    refetchInterval: pollWhile((data: Page<SourcePost>) =>
      data.items.some((p) => p.status === "processing"),
    ),
  });
}

export interface CreateSourcePostInput {
  source_url: string;
  collect_mode: CollectMode;
  prompt_id?: string | null;
  language?: string;
  auto_process?: boolean;
  /** Để trống thì hệ thống tự nhận diện nền tảng từ URL. */
  platform?: string;
  /**
   * Bỏ qua kiểm tra trùng: chỉ đặt sau khi người dùng đã thấy thông báo trùng
   * và chọn vẫn tạo mới.
   */
  allow_duplicate?: boolean;
  /**
   * Metadata người dùng đã điền sẵn ở màn tạo Voice. Gửi kèm ngay từ đây thay
   * vì PATCH sau: bấm Đăng là đóng hộp thoại, worker tạo audio xong tự đăng —
   * không còn ai ngồi đó để sửa tiếp.
   */
  voice?: VoiceSeedInput;
}

/** VoiceSeedInput — mọi trường đều tuỳ chọn, để trống thì lấy từ bài gốc. */
export interface VoiceSeedInput {
  title?: string;
  /** Bộ API key viết lại nội dung — chỉ hình thức C mới cần. */
  llm_api_set_id?: string | null;
  /** Gộp với hashtag của bài gốc chứ không thay thế. */
  hashtag?: string;
  language?: string;
  image_url?: string;
  image_uploaded?: boolean;
  /** Chủ động chọn "không có ảnh" — khác với để trống (lấy ảnh bài gốc). */
  no_image?: boolean;
  author_id?: number | null;
  author_email?: string | null;
  author_gender?: Gender | null;
  /**
   * Quốc gia đã lọc lúc chọn author.
   *
   * Đi theo voice vì việc BỐC tài khoản diễn ra lúc worker đăng bài — lúc đó
   * không còn form nào để hỏi lại người dùng đã lọc theo quốc gia nào.
   */
  author_country_id?: number | null;
  /** Tạo xong audio thì đăng luôn lên multime. */
  publish_when_ready?: boolean;
  /** Cấu hình giọng đọc. Bỏ trống = giọng mặc định của nhà cung cấp. */
  tts_config?: VoiceStyle | null;
}

/**
 * duplicateOf đọc thông tin bài trùng ra khỏi lỗi 409 `duplicate_post`;
 * null nếu đây là lỗi khác.
 */
export function duplicateOf(error: unknown): DuplicatePost | null {
  if (!(error instanceof ApiError) || error.code !== "duplicate_post") return null;
  const existing = (error.payload as { existing?: DuplicatePost } | undefined)?.existing;
  return existing ?? null;
}

export function useCreateSourcePost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateSourcePostInput) => api.post<SourcePost>("/source-posts", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["source-posts"] });
      qc.invalidateQueries({ queryKey: ["voices"] });
    },
  });
}

export function useUpdateSourcePost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & Partial<SourcePost>) =>
      api.patch<SourcePost>(`/source-posts/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["source-posts"] }),
  });
}

export function useRunSourcePost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<{ status: string }>(`/source-posts/${id}/run`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["source-posts"] });
      qc.invalidateQueries({ queryKey: ["voices"] });
    },
  });
}

export function useDeleteSourcePost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/source-posts/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["source-posts"] }),
  });
}

// ---------------------------------------------------------------------------
// Voice
// ---------------------------------------------------------------------------

export interface VoiceFilters {
  [key: string]: QueryValue;
  publish_status?: string;
  source_post_id?: string;
  platform?: string;
  language?: string;
  created_by?: string;
  created_from?: string;
  created_to?: string;
  published_from?: string;
  published_to?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  dir?: string;
}

/**
 * useVoices tự làm mới khi trong trang còn việc đang chạy — worker xong là bảng
 * tự đổi trạng thái, không phải bấm "Làm mới".
 *
 * Ba trường hợp còn việc:
 *   - voice đang tạo audio (`processing`);
 *   - voice đặt đăng-ngay-khi-xong mà chưa tới đích (publish_when_ready);
 *   - voice người dùng vừa bấm Đăng — `pendingIds`, vì việc đăng KHÔNG đổi
 *     publish_status ngay: nó vào hàng đợi rồi vài giây sau mới thành
 *     `published`, nên nhìn vào dữ liệu hiện có thì không biết là đang có việc.
 */
export function useVoices(filters: VoiceFilters = {}, pendingIds: string[] = []) {
  const waiting = new Set(pendingIds);
  return useQuery({
    queryKey: keys.voices(filters),
    queryFn: () => api.get<Page<Voice>>("/voices", filters),
    refetchInterval: pollWhile((data: Page<Voice>) =>
      data.items.some(
        (v) =>
          v.publish_status === "processing" ||
          waiting.has(v.id) ||
          (v.publish_when_ready &&
            v.publish_status !== "published" &&
            v.publish_status !== "failed"),
      ),
    ),
  });
}

/**
 * CreateTextVoiceInput — tạo Voice thẳng từ text gõ tay, KHÔNG qua Bài Post.
 *
 * Bài lấy từ URL vẫn đi đường cũ (`useCreateSourcePost`): chỉ khi đó Voice mới
 * truy vết được về bài gốc. Text gõ tay không có bài gốc nào để truy vết.
 */
export interface CreateTextVoiceInput {
  text: string;
  /** Chỉ B (đọc nguyên văn) hoặc C (LLM viết lại theo Prompt rồi đọc). */
  collect_mode: Exclude<CollectMode, "A">;
  prompt_id?: string | null;
  language?: string;
  /** Bộ API key viết lại nội dung — chỉ hình thức C mới cần. */
  llm_api_set_id?: string | null;
  /**
   * Metadata điền sẵn — ĐÚNG bộ trường của CreateSourcePostInput.voice.
   *
   * Form tạo voice là một form cho cả ba hình thức, nên hai đường gửi phải nhận
   * cùng một bộ trường; lệch một trường là một thứ người dùng điền rồi mà biến
   * mất tuỳ hình thức họ chọn.
   */
  voice?: VoiceSeedInput;
}

export function useCreateTextVoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTextVoiceInput) => api.post<Voice>("/voices", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["voices"] }),
  });
}

/**
 * RegenerateVoiceInput — sửa lời đọc rồi tạo lại chính voice đó.
 *
 * Ghi đè lên bản ghi cũ (kể cả file audio), không tạo thêm dòng mới: đây là
 * "sửa lại cho đúng", còn tiêu đề/hashtag/ảnh bìa đã điền vẫn giữ nguyên.
 */
export interface RegenerateVoiceInput {
  /** Hình thức C: đầu vào của prompt. Hình thức B: chính là lời đọc. */
  text: string;
  collect_mode: Exclude<CollectMode, "A">;
  prompt_id?: string | null;
  language?: string;
  /** Bỏ trống = giữ bộ API voice đang dùng. */
  llm_api_set_id?: string | null;
  /**
   * Lời đọc người dùng tự chốt. Có giá trị thì lần này TTS đọc nguyên văn nó và
   * KHÔNG gọi LLM — dùng khi họ sửa tay bản LLM đã viết ra.
   */
  spoken_text?: string | null;
  /**
   * Cấu hình giọng đọc cho lần chạy này.
   *
   * Bỏ hẳn trường = giữ nguyên giọng voice đang dùng; gửi `{}` = về mặc định.
   */
  tts_config?: VoiceStyle | null;
}

export function useRegenerateVoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & RegenerateVoiceInput) =>
      api.post<Voice>(`/voices/${id}/regenerate`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["voices"] }),
  });
}

export function useUpdateVoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & Partial<Voice>) =>
      api.patch<Voice>(`/voices/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["voices"] }),
  });
}

/**
 * useUploadVoiceImage tải ảnh bìa từ máy lên cho 1 voice.
 *
 * Ảnh nằm trong storage của tool và bị xoá ngay sau khi đăng lên multime (bên
 * đó đã giữ một bản) — nên đây là file tạm, không phải kho ảnh.
 */
export function useUploadVoiceImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, file }: { id: string; file: File }) =>
      api.upload<Voice>(`/voices/${id}/image`, file),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["voices"] }),
  });
}

export function usePublishVoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<{ status: string }>(`/voices/${id}/publish`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["voices"] }),
  });
}

export function useDeleteVoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/voices/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["voices"] }),
  });
}

// ---------------------------------------------------------------------------
// Danh sách Breaking (F2)
// ---------------------------------------------------------------------------

export interface ChannelFilters {
  [key: string]: QueryValue;
  status?: string;
  search?: string;
  platform?: string;
  created_by?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  dir?: string;
}

export function useBreakingLists(filters: ChannelFilters = {}) {
  return useQuery({
    queryKey: keys.breaking(filters),
    queryFn: () => api.get<Page<ListBreaking>>("/lists/breaking", filters),
  });
}

export interface CreateBreakingInput {
  source_url: string;
  /** Nhiều pattern kết hợp OR — khớp 1 pattern là bắt bài. */
  regex_patterns: string[];
  collect_mode: CollectMode;
  prompt_id?: string | null;
  language_default?: string;
  auto_process?: boolean;
  auto_publish?: boolean;
  random_author?: boolean;
  scan_limit?: number;
  scan_interval?: string;
  /**
   * Số bài CŨ lấy về ở vòng quét đầu. Bỏ trống = 0 = chỉ lấy bài đăng sau khi
   * thêm kênh.
   */
  backfill_limit?: number;
  /** Trần Bài Post mỗi vòng quét. 0 = không giới hạn (mặc định). */
  max_posts_per_run?: number;
  /**
   * Bộ API key cho hình thức C. Quét tự động không có ai bấm nút để chọn bộ,
   * nên bộ phải nằm sẵn trên kênh.
   */
  llm_api_set_id?: string | null;
  /** Bỏ trống = quét 24/7 (hành vi cũ). */
  schedule?: ChannelSchedule;
  /** Quốc gia của kênh. 0 = gỡ quốc gia, quay lại suy từ ngôn ngữ. */
  country_id?: number;
}

/**
 * Sửa kênh: mọi trường tuỳ chọn, chỉ gửi thứ thật sự đổi.
 *
 * `backfill_done_at` bị loại: nó là mốc do vòng quét ghi, không phải thứ người
 * dùng đặt — gửi lên chỉ tổ để server phải bỏ qua.
 */
export type UpdateBreakingInput = Partial<
  Omit<
    ListBreaking,
    | "id"
    | "created_at"
    | "created_by"
    | "scan_interval"
    | "backfill_done_at"
    // last_run_status là thứ server tính, không phải thứ client đặt.
    | "last_run_status"
  >
> & {
  scan_interval?: string;
  schedule?: ChannelSchedule;
};

export function useCreateBreakingList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateBreakingInput) => api.post<ListBreaking>("/lists/breaking", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "breaking"] }),
  });
}

export function useUpdateBreakingList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateBreakingInput) =>
      api.patch<ListBreaking>(`/lists/breaking/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "breaking"] }),
  });
}

export function useRunBreakingList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<{ status: string }>(`/lists/breaking/${id}/run`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["source-posts"] });
      // Bảng kênh đọc trạng thái vòng quét gần nhất, nên nó phải đổi ngay sang
      // "Đang quét" sau khi bấm.
      qc.invalidateQueries({ queryKey: ["lists", "breaking"] });
    },
  });
}

/**
 * Bài kênh này đã xét rồi bỏ.
 *
 * Chỉ gọi khi tab được mở (`enabled`): phần lớn lần mở modal là để sửa regex,
 * và tải sẵn vài trăm dòng log cho mọi lần đó là trả tiền cho thứ không ai xem.
 */
export function useSkippedLogs(id: string, limit = 20, offset = 0, enabled = true) {
  return useQuery({
    queryKey: keys.skippedLogs(id, limit, offset),
    queryFn: () => api.get<SkippedLogPage>(`/lists/breaking/${id}/skipped`, { limit, offset }),
    enabled: enabled && Boolean(id),
  });
}

export function useDeleteBreakingList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/lists/breaking/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "breaking"] }),
  });
}

/** ChannelKind phân biệt hai bảng kênh trong các hook dùng chung. */
export type ChannelKind = "breaking" | "scheduled";

/**
 * Lịch sử quét của 1 kênh: vòng nào chạy lúc nào, ai cho chạy, ra bao nhiêu bài
 * và voice.
 *
 * Chỉ gọi khi tab được mở (`enabled`), cùng lý do với useSkippedLogs: phần lớn
 * lần mở modal là để sửa cấu hình.
 *
 * `refetchInterval` khi đang có vòng chạy: trạng thái "Đang quét" chỉ có ích
 * nếu nó tự đổi thành "Xong" — bắt người dùng bấm Làm mới để biết vòng quét đã
 * kết thúc chưa thì thà đừng hiện trạng thái đó.
 */
export function useScanRuns(kind: ChannelKind, id: string, limit = 20, offset = 0, enabled = true) {
  return useQuery({
    queryKey: keys.scanRuns(kind, id, limit, offset),
    queryFn: () => api.get<ScanHistory>(`/lists/${kind}/${id}/scans`, { limit, offset }),
    enabled: enabled && Boolean(id),
    refetchInterval: (query) =>
      query.state.data?.items.some((r) => r.status === "running") ? 5_000 : false,
  });
}

// ---------------------------------------------------------------------------
// Danh sách Định kỳ (F3)
// ---------------------------------------------------------------------------

export function useScheduledLists(filters: ChannelFilters = {}) {
  return useQuery({
    queryKey: keys.scheduled(filters),
    queryFn: () => api.get<Page<ListScheduled>>("/lists/scheduled", filters),
  });
}

export interface CreateScheduledInput {
  source_url: string;
  collect_mode: CollectMode;
  scan_frequency: string;
  prompt_id?: string | null;
  language_default?: string;
  auto_process?: boolean;
  auto_publish?: boolean;
  random_author?: boolean;
  scan_limit?: number;
  /** Trần Bài Post mỗi vòng quét. 0 = không giới hạn (mặc định). */
  max_posts_per_run?: number;
  /**
   * Số bài CŨ lấy về ở vòng quét đầu. Bỏ trống = 0 = chỉ lấy bài đăng sau khi
   * thêm kênh.
   */
  backfill_limit?: number;
  llm_api_set_id?: string | null;
  /**
   * Bỏ trống = quét 24/7 theo `scan_frequency`. Đặt `fixed_times_min` thì giờ
   * cố định THAY THẾ tần suất.
   */
  schedule?: ChannelSchedule;
  /** Quốc gia của kênh. 0 = gỡ quốc gia, quay lại suy từ ngôn ngữ. */
  country_id?: number;
}

export type UpdateScheduledInput = Partial<
  Omit<
    ListScheduled,
    "id" | "created_at" | "created_by" | "scan_frequency" | "backfill_done_at" | "last_run_status"
  >
> & {
  scan_frequency?: string;
  schedule?: ChannelSchedule;
};

export function useCreateScheduledList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateScheduledInput) => api.post<ListScheduled>("/lists/scheduled", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "scheduled"] }),
  });
}

export function useUpdateScheduledList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateScheduledInput) =>
      api.patch<ListScheduled>(`/lists/scheduled/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "scheduled"] }),
  });
}

/**
 * Quét thử 1 kênh Định kỳ. Đối xứng với useRunBreakingList, và cần vì cùng lý
 * do: kênh đặt tần suất 6 tiếng thì không có cách nào thử cấu hình vừa sửa
 * ngoài việc ngồi chờ.
 *
 * Làm mới cả bảng kênh, không chỉ Bài Post: vòng quét vừa đẩy vào hàng đợi sẽ
 * hiện lên đó thành "Đang quét".
 */
export function useRunScheduledList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<{ status: string }>(`/lists/scheduled/${id}/run`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["source-posts"] });
      qc.invalidateQueries({ queryKey: ["lists", "scheduled"] });
    },
  });
}

export function useDeleteScheduledList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/lists/scheduled/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "scheduled"] }),
  });
}

// ---------------------------------------------------------------------------
// Danh mục
// ---------------------------------------------------------------------------

export interface PromptFilters {
  [key: string]: QueryValue;
  limit?: number;
  offset?: number;
  sort?: string;
  dir?: string;
}

/**
 * usePrompts: mặc định lấy 200 prompt cho các ô chọn "Prompt mẫu"; bảng quản lý
 * prompt truyền limit/offset để phân trang thật.
 */
export function usePrompts(filters: PromptFilters = { limit: 200 }) {
  return useQuery({
    queryKey: keys.prompts(filters),
    queryFn: () => api.get<Page<Prompt>>("/prompts", filters),
  });
}

export function useCreatePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; content: string }) => api.post<Prompt>("/prompts", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["prompts"] }),
  });
}

/**
 * useUpdatePrompt — sửa Prompt mẫu tại chỗ.
 *
 * Sửa chứ không "xoá rồi thêm lại": prompt_id đang được các kênh và các voice
 * trỏ tới, xoá đi là cắt đứt hết những liên kết đó rồi phải đi gán lại từng cái.
 */
export function useUpdatePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; content?: string }) =>
      api.patch<Prompt>(`/prompts/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["prompts"] }),
  });
}

export function useDeletePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/prompts/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["prompts"] }),
  });
}

/**
 * CreateAIEngineInput — payload thêm API key TTS.
 *
 * `user_ids` chỉ admin gửi (gán key cho người khác, nhiều người một lượt —
 * mỗi người nhận 1 bản ghi riêng). Bỏ trống = key của chính mình; backend ép
 * điều này với mọi vai trò khác, không phụ thuộc UI.
 */
export interface CreateAIEngineInput {
  api_key: string;
  user_ids?: string[];
}

/**
 * UpdateAIEngineInput — payload sửa API key TTS.
 *
 * `api_key` bỏ trống nghĩa là GIỮ key cũ: form không hiển thị key thật nên
 * không có gì để gửi lại. `user_id` là gán key sang người khác — chỉ admin.
 */
export interface UpdateAIEngineInput {
  api_key?: string;
  user_id?: string;
  /**
   * Bật/tắt key. Bỏ trống = giữ nguyên, nên công tắc ở bảng gửi đúng một
   * trường này — không gửi kèm key hay chủ sở hữu thì không có gì để ghi nhầm.
   */
  is_active?: boolean;
}

/**
 * Danh sách key: admin nhận key của mọi người, các vai trò khác chỉ nhận key
 * của chính mình — backend ép, không phụ thuộc tham số.
 */
export function useAIEngines() {
  return useQuery({
    queryKey: keys.aiEngines,
    queryFn: () => api.get<{ items: AIEngine[] }>("/ai-engines"),
  });
}

/** Trả về mảng: admin gán 1 key cho nhiều người thì mỗi người là 1 bản ghi. */
export function useCreateAIEngine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateAIEngineInput) =>
      api.post<{ items: AIEngine[] }>("/ai-engines", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.aiEngines }),
  });
}

export function useUpdateAIEngine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateAIEngineInput) =>
      api.patch<AIEngine>(`/ai-engines/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.aiEngines }),
  });
}

export function useDeleteAIEngine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/ai-engines/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.aiEngines }),
  });
}

// ---------------------------------------------------------------------------
// Bộ API key LLM (tab "LLM Model")
// ---------------------------------------------------------------------------

/** 1 key gửi lên khi tạo bộ hoặc thêm vào bộ. */
export interface LLMAPIKeyInput {
  provider: LLMProvider;
  api_key: string;
  label?: string;
  priority?: number;
}

export interface CreateLLMAPISetInput {
  name: string;
  note?: string;
  /** Chỉ admin đặt được — backend ép người khác về false. */
  visible_to_users?: boolean;
  /** Chia sẻ bộ cho ai. */
  user_ids?: string[];
  keys?: LLMAPIKeyInput[];
}

export interface UpdateLLMAPISetInput {
  name?: string;
  note?: string;
  visible_to_users?: boolean;
  /** Gửi lên đúng những ai được tick: bỏ tick = gỡ quyền. */
  user_ids?: string[];
}

/** `api_key` bỏ trống = giữ key cũ (và không reset sức khoẻ key). */
export interface UpdateLLMAPIKeyInput {
  provider?: LLMProvider;
  api_key?: string;
  label?: string;
  priority?: number;
}

/**
 * Danh sách bộ API: backend quyết định ai thấy bộ nào (bộ mình tạo, bộ được
 * chia sẻ, bộ admin đã bật hiển thị). Đây cũng là nguồn cho ô "Bộ API" ở form
 * tạo voice.
 */
export function useLLMAPISets() {
  return useQuery({
    queryKey: keys.llmApiSets,
    queryFn: () => api.get<{ items: LLMAPISet[] }>("/llm-api-sets"),
  });
}

/** Mở 1 bộ ra: chỉ endpoint này mới trả kèm danh sách key bên trong. */
export function useLLMAPISet(id: string | null) {
  return useQuery({
    queryKey: keys.llmApiSet(id ?? ""),
    queryFn: () => api.get<LLMAPISet>(`/llm-api-sets/${id}`),
    enabled: !!id,
  });
}

function invalidateLLMSets(qc: ReturnType<typeof useQueryClient>) {
  // Một khoá cho cả danh sách lẫn từng bộ: mọi key của bộ đều bắt đầu bằng
  // "llm-api-sets", nên sửa key bên trong cũng làm mới được bảng ngoài.
  qc.invalidateQueries({ queryKey: keys.llmApiSets });
}

export function useCreateLLMAPISet() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateLLMAPISetInput) => api.post<LLMAPISet>("/llm-api-sets", input),
    onSuccess: () => invalidateLLMSets(qc),
  });
}

export function useUpdateLLMAPISet() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateLLMAPISetInput) =>
      api.patch<LLMAPISet>(`/llm-api-sets/${id}`, body),
    onSuccess: () => invalidateLLMSets(qc),
  });
}

export function useDeleteLLMAPISet() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/llm-api-sets/${id}`),
    onSuccess: () => invalidateLLMSets(qc),
  });
}

export function useAddLLMAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ setId, ...body }: { setId: string } & LLMAPIKeyInput) =>
      api.post<LLMAPIKey>(`/llm-api-sets/${setId}/keys`, body),
    onSuccess: () => invalidateLLMSets(qc),
  });
}

export function useUpdateLLMAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateLLMAPIKeyInput) =>
      api.patch<LLMAPIKey>(`/llm-api-keys/${id}`, body),
    onSuccess: () => invalidateLLMSets(qc),
  });
}

export function useDeleteLLMAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/llm-api-keys/${id}`),
    onSuccess: () => invalidateLLMSets(qc),
  });
}

// ---------------------------------------------------------------------------
// Cài đặt (chỉ admin)
// ---------------------------------------------------------------------------

export function useSettings() {
  return useQuery({
    queryKey: keys.settings,
    queryFn: () => api.get<Settings>("/settings"),
  });
}

export function useUpdateSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Pick<Settings, "llm_chain" | "llm_batch" | "ai_prices">>) =>
      api.patch<Settings>("/settings", body),
    onSuccess: (data) => {
      qc.setQueryData(keys.settings, data);
      // Đổi đơn giá là đổi mọi con số tiền trong báo cáo — bảng đang mở phải
      // tính lại, nếu không admin sửa giá xong vẫn thấy con số cũ.
      qc.invalidateQueries({ queryKey: ["settings", "ai-usage"] });
    },
  });
}

/** Token/ký tự đã tiêu, quy ra tiền theo bảng giá admin khai. */
export function useAIUsage(days = 30) {
  return useQuery({
    queryKey: keys.aiUsage(days),
    queryFn: () => api.get<AIUsageReport>("/settings/ai-usage", { days }),
  });
}

/**
 * Đếm những thứ đang hỏng âm thầm — voice lỗi, token chủ kênh chết, kênh quét
 * hỏng.
 *
 * Tự làm mới mỗi phút: đây là thứ người dùng KHÔNG chủ động đi tìm, nên nếu chỉ
 * cập nhật lúc tải trang thì một phiên làm việc dài sẽ không bao giờ thấy sự cố
 * vừa xảy ra.
 */
export function useHealth() {
  return useQuery({
    queryKey: keys.health,
    queryFn: () => api.get<Health>("/meta/health"),
    refetchInterval: 60_000,
    // Hỏng thì im lặng: thanh cảnh báo không phải thứ đáng làm vỡ màn hình.
    retry: false,
  });
}

/**
 * Thống kê lỗi bị nền tảng chặn — dữ liệu để quyết định có cần proxy không.
 *
 * Chỉ `bot_block` và `rate_limit` là hai loại proxy giải quyết được;
 * `login_required` thì phải có cookies chứ proxy không giúp gì.
 */
export function useFetchStats(days = 7) {
  return useQuery({
    queryKey: keys.fetchStats(days),
    queryFn: () =>
      api.get<{ items: FetchErrorStat[]; days: number }>("/settings/fetch-stats", { days }),
  });
}

// ---------------------------------------------------------------------------
// Nhật ký thao tác
// ---------------------------------------------------------------------------

export interface AuditLogFilters {
  [key: string]: QueryValue;
  object_type?: string;
  object_id?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  dir?: string;
}

export function useAuditLog(filters: AuditLogFilters = {}) {
  return useQuery({
    queryKey: keys.auditLog(filters),
    queryFn: () => api.get<Page<AuditLog>>("/audit-log", filters),
  });
}

// ---------------------------------------------------------------------------
// Metadata hệ thống
// ---------------------------------------------------------------------------

/** Nền tảng nào đang nhận diện được — dùng cho ô chọn "Nền tảng". */
/**
 * ChannelScan — nền tảng này có quét được CẢ KÊNH không.
 *
 * Khác hẳn "nhận diện được URL": yt-dlp lấy từng bài X / Facebook / Instagram
 * bình thường nhưng không có extractor nào đọc được dòng thời gian của chúng.
 */
export interface ChannelScan {
  platform: Platform;
  enabled: boolean;
  /** Chỉ có khi enabled=false: vì sao không, và nên làm gì thay thế. */
  reason?: string;
}

/**
 * useLastUsedChoices — Prompt mẫu + Bộ API người này chạy gần nhất.
 *
 * Để form tạo voice chọn sẵn hai ô của hình thức C: gần như ai cũng chạy đi
 * chạy lại cùng một prompt, nên bắt chọn tay mỗi lần là hai lần bấm thừa cộng
 * một lần quên.
 *
 * Không cache lâu: người dùng vừa tạo voice bằng prompt khác thì lần mở modal
 * sau phải thấy đúng cái đó.
 */
export function useLastUsedChoices() {
  return useQuery({
    queryKey: ["meta", "last-used"],
    queryFn: () =>
      api.get<{ prompt_id: string | null; llm_api_set_id: string | null }>("/meta/last-used"),
    staleTime: 0,
  });
}

export function usePlatforms() {
  return useQuery({
    queryKey: keys.platforms,
    queryFn: () =>
      api.get<{ platforms: Platform[]; channel_scan: ChannelScan[] }>("/meta/platforms"),
    staleTime: 60 * 60_000,
  });
}

/**
 * useChannelScanSupport dựng sẵn hàm tra "URL này có quét được kênh không".
 *
 * Khớp theo HOST của URL người dùng đang gõ, vì form thêm kênh không có ô chọn
 * nền tảng — nền tảng được suy ra từ chính URL, giống hệt cách backend làm.
 */
export function useChannelScanSupport() {
  const meta = usePlatforms();
  const items = meta.data?.channel_scan ?? [];
  return (rawURL: string): ChannelScan | null => {
    const url = rawURL.trim().toLowerCase();
    if (!url) return null;
    const blocked = items.find((p) => !p.enabled && hostMatches(url, p.platform));
    return blocked ?? null;
  };
}

/** hostMatches: tên nền tảng có xuất hiện trong phần host của URL không. */
function hostMatches(url: string, platform: string): boolean {
  const host = url.replace(/^https?:\/\//, "").split("/")[0] ?? "";
  if (platform === "x") return /(^|\.)(x\.com|twitter\.com)$/.test(host);
  return host.includes(platform);
}

/**
 * useRandomAuthor bốc 1 tài khoản Strongbody theo giới tính.
 *
 * Là mutation chứ không phải query vì mỗi lần gọi phải ra một người KHÁC: bấm
 * nút random là bốc lại, cache ở đây sẽ trả về đúng người cũ.
 */
/**
 * useRandomAuthor bốc 1 tài khoản đứng tên bài đăng.
 *
 * KHÔNG dùng ở luồng tạo Voice nữa: việc bốc đã lùi xuống bước đăng ở backend
 * (Engine.ensureAuthor), nên chọn giới tính trên form không còn gọi mạng. Giữ
 * lại vì endpoint vẫn tồn tại và vẫn là cách duy nhất để bốc tay một tài khoản
 * cụ thể khi cần.
 */
export function useRandomAuthor() {
  return useMutation({
    mutationFn: ({ gender, countryId }: { gender: Gender; countryId?: number | null }) =>
      api.get<{ author: Author }>("/meta/authors/random", {
        gender,
        country_id: countryId || undefined,
      }),
  });
}

/**
 * useCountries lấy danh mục quốc gia của Strongbody để lọc author.
 *
 * Danh mục này gần như không đổi nên giữ cache cả phiên làm việc.
 */
export function useCountries() {
  return useQuery({
    queryKey: ["meta", "countries"] as const,
    queryFn: () => api.get<{ countries: Country[] }>("/meta/countries"),
    staleTime: 24 * 60 * 60_000,
  });
}

/**
 * useHashtagSearch tìm hashtag trong danh mục ĐÃ LƯU của tool.
 *
 * Phải hỏi server vì danh mục MultiMe có ~94.000 mục — tải hết về trình duyệt
 * để lọc tại chỗ là vài MB cho mỗi lần mở modal. Đây KHÔNG phải gọi sang
 * MultiMe: nó lọc trong DB của tool, và chỉ chạy khi người dùng đã gõ.
 *
 * `enabled` tắt khi chưa gõ gì: lúc đó dropdown dùng phần đầu danh mục đã nằm
 * sẵn trong cache của useCatalog, không tốn thêm request nào.
 */
export function useHashtagSearch(q: string) {
  const keyword = q.trim();
  return useQuery({
    queryKey: keys.hashtags(keyword),
    queryFn: () => api.get<{ items: Hashtag[] }>("/meta/hashtags", { q: keyword }),
    enabled: keyword.length > 0,
    staleTime: 5 * 60_000,
    placeholderData: (prev) => prev,
  });
}

/**
 * useCatalog — danh mục Quốc gia + Hashtag + thứ tự Ngôn ngữ, LẤY MỘT LẦN.
 *
 * `staleTime: Infinity` là điều kiện của yêu cầu "modal Tạo Voice không gọi API
 * để lấy danh sách": react-query chỉ fetch ở lần đầu trong phiên, những lần mở
 * modal sau đọc thẳng từ cache và mọi thao tác search chạy trên đó.
 *
 * Danh mục thay đổi rất chậm (quốc gia vài năm, hashtag theo lượng voice mới),
 * nên tải lại trang là đủ để thấy bản mới — không đáng đánh đổi bằng một lần
 * gọi mạng ở mỗi lần mở modal.
 */
export function useCatalog() {
  return useQuery({
    queryKey: keys.catalog,
    queryFn: () => api.get<Catalog>("/meta/catalog"),
    staleTime: Infinity,
    gcTime: Infinity,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });
}

/**
 * useUploadPendingImage tải ảnh bìa lên khi CHƯA có voice nào.
 *
 * Màn tạo Voice là một bước: ảnh được chọn trước cả khi Bài Post tồn tại, nên
 * không dùng được `/voices/:id/image`. URL trả về đi kèm request tạo Bài Post.
 */
export function useUploadPendingImage() {
  return useMutation({
    mutationFn: (file: File) =>
      api.upload<{ image_url: string; image_uploaded: boolean }>("/images", file),
  });
}

/**
 * Điều kiện đăng bài của multime.ai. Hashtag mặc định đã bị bỏ, nên thứ còn
 * đọc ở đây là độ dài tối thiểu và category cấu hình sẵn.
 */
export function usePublishRequirements() {
  return useQuery({
    queryKey: keys.publishMeta,
    queryFn: () => api.get<PublishRequirements>("/meta/publish"),
    staleTime: 60 * 60_000,
  });
}

/** Hình thức thu thập nào đang bật (ENABLED_COLLECT_MODES) — tắt thì UI làm mờ. */
/**
 * useVoiceStyleOptions — bộ giá trị hợp lệ cho mục "Cấu hình giọng đọc".
 *
 * Lấy từ backend thay vì chép cứng: đây là bộ từ khoá của nhà cung cấp TTS và
 * nó là thứ duy nhất được validate. Cache dài vì nó gần như không đổi.
 */
export function useVoiceStyleOptions() {
  return useQuery({
    queryKey: keys.voiceStyleOptions,
    queryFn: () => api.get<VoiceStyleOptions>("/meta/voice-style"),
    staleTime: 60 * 60_000,
  });
}

export function useCollectModes() {
  return useQuery({
    queryKey: keys.collectModes,
    queryFn: () => api.get<{ collect_modes: CollectModeMeta[] }>("/meta/collect-modes"),
    staleTime: 60 * 60_000,
  });
}

// ---------------------------------------------------------------------------
// Phân quyền + quản lý tài khoản
// ---------------------------------------------------------------------------

/** useMe trả về role và các quyền của người đang đăng nhập. */
export function useMe() {
  return useQuery({
    queryKey: keys.me,
    queryFn: () => api.get<Me>("/me"),
    staleTime: 5 * 60_000,
  });
}

/**
 * Danh sách tài khoản. Route `/users` CHỈ ADMIN.
 *
 * `enabled` để chỗ gọi tắt hẳn query cho vai trò không được xem: không tắt thì
 * mỗi lần mở bảng là một lỗi 403 vô nghĩa trong log của cả hai phía.
 */
export function useUsers(
  filters: { limit?: number; offset?: number; sort?: string; dir?: string } = {},
  enabled = true,
) {
  return useQuery({
    queryKey: keys.users(filters),
    queryFn: () => api.get<Page<User>>("/users", filters),
    enabled,
  });
}

export function useSetUserRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) =>
      api.patch<User>(`/users/${id}/role`, { role }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export function useSetUserActive() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, is_active }: { id: string; is_active: boolean }) =>
      api.patch<User>(`/users/${id}/active`, { is_active }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

// ---------------------------------------------------------------------------
// Hạ tầng via/proxy (màn Cài đặt)
// ---------------------------------------------------------------------------

/**
 * Bảng via tự làm mới mỗi 30 giây.
 *
 * Trạng thái via đổi do WORKER chứ không do thao tác trên màn hình này: một via
 * có thể vào cooldown trong lúc người vận hành đang nhìn bảng. Không tự làm mới
 * thì thứ họ đang nhìn là ảnh chụp của vài phút trước, và đó chính là lúc họ ra
 * quyết định thay via.
 */
export function useVias(platform = "") {
  return useQuery({
    queryKey: keys.vias(platform),
    queryFn: () => api.get<{ items: Via[] }>("/settings/vias", { platform: platform || undefined }),
    refetchInterval: 30_000,
  });
}

export interface ViaInput {
  platform?: string;
  label?: string;
  /** Chỉ đi MỘT CHIỀU. Bỏ trống khi sửa = giữ cookies cũ. */
  cookies?: string;
  daily_quota?: number;
  /** Chỉ `active` / `disabled` — cooldown và dead do hệ thống kết luận. */
  status?: "active" | "disabled";
}

export function useCreateVia() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ViaInput) => api.post<Via>("/settings/vias", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings"] }),
  });
}

export function useUpdateVia() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & ViaInput) =>
      api.patch<Via>(`/settings/vias/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings"] }),
  });
}

export function useDeleteVia() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/settings/vias/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings"] }),
  });
}

export function useProxies(platform = "") {
  return useQuery({
    queryKey: keys.proxies(platform),
    queryFn: () =>
      api.get<{ items: ScrapeProxy[] }>("/settings/proxies", { platform: platform || undefined }),
    refetchInterval: 30_000,
  });
}

export interface ProxyInput {
  label?: string;
  platform?: string;
  /** Chỉ đi MỘT CHIỀU. Bỏ trống khi sửa = giữ endpoint cũ. */
  endpoint?: string;
  kind?: "residential" | "mobile" | "datacenter";
  status?: "active" | "disabled";
}

export function useCreateProxy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ProxyInput) => api.post<ScrapeProxy>("/settings/proxies", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings"] }),
  });
}

export function useUpdateProxy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & ProxyInput) =>
      api.patch<ScrapeProxy>(`/settings/proxies/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings"] }),
  });
}

export function useDeleteProxy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/settings/proxies/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings"] }),
  });
}

export function useScrapeHealth() {
  return useQuery({
    queryKey: keys.scrapeHealth,
    queryFn: () => api.get<{ items: ViaHealth[] }>("/settings/scrape-health"),
    refetchInterval: 30_000,
  });
}

export function useScrapeLoad(days = 7) {
  return useQuery({
    queryKey: keys.scrapeLoad(days),
    queryFn: () =>
      api.get<{ items: ScrapeHourRow[]; days: number }>("/settings/scrape-load", { days }),
  });
}

/**
 * Nền tảng nào cần cookie gì.
 *
 * Danh mục tĩnh (chỉ đổi khi code đổi) nên lấy một lần rồi giữ: mở form thêm
 * via không phải gọi lại.
 */
export function useViaCookieSpecs() {
  return useQuery({
    queryKey: keys.viaCookieSpecs,
    queryFn: () => api.get<{ items: ViaCookieSpec[] }>("/settings/via-cookie-specs"),
    staleTime: Infinity,
  });
}
