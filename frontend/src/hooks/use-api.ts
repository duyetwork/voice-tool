"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "@/lib/api";
import type {
  AIEngine,
  AuditLog,
  CollectMode,
  CollectModeMeta,
  Platform,
  PublishRequirements,
  Me,
  ItemList,
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
  prompts: ["prompts"] as const,
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
}

export function useSourcePosts(filters: SourcePostFilters = {}) {
  return useQuery({
    queryKey: keys.sourcePosts(filters),
    queryFn: () => api.get<Page<SourcePost>>("/source-posts", filters),
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
}

export function useVoices(filters: VoiceFilters = {}) {
  return useQuery({
    queryKey: keys.voices(filters),
    queryFn: () => api.get<Page<Voice>>("/voices", filters),
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
}

export function useBreakingLists(filters: ChannelFilters = {}) {
  return useQuery({
    queryKey: keys.breaking(filters),
    queryFn: () => api.get<ItemList<ListBreaking>>("/lists/breaking", filters),
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
    queryFn: () => api.get<ItemList<ListScheduled>>("/lists/scheduled", filters),
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

export function usePrompts() {
  return useQuery({
    queryKey: keys.prompts,
    queryFn: () => api.get<ItemList<Prompt>>("/prompts", { limit: 200 }),
  });
}

export function useCreatePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; content: string }) => api.post<Prompt>("/prompts", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.prompts }),
  });
}

export function useDeletePrompt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/prompts/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.prompts }),
  });
}

export function useAIEngines() {
  return useQuery({
    queryKey: keys.aiEngines,
    queryFn: () => api.get<ItemList<AIEngine>>("/ai-engines"),
  });
}

export function useCreateAIEngine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      name: string;
      provider: string;
      supported_languages: string[];
      is_active: boolean;
    }) => api.post<AIEngine>("/ai-engines", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.aiEngines }),
  });
}

export function useUpdateAIEngine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; is_active?: boolean }) =>
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

export function useAuditLog(filters: { object_type?: string; object_id?: string } = {}) {
  return useQuery({
    queryKey: keys.auditLog(filters),
    queryFn: () => api.get<ItemList<AuditLog>>("/audit-log", { ...filters, limit: 100 }),
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
 * Điều kiện đăng bài của multime.ai. Có `default_hashtags` cấu hình sẵn thì
 * voice không có hashtag riêng vẫn đăng được — UI không nên chặn.
 */
export function usePublishRequirements() {
  return useQuery({
    queryKey: keys.publishMeta,
    queryFn: () => api.get<PublishRequirements>("/meta/publish"),
    staleTime: 60 * 60_000,
  });
}

/** Hình thức thu thập nào đang bật — B/C chưa hỗ trợ thì hiển thị mờ. */
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

export function useUsers(filters: { limit?: number; offset?: number } = {}) {
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
