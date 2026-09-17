"use client";

import * as React from "react";

import { Field, Input } from "@/components/ui/field";
import { useChannelPostCap } from "@/hooks/use-api";

/**
 * Ba con số hay bị nhầm với nhau, nên gom một chỗ và gọi tên theo đúng việc
 * chúng làm:
 *
 *   scan_limit        — mỗi LẦN quét lấy về bao nhiêu bài mới nhất của kênh
 *   backfill_limit    — lúc THÊM kênh, lấy về bao nhiêu bài đã đăng từ trước
 *   max_posts_per_run — mỗi vòng được TẠO tối đa bao nhiêu Bài Post
 *
 * Hai con số đầu trả lời hai câu hỏi khác nhau ở hai thời điểm khác nhau: vòng
 * quét đầu tiên dùng backfill_limit, mọi vòng sau dùng scan_limit.
 *
 * MỌI Ô ĐỀU ĐIỀN SẴN GIÁ TRỊ MẶC ĐỊNH, không để trống kèm placeholder. Ô trống
 * buộc người dùng phải đoán "trống nghĩa là gì" — và với ba con số mang ba ý
 * nghĩa khác nhau thì mỗi ô lại đoán một kiểu. Xoá trắng ô nào thì ô đó về 0,
 * và 0 có nghĩa rõ ràng: không lấy bài cũ nào / không giới hạn số bài tạo ra.
 */

/** DEFAULT_SCAN_LIMIT / DEFAULT_BACKFILL là giá trị điền sẵn cho kênh MỚI. */
export const DEFAULT_SCAN_LIMIT = 50;
export const DEFAULT_BACKFILL = 50;

export interface TuningDraft {
  scanLimit: string;
  backfillLimit: string;
  maxPostsPerRun: string;
}

/** emptyTuning là bản nháp cho kênh MỚI — điền sẵn, không để trống. */
export const emptyTuning: TuningDraft = {
  scanLimit: String(DEFAULT_SCAN_LIMIT),
  backfillLimit: String(DEFAULT_BACKFILL),
  maxPostsPerRun: String(DEFAULT_SCAN_LIMIT),
};

export function tuningDraftOf(list: {
  scan_limit: number;
  backfill_limit: number;
  max_posts_per_run?: number | null;
  backfill_done_at?: string | null;
}): TuningDraft {
  return {
    scanLimit: String(list.scan_limit),
    backfillLimit: String(list.backfill_limit),
    // null trong DB = không giới hạn. Hiện lại bằng chính cửa sổ quét, vì đó là
    // trần thật sự: một vòng không bao giờ tạo ra nhiều Bài Post hơn số bài nó
    // nhìn thấy.
    maxPostsPerRun:
      list.max_posts_per_run == null ? String(list.scan_limit) : String(list.max_posts_per_run),
  };
}

/** numOr đọc ô số; rỗng hoặc không phải số = 0. */
function numOr(raw: string): number {
  const n = Number(raw.trim());
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : 0;
}

export function tuningCreatePayload(draft: TuningDraft) {
  return {
    scan_limit: numOr(draft.scanLimit) || undefined,
    backfill_limit: numOr(draft.backfillLimit),
    max_posts_per_run: numOr(draft.maxPostsPerRun) || undefined,
  };
}

export function tuningUpdatePayload(draft: TuningDraft) {
  return {
    scan_limit: numOr(draft.scanLimit) || undefined,
    backfill_limit: numOr(draft.backfillLimit),
    // 0 ở PATCH = BỎ trần (backend đổi thành NULL). Khác undefined, vốn nghĩa
    // là "không sửa" — không phân biệt hai cái thì đặt trần rồi không gỡ được.
    max_posts_per_run: numOr(draft.maxPostsPerRun),
  };
}

export function ChannelTuning({
  value,
  onChange,
  /** Vòng quét đầu đã chạy xong -> số bài cũ không còn tác dụng gì nữa. */
  backfillDone = false,
  /** Nền tảng của kênh — để KHOÁ ô nhập theo trần của nó. */
  platform = "",
}: {
  value: TuningDraft;
  onChange: (next: TuningDraft) => void;
  backfillDone?: boolean;
  platform?: string;
}) {
  // Trần số bài/lượt quét của nền tảng này, lấy từ backend. null = không có
  // trần (YouTube, TikTok — yt-dlp phân trang được).
  const cap = useChannelPostCap(platform);

  /**
   * KHOÁ ô nhập thay vì cảnh báo.
   *
   * Trước đây form giữ nguyên con số 50 và hiện một dòng đỏ "nền tảng chỉ trả
   * về 12". Người dùng vẫn lưu được 50, vòng quét vẫn chỉ mang về 12, và cảnh
   * báo đó biến thành thứ đọc một lần rồi bỏ qua. Một con số không bao giờ đạt
   * được thì không nên nhập vào được ngay từ đầu.
   */
  const clamp = (raw: string) => {
    if (!cap) return raw;
    const n = numOr(raw);
    return n > cap.max_posts ? String(cap.max_posts) : raw;
  };

  const set = (patch: Partial<TuningDraft>) => onChange({ ...value, ...patch });

  // Nền tảng chỉ biết được sau khi người dùng gõ URL, nên giá trị điền sẵn (50)
  // và giá trị của kênh cũ có thể đang vượt trần. Ép chúng xuống ngay khi biết
  // trần là gì — nếu không, người dùng bấm Lưu mà không hề đụng vào hai ô này
  // và con số quá trần vẫn đi thẳng xuống DB.
  React.useEffect(() => {
    if (!cap) return;
    const scan = numOr(value.scanLimit);
    // Backfill đã chạy xong thì KHÔNG đụng vào: con số đó chỉ còn là ghi chép
    // về việc đã xảy ra, và sửa nó ở đây là lặng lẽ viết lại lịch sử của kênh
    // trong lúc người dùng chỉ định sửa một thứ khác.
    const backfill = backfillDone ? 0 : numOr(value.backfillLimit);
    if (scan <= cap.max_posts && backfill <= cap.max_posts) return;
    onChange({
      ...value,
      scanLimit: scan > cap.max_posts ? String(cap.max_posts) : value.scanLimit,
      backfillLimit: backfill > cap.max_posts ? String(cap.max_posts) : value.backfillLimit,
    });
  }, [cap, value, onChange, backfillDone]);

  // Cửa sổ quét là TRẦN CỨNG của trần Bài Post: một vòng không thể tạo ra nhiều
  // bài hơn số bài nó nhìn thấy, nên số lớn hơn chỉ im lặng không có tác dụng.
  //
  // Số bài cũ thì KHÔNG bị chặn bởi nó: vòng quét đầu đi hỏi nền tảng đúng con
  // số này, độc lập với cửa sổ của các vòng sau (xem firstRunLimit ở backend).
  const window = numOr(value.scanLimit);
  const maxOver = window > 0 && numOr(value.maxPostsPerRun) > window;

  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Field
        label="Số bài mỗi lần quét"
        hint={
          cap
            ? `Tối đa ${cap.max_posts} với nền tảng này. ${cap.max_posts_reason ?? ""}`
            : "Mỗi vòng quét nhìn bấy nhiêu bài mới nhất của kênh để dò bài mới. Xoá trắng = 0 = dùng mặc định hệ thống."
        }
      >
        <Input
          type="number"
          min={0}
          max={cap ? cap.max_posts : 200}
          value={value.scanLimit}
          onChange={(e) => set({ scanLimit: clamp(e.target.value) })}
        />
      </Field>

      <Field
        label="Số bài cũ của kênh"
        hint={
          backfillDone
            ? "Đã lấy xong ở vòng quét đầu — đổi số này không còn tác dụng."
            : cap
              ? `Tối đa ${cap.max_posts} với nền tảng này. ${cap.max_posts_reason ?? ""}`
              : "Lúc thêm kênh, lấy về bấy nhiêu bài đã đăng từ trước (mới nhất trước). 0 = chỉ lấy bài đăng sau khi thêm kênh."
        }
      >
        <Input
          type="number"
          min={0}
          max={cap ? cap.max_posts : 200}
          disabled={backfillDone}
          value={value.backfillLimit}
          onChange={(e) => set({ backfillLimit: clamp(e.target.value) })}
        />
      </Field>

      <Field
        label="Trần Bài Post mỗi vòng"
        error={
          maxOver
            ? `Lớn hơn số bài mỗi lần quét (${window}) — phần vượt không có tác dụng.`
            : undefined
        }
        hint="Chặn nổ chi phí AI khi kênh đăng ồ ạt; phần dư để vòng sau xử lý tiếp. Xoá trắng = 0 = không giới hạn."
      >
        <Input
          type="number"
          min={0}
          value={value.maxPostsPerRun}
          onChange={(e) => set({ maxPostsPerRun: e.target.value })}
        />
      </Field>
    </div>
  );
}
