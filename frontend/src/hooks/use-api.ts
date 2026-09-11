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
  CollectMode,
  CollectModeMeta,
  Platform,
  PublishRequirements,
  Me,
  ListBreaking,
  ListScheduled,
  Page,
  Prompt,
  Role,
  SourcePost,
  User,
  Voice,
} from "@/types/api";

export const keys = {
  me: ["me"] as const,
  users: (filters: Record<string, unknown>) => ["users", filters] as const,
  sourcePosts: (filters: Record<string, unknown>) => ["source-posts", filters] as const,
  voices: (filters: Record<string, unknown>) => ["voices", filters] as const,
  breaking: (filters: Record<string, unknown>) => ["lists", "breaking", filters] as const,
  scheduled: (filters: Record<string, unknown>) => ["lists", "scheduled", filters] as const,
  prompts: (filters: Record<string, unknown>) => ["prompts", filters] as const,
  aiEngines: ["ai-engines"] as const,
  auditLog: (filters: Record<string, unknown>) => ["audit-log", filters] as const,
  platforms: ["meta", "platforms"] as const,
  publishMeta: ["meta", "publish"] as const,

  collectModes: ["meta", "collect-modes"] as const,
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
  /** Tạo xong audio thì đăng luôn lên multime. */
  publish_when_ready?: boolean;
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
          (v.publish_when_ready && v.publish_status !== "published" && v.publish_status !== "failed"),
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
  text: string;
  collect_mode: Exclude<CollectMode, "A">;
  prompt_id?: string | null;
  language?: string;
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
  scan_limit?: number;
  scan_interval?: string;
}

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
    mutationFn: ({ id, ...body }: { id: string } & Partial<ListBreaking>) =>
      api.patch<ListBreaking>(`/lists/breaking/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "breaking"] }),
  });
}

export function useRunBreakingList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<{ status: string }>(`/lists/breaking/${id}/run`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["source-posts"] }),
  });
}

export function useDeleteBreakingList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/lists/breaking/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "breaking"] }),
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
  scan_limit?: number;
  max_posts_per_run?: number;
}

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
    mutationFn: ({ id, ...body }: { id: string } & Partial<ListScheduled>) =>
      api.patch<ListScheduled>(`/lists/scheduled/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["lists", "scheduled"] }),
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
export function usePlatforms() {
  return useQuery({
    queryKey: keys.platforms,
    queryFn: () => api.get<{ platforms: Platform[] }>("/meta/platforms"),
    staleTime: 60 * 60_000,
  });
}

/**
 * useRandomAuthor bốc 1 tài khoản Strongbody theo giới tính.
 *
 * Là mutation chứ không phải query vì mỗi lần gọi phải ra một người KHÁC: bấm
 * nút random là bốc lại, cache ở đây sẽ trả về đúng người cũ.
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

export function useUsers(
  filters: { limit?: number; offset?: number; sort?: string; dir?: string } = {},
) {
  return useQuery({
    queryKey: keys.users(filters),
    queryFn: () => api.get<Page<User>>("/users", filters),
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
