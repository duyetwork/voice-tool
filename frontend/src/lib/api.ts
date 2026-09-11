import type { TokenPair, User } from "@/types/api";

/**
 * BASE_URL là địa chỉ API.
 *
 * QUAN TRỌNG: `NEXT_PUBLIC_*` được Next.js thay vào code lúc BUILD, không phải
 * lúc chạy — đặt biến này trong `environment` của container là vô tác dụng,
 * phải truyền qua build arg (xem frontend/Dockerfile).
 *
 * Mặc định là đường dẫn TƯƠNG ĐỐI: khi frontend và API nằm sau cùng 1 reverse
 * proxy / 1 tên miền (cách deploy thông thường), image build 1 lần chạy được ở
 * mọi môi trường mà không cần biết trước domain. Dev chạy 2 cổng riêng thì
 * `.env.development` trỏ sang http://localhost:8080.
 */
//
// Dùng `||` chứ KHÔNG phải `??`: build production truyền biến này là chuỗi
// RỖNG để nói "gọi tương đối đi". `??` chỉ rơi về mặc định khi null/undefined
// nên chuỗi rỗng sẽ thắng, và bundle gọi `/voices` thay vì `/api/v1/voices` —
// reverse proxy đẩy về Next và toàn bộ API trả 404. Lỗi này chỉ lộ ra sau khi
// đã deploy, nên phải chặn ngay ở đây.
const BASE_URL = (process.env.NEXT_PUBLIC_API_URL || "/api/v1").replace(/\/$/, "");

/** absolute dựng URL đầy đủ, chấp nhận BASE_URL tương đối lẫn tuyệt đối. */
function absolute(path: string): string {
  if (/^https?:\/\//i.test(BASE_URL)) return BASE_URL + path;
  const origin = typeof window === "undefined" ? "http://localhost" : window.location.origin;
  return origin + BASE_URL + path;
}

const ACCESS_KEY = "voice-tool.access_token";
const REFRESH_KEY = "voice-tool.refresh_token";
const USER_KEY = "voice-tool.user";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    /**
     * Payload gốc của response lỗi. Một số lỗi mang thêm dữ liệu có cấu trúc
     * (ví dụ `duplicate_post` trả kèm bài đã có trong hệ thống) mà UI cần đọc,
     * không chỉ hiện câu thông báo.
     */
    public payload?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// ---------------------------------------------------------------------------
// Token store — giữ trong localStorage, chỉ đọc được ở client component.
// ---------------------------------------------------------------------------

export const tokenStore = {
  access(): string | null {
    if (typeof window === "undefined") return null;
    return window.localStorage.getItem(ACCESS_KEY);
  },
  refresh(): string | null {
    if (typeof window === "undefined") return null;
    return window.localStorage.getItem(REFRESH_KEY);
  },
  user(): User | null {
    if (typeof window === "undefined") return null;
    const raw = window.localStorage.getItem(USER_KEY);
    return raw ? (JSON.parse(raw) as User) : null;
  },
  save(pair: TokenPair, user?: User) {
    window.localStorage.setItem(ACCESS_KEY, pair.access_token);
    window.localStorage.setItem(REFRESH_KEY, pair.refresh_token);
    if (user) window.localStorage.setItem(USER_KEY, JSON.stringify(user));
  },
  clear() {
    window.localStorage.removeItem(ACCESS_KEY);
    window.localStorage.removeItem(REFRESH_KEY);
    window.localStorage.removeItem(USER_KEY);
  },
};

// ---------------------------------------------------------------------------
// Fetch wrapper: tự gắn Bearer token và tự refresh 1 lần khi gặp 401.
// ---------------------------------------------------------------------------

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined | null>;
  skipAuth?: boolean;
}

async function request<T>(path: string, options: RequestOptions = {}, retry = true): Promise<T> {
  const { method = "GET", body, query, skipAuth } = options;

  const url = new URL(absolute(path));
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
  }

  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";

  const token = skipAuth ? null : tokenStore.access();
  if (token) headers.Authorization = `Bearer ${token}`;

  const res = await fetch(url.toString(), {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });

  if (res.status === 401 && retry && !skipAuth && tokenStore.refresh()) {
    const refreshed = await tryRefresh();
    if (refreshed) return request<T>(path, options, false);
  }

  if (!res.ok) {
    const payload = await res.json().catch(() => null);
    throw new ApiError(
      res.status,
      payload?.error ?? "unknown_error",
      payload?.message ?? `Yêu cầu thất bại (HTTP ${res.status})`,
      payload,
    );
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

async function tryRefresh(): Promise<boolean> {
  try {
    const data = await request<{ token: TokenPair }>(
      "/auth/refresh",
      {
        method: "POST",
        body: { refresh_token: tokenStore.refresh() },
        skipAuth: true,
      },
      false,
    );
    tokenStore.save(data.token);
    return true;
  } catch {
    tokenStore.clear();
    return false;
  }
}

/**
 * audioUrl trả URL phát/tải file voice để gắn thẳng vào thẻ <audio>.
 *
 * Không dùng URL storage trực tiếp: bucket là riêng tư và host `minio:9000`
 * chỉ tồn tại trong mạng Docker. Thẻ <audio> lại không gắn được header
 * Authorization, nên token đi kèm trong query — backend chấp nhận điều này
 * riêng cho route media (`middleware.AuthMedia`).
 */
export function audioUrl(voiceId: string): string {
  const base = absolute(`/voices/${voiceId}/audio`);
  const token = tokenStore.access();
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}

/**
 * coverImageUrl trả URL ảnh bìa ĐÃ TẢI LÊN để gắn vào thẻ <img>.
 *
 * Cùng lý do với audioUrl: bucket riêng tư và host storage chỉ tồn tại trong
 * mạng nội bộ, nên ảnh phải đi qua API và token nằm trong URL (thẻ <img> không
 * gắn được header Authorization).
 *
 * `version` là image_url đang lưu: đổi ảnh là đổi key, nên URL đổi theo và
 * trình duyệt không hiện lại ảnh cũ trong cache.
 */
export function coverImageUrl(voiceId: string, version?: string | null): string {
  const params = new URLSearchParams();
  const token = tokenStore.access();
  if (token) params.set("token", token);
  if (version) params.set("v", version);
  return `${absolute(`/voices/${voiceId}/image`)}?${params.toString()}`;
}

/**
 * upload gửi file qua multipart. Không đi qua `request` vì hàm đó luôn
 * JSON.stringify body; ảnh vài MB nhét vào JSON phải base64 (phình 33%) trong
 * khi multipart là thứ trình duyệt gửi sẵn.
 *
 * Content-Type để trình duyệt tự đặt — tự viết tay là thiếu `boundary` và
 * server không tách được các phần.
 */
async function upload<T>(path: string, file: File, retry = true): Promise<T> {
  const body = new FormData();
  body.append("file", file);

  const headers: Record<string, string> = { Accept: "application/json" };
  const token = tokenStore.access();
  if (token) headers.Authorization = `Bearer ${token}`;

  const res = await fetch(absolute(path), { method: "POST", headers, body, cache: "no-store" });

  if (res.status === 401 && retry && tokenStore.refresh()) {
    const refreshed = await tryRefresh();
    if (refreshed) return upload<T>(path, file, false);
  }
  if (!res.ok) {
    const payload = await res.json().catch(() => null);
    throw new ApiError(
      res.status,
      payload?.error ?? "unknown_error",
      payload?.message ?? `Tải file thất bại (HTTP ${res.status})`,
      payload,
    );
  }
  return (await res.json()) as T;
}

export const api = {
  get: <T>(path: string, query?: RequestOptions["query"]) => request<T>(path, { query }),
  upload,
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", body }),
  patch: <T>(path: string, body?: unknown) => request<T>(path, { method: "PATCH", body }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
  /**
   * signIn xác thực bằng tài khoản strongbody/multime. Hệ thống không có đăng
   * ký — tài khoản nào đăng nhập được vào multime thì đăng nhập được vào đây.
   */
  signIn: (email: string, password: string) =>
    request<{ token: TokenPair; user: User }>("/auth/login", {
      method: "POST",
      body: { email, password },
      skipAuth: true,
    }),
};
