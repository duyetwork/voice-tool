"use client";

import * as React from "react";

import { Checkbox, Field, Input, Select } from "@/components/ui/field";
import type { ChannelSchedule } from "@/types/api";

/**
 * Khung giờ quét của 1 kênh — dùng chung cho cả Breaking lẫn Định kỳ.
 *
 * Kênh tin tức không đăng lúc 3h sáng; quét lúc đó là đốt hạn mức và mời gọi
 * rate-limit đúng vào lúc chẳng thu được gì. Và khung giờ phải đi kèm MÚI GIỜ:
 * lịch chạy theo giờ container (UTC), nên "6h–23h" mà không nói múi giờ thì
 * lệch 7 tiếng so với ý người dùng.
 *
 * Bỏ trống cả hai đầu = quét 24/7, đúng hành vi trước khi có tính năng này.
 */

/** Múi giờ hay dùng. Không đổ cả danh sách IANA: 99% kênh nằm trong vài múi này. */
const TIMEZONES = [
  { value: "Asia/Ho_Chi_Minh", label: "Việt Nam (GMT+7)" },
  { value: "Asia/Bangkok", label: "Thái Lan (GMT+7)" },
  { value: "Asia/Singapore", label: "Singapore (GMT+8)" },
  { value: "Asia/Tokyo", label: "Nhật Bản (GMT+9)" },
  { value: "Europe/London", label: "Anh (GMT+0/+1)" },
  { value: "America/New_York", label: "Mỹ — miền Đông" },
  { value: "UTC", label: "UTC" },
];

/** 0 = Chủ nhật … 6 = Thứ bảy — đúng quy ước của Postgres và của Go. */
const WEEKDAYS = [
  { value: 1, label: "T2" },
  { value: 2, label: "T3" },
  { value: 3, label: "T4" },
  { value: 4, label: "T5" },
  { value: 5, label: "T6" },
  { value: 6, label: "T7" },
  { value: 0, label: "CN" },
];

/**
 * ScheduleDraft là hình dạng của FORM, không phải của API.
 *
 * Giờ ở đây là chuỗi "HH:MM" vì đó là thứ <input type="time"> nhận và trả;
 * việc đổi sang số phút để gửi lên nằm gọn trong `toChannelSchedule`.
 */
export interface ScheduleDraft {
  timezone: string;
  from: string;
  to: string;
  weekdays: number[];
  fixedTimes: string[];
}

/**
 * emptySchedule là lịch của kênh MỚI, và nó ĐIỀN SẴN 24/7 chứ không để trống.
 *
 * Ô trống với ô điền "00:00–23:59, cả 7 ngày" mô tả cùng một hành vi, nhưng ô
 * trống bắt người dùng đoán xem trống nghĩa là gì — và với một cặp giờ thì
 * "trống" hoàn toàn có thể đọc thành "không quét giờ nào". Điền sẵn thì cái
 * đang chạy hiện ra ngay, và thu hẹp khung giờ chỉ là sửa hai con số.
 */
export const emptySchedule: ScheduleDraft = {
  timezone: "Asia/Ho_Chi_Minh",
  from: "00:00",
  to: "23:59",
  weekdays: [0, 1, 2, 3, 4, 5, 6],
  fixedTimes: [],
};

/** minutesOf đổi "06:30" -> 390; chuỗi rỗng hoặc sai -> null. */
function minutesOf(clock: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(clock.trim());
  if (!m) return null;
  const value = Number(m[1]) * 60 + Number(m[2]);
  return value >= 0 && value < 1440 ? value : null;
}

/** clockOf đổi 390 -> "06:30". */
export function clockOf(minutes: number): string {
  return `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
}

/**
 * toChannelSchedule đổi form sang payload API.
 *
 * `clear_window` là cờ riêng chứ không dựa vào null: trong JSON, thiếu trường
 * vừa có nghĩa "không sửa" vừa có nghĩa "xoá", và chỉ một trong hai diễn giải
 * được — không có cờ thì người dùng không bao giờ bỏ được khung giờ đã đặt.
 */
export function toChannelSchedule(draft: ScheduleDraft): ChannelSchedule {
  const from = minutesOf(draft.from);
  const to = minutesOf(draft.to);
  const hasWindow = from !== null && to !== null;

  return {
    timezone: draft.timezone,
    active_from_min: hasWindow ? from : null,
    active_to_min: hasWindow ? to : null,
    active_weekdays: draft.weekdays.length ? [...draft.weekdays].sort((a, b) => a - b) : [],
    // Luôn RỖNG: "giờ chạy cố định" đã bị bỏ khỏi form, nên mỗi lần lưu là một
    // lần dọn sạch giá trị cũ. Để lại giá trị mà không còn ô nào sửa nghĩa là
    // kênh chạy theo một lịch người dùng không nhìn thấy và không gỡ được.
    fixed_times_min: [],
    clear_window: !hasWindow,
  };
}

/** scheduleDraftOf dựng lại form từ bản ghi kênh đã lưu. */
export function scheduleDraftOf(list: {
  timezone?: string | null;
  active_from_min?: number | null;
  active_to_min?: number | null;
  active_weekdays?: number[] | null;
  fixed_times_min?: number[] | null;
}): ScheduleDraft {
  return {
    timezone: list.timezone || emptySchedule.timezone,
    // Kênh lưu NULL nghĩa là 24/7 — hiện đúng khung đó thay vì ô trống, để thứ
    // đang chạy và thứ nhìn thấy là một.
    from: list.active_from_min != null ? clockOf(list.active_from_min) : emptySchedule.from,
    to: list.active_to_min != null ? clockOf(list.active_to_min) : emptySchedule.to,
    weekdays: list.active_weekdays?.length ? list.active_weekdays : emptySchedule.weekdays,
    fixedTimes: (list.fixed_times_min ?? []).map(clockOf),
  };
}

/** describeSchedule — câu mô tả ngắn cho bảng danh sách kênh. */
export function describeSchedule(list: {
  timezone?: string | null;
  active_from_min?: number | null;
  active_to_min?: number | null;
  active_weekdays?: number[] | null;
  fixed_times_min?: number[] | null;
}): string {
  const parts: string[] = [];
  if (list.fixed_times_min?.length) {
    parts.push("chạy lúc " + list.fixed_times_min.map(clockOf).join(", "));
  }
  if (list.active_from_min != null && list.active_to_min != null) {
    parts.push(`${clockOf(list.active_from_min)}–${clockOf(list.active_to_min)}`);
  }
  if (list.active_weekdays?.length) {
    const names = list.active_weekdays
      .map((d) => WEEKDAYS.find((w) => w.value === d)?.label ?? String(d))
      .join(", ");
    parts.push(names);
  }
  return parts.length ? parts.join(" · ") : "24/7";
}

/**
 * ScheduleFields — khung giờ quét của kênh.
 *
 * "Giờ chạy cố định" đã bị bỏ khỏi form: nó là kiểu lịch THỨ HAI, chạy theo mốc
 * đồng hồ và làm tần suất "mỗi N phút" bị bỏ qua — hai cách hẹn lịch cùng nằm
 * trong một hộp thoại, cái này lặng lẽ vô hiệu hoá cái kia. Kênh nào còn giá
 * trị cũ thì lần lưu tiếp theo xoá sạch (form luôn gửi mảng rỗng).
 */
export function ScheduleFields({
  value,
  onChange,
}: {
  value: ScheduleDraft;
  onChange: (next: ScheduleDraft) => void;
}) {
  const [open, setOpen] = React.useState(false);

  function toggleDay(day: number) {
    onChange({
      ...value,
      weekdays: value.weekdays.includes(day)
        ? value.weekdays.filter((d) => d !== day)
        : [...value.weekdays, day],
    });
  }

  return (
    <div className="rounded-md border border-slate-200 p-3">
      <button
        type="button"
        className="text-sm font-medium text-slate-700"
        onClick={() => setOpen((v) => !v)}
      >
        {open ? "▾" : "▸"} Lịch quét — {describeDraft(value)}
      </button>

      {open ? (
        <div className="mt-3 space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Múi giờ" hint="Khung giờ bên dưới tính theo múi giờ này.">
              <Select
                value={value.timezone}
                onChange={(e) => onChange({ ...value, timezone: e.target.value })}
              >
                {TIMEZONES.map((tz) => (
                  <option key={tz.value} value={tz.value}>
                    {tz.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Bắt đầu quét" hint="Mặc định 00:00–23:59 = quét cả ngày.">
              <Input
                type="time"
                value={value.from}
                onChange={(e) => onChange({ ...value, from: e.target.value })}
              />
            </Field>
            <Field label="Ngừng quét" hint="Đặt sớm hơn giờ bắt đầu = khung vắt qua nửa đêm.">
              <Input
                type="time"
                value={value.to}
                onChange={(e) => onChange({ ...value, to: e.target.value })}
              />
            </Field>
          </div>

          <Field label="Ngày trong tuần" hint="Mặc định cả 7 ngày. Bỏ tick hết cũng là quét mọi ngày.">
            <div className="flex flex-wrap gap-3">
              {WEEKDAYS.map((d) => (
                <label key={d.value} className="flex items-center gap-1.5 text-sm text-slate-700">
                  <Checkbox
                    checked={value.weekdays.includes(d.value)}
                    onChange={() => toggleDay(d.value)}
                  />
                  {d.label}
                </label>
              ))}
            </div>
          </Field>

        </div>
      ) : null}
    </div>
  );
}

function describeDraft(draft: ScheduleDraft): string {
  const parts: string[] = [];
  if (draft.from && draft.to) parts.push(`${draft.from}–${draft.to}`);
  if (draft.weekdays.length) {
    parts.push(
      [...draft.weekdays]
        .sort((a, b) => a - b)
        .map((d) => WEEKDAYS.find((w) => w.value === d)?.label ?? String(d))
        .join(", "),
    );
  }
  return parts.length ? parts.join(" · ") : "24/7 (mọi ngày, mọi giờ)";
}
