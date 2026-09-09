import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDateTime(value?: string | null) {
  if (!value) return "—";
  return new Date(value).toLocaleString("vi-VN", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** formatInterval hiển thị Postgres INTERVAL (pgx trả về dạng object). */
export function formatInterval(
  iv?: {
    Microseconds: number;
    Days: number;
    Months: number;
    Valid: boolean;
  } | null,
): string {
  if (!iv?.Valid) return "—";

  const totalSeconds =
    Math.round(iv.Microseconds / 1_000_000) + iv.Days * 86_400 + iv.Months * 30 * 86_400;

  if (totalSeconds < 60) return `mỗi ${totalSeconds}s`;
  if (totalSeconds % 86_400 === 0) return `mỗi ${totalSeconds / 86_400} ngày`;
  if (totalSeconds % 3_600 === 0) return `mỗi ${totalSeconds / 3_600} giờ`;
  if (totalSeconds % 60 === 0) return `mỗi ${totalSeconds / 60} phút`;
  return `mỗi ${totalSeconds}s`;
}

/** Nhãn tiếng Việt cho 3 hình thức thu thập voice (specs 1.2). */
export const COLLECT_MODE_LABELS: Record<string, string> = {
  A: "A — Extract từ URL",
  B: "B — Text → AI gen → voice",
  C: "C — Text + Prompt → AI gen → voice",
};

export const POST_STATUS_LABELS: Record<string, string> = {
  new: "Mới",
  processing: "Đang xử lý",
  processed: "Đã tạo voice",
  failed: "Lỗi",
};

export const PUBLISH_STATUS_LABELS: Record<string, string> = {
  draft: "Nháp",
  ready: "Chờ đăng",
  published: "Đã đăng",
  failed: "Đăng lỗi",
};

export const SOURCE_TYPE_LABELS: Record<string, string> = {
  F1: "F1 — On-demand",
  BREAKING: "F2 — Breaking",
  SCHEDULED: "F3 — Định kỳ",
};

export const PLATFORM_LABELS: Record<string, string> = {
  youtube: "YouTube",
  facebook: "Facebook",
  tiktok: "TikTok",
  instagram: "Instagram",
  x: "X (Twitter)",
};

export function platformLabel(value?: string | null): string {
  if (!value) return "—";
  return PLATFORM_LABELS[value] ?? value;
}

/**
 * Ngôn ngữ: "auto" là mặc định — để nền tảng nguồn / multime.ai tự nhận diện.
 * Đoán sai ngôn ngữ tệ hơn là không đoán.
 */
export const LANGUAGE_OPTIONS: { value: string; label: string }[] = [
  { value: "auto", label: "Tự nhận diện" },
  { value: "vi", label: "Tiếng Việt (vi)" },
  { value: "en", label: "English (en)" },
  { value: "ja", label: "日本語 (ja)" },
  { value: "ko", label: "한국어 (ko)" },
  { value: "zh", label: "中文 (zh)" },
];

export function languageLabel(value?: string | null): string {
  if (!value) return "—";
  return LANGUAGE_OPTIONS.find((o) => o.value === value)?.label ?? value;
}

/** formatBytes hiển thị dung lượng file voice. */
export function formatBytes(bytes?: number | null): string {
  if (!bytes) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

/** formatDuration: 95 -> "1:35". */
export function formatDuration(seconds?: number | null): string {
  if (!seconds) return "—";
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}
