"use client";

import * as React from "react";

import { Field, Input } from "@/components/ui/field";

/**
 * Tham số quét của 1 kênh — dùng chung cho cả Breaking lẫn Định kỳ, cả lúc
 * thêm lẫn lúc sửa.
 *
 * Ba con số ở đây trả lời ba câu hỏi khác nhau, và trước khi gom vào một chỗ
 * thì chúng hay bị nhầm lẫn với nhau:
 *
 *   scan_limit        — mỗi vòng NHÌN bao nhiêu bài mới nhất của kênh
 *   backfill_limit    — lần đầu tiên, LẤY bao nhiêu bài đã đăng từ trước
 *   max_posts_per_run — mỗi vòng được TẠO tối đa bao nhiêu Bài Post
 */

/** Hình dạng của FORM: chuỗi, vì ô trống phải phân biệt được với số 0. */
export interface TuningDraft {
  scanLimit: string;
  backfillLimit: string;
  maxPostsPerRun: string;
}

export const emptyTuning: TuningDraft = {
  scanLimit: "",
  backfillLimit: "",
  maxPostsPerRun: "",
};

export interface TunableChannel {
  scan_limit: number;
  backfill_limit: number;
  max_posts_per_run?: number | null;
  backfill_done_at?: string | null;
}

/** Đổ giá trị đang lưu vào form sửa. */
export function tuningDraftOf(list: TunableChannel): TuningDraft {
  return {
    scanLimit: String(list.scan_limit),
    backfillLimit: String(list.backfill_limit),
    // null = không giới hạn, và "không giới hạn" hiển thị bằng ô TRỐNG chứ
    // không phải số 0: ô trống là thứ người dùng đọc được ngay.
    maxPostsPerRun: list.max_posts_per_run == null ? "" : String(list.max_posts_per_run),
  };
}

/** Payload lúc THÊM kênh: bỏ trống = để server dùng mặc định của nó. */
export function tuningCreatePayload(draft: TuningDraft) {
  return {
    scan_limit: draft.scanLimit ? Number(draft.scanLimit) : undefined,
    backfill_limit: draft.backfillLimit ? Number(draft.backfillLimit) : undefined,
    max_posts_per_run: draft.maxPostsPerRun ? Number(draft.maxPostsPerRun) : undefined,
  };
}

/**
 * Payload lúc SỬA kênh.
 *
 * Khác `tuningCreatePayload` ở đúng một chỗ: ô trần bỏ trống nghĩa là người
 * dùng vừa XOÁ cái trần, và điều đó phải gửi lên thành 0 (= bỏ giới hạn).
 * Gửi `undefined` như lúc tạo thì server hiểu là "không đụng tới", và cái trần
 * cũ ở lại mãi mãi.
 */
export function tuningUpdatePayload(draft: TuningDraft) {
  return {
    scan_limit: draft.scanLimit ? Number(draft.scanLimit) : undefined,
    backfill_limit: draft.backfillLimit ? Number(draft.backfillLimit) : undefined,
    max_posts_per_run: draft.maxPostsPerRun ? Number(draft.maxPostsPerRun) : 0,
  };
}

export function ChannelTuning({
  value,
  onChange,
  /** Vòng quét đầu đã chạy xong -> số bài cũ không còn tác dụng gì nữa. */
  backfillDone = false,
  defaultScanLimit = 20,
}: {
  value: TuningDraft;
  onChange: (next: TuningDraft) => void;
  backfillDone?: boolean;
  defaultScanLimit?: number;
}) {
  const set = (patch: Partial<TuningDraft>) => onChange({ ...value, ...patch });

  // Cửa sổ quét là TRẦN CỨNG của cả hai ô kia: vòng quét chỉ nhìn thấy bấy
  // nhiêu bài, nên xin nhiều hơn không lấy thêm được bài nào — con số thừa ra
  // chỉ im lặng không có tác dụng. Cảnh báo chứ không chặn: đặt trần 100 cho
  // một kênh đang nhìn 20 bài là cách hợp lệ để nói "coi như không giới hạn".
  const window = Number(value.scanLimit) || defaultScanLimit;
  const over = (raw: string) => {
    const n = Number(raw);
    return raw !== "" && Number.isFinite(n) && n > window;
  };
  const overNote = (what: string) =>
    `${what} (${window}) — phần vượt không có tác dụng vì mỗi vòng chỉ nhìn thấy ${window} bài. Tăng "Số bài nhìn mỗi vòng quét" nếu thật sự cần.`;

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field
        label="Số bài nhìn mỗi vòng quét"
        hint={`Cửa sổ quét: lấy về bấy nhiêu bài mới nhất rồi mới lọc. Bỏ trống = mặc định hệ thống (${defaultScanLimit}).`}
      >
        <Input
          type="number"
          min={1}
          max={200}
          placeholder={String(defaultScanLimit)}
          value={value.scanLimit}
          onChange={(e) => set({ scanLimit: e.target.value })}
        />
      </Field>

      <Field
        label="Lấy bài cũ khi thêm kênh"
        error={
          !backfillDone && over(value.backfillLimit)
            ? overNote("Lớn hơn cửa sổ quét")
            : undefined
        }
        hint={
          backfillDone
            ? "Vòng quét đầu đã chạy xong — đổi số này không còn tác dụng."
            : "0 = chỉ lấy bài đăng SAU khi thêm kênh. Đặt N để lấy thêm N bài gần nhất đã đăng từ trước."
        }
      >
        <Input
          type="number"
          min={0}
          max={200}
          placeholder="0"
          disabled={backfillDone}
          value={value.backfillLimit}
          onChange={(e) => set({ backfillLimit: e.target.value })}
        />
      </Field>

      <Field
        label="Trần Bài Post mỗi vòng"
        error={over(value.maxPostsPerRun) ? overNote("Lớn hơn cửa sổ quét") : undefined}
        hint="Bỏ trống = không giới hạn, lấy hết bài mới. Đặt số để chặn nổ chi phí AI khi kênh đăng ồ ạt — phần dư để vòng sau xử lý tiếp."
      >
        <Input
          type="number"
          min={1}
          placeholder="không giới hạn"
          value={value.maxPostsPerRun}
          onChange={(e) => set({ maxPostsPerRun: e.target.value })}
        />
      </Field>
    </div>
  );
}
