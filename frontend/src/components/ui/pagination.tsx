"use client";

import * as React from "react";

import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/field";

/** Số dòng mỗi trang cho người dùng chọn. */
export const PAGE_SIZES = [10, 20, 50, 100] as const;

export const DEFAULT_PAGE_SIZE = 20;

/**
 * usePaging giữ trạng thái phân trang của 1 bảng.
 *
 * `reset` phải được gọi khi bộ lọc đổi: đang ở trang 5 mà lọc lại còn 12 dòng
 * thì bảng sẽ trống trơn dù có dữ liệu.
 */
export function usePaging(pageSize: number = DEFAULT_PAGE_SIZE) {
  const [limit, setLimitRaw] = React.useState(pageSize);
  const [offset, setOffset] = React.useState(0);

  const setLimit = React.useCallback((next: number) => {
    setLimitRaw(next);
    // Đổi số dòng/trang thì về trang đầu — offset cũ không còn ý nghĩa.
    setOffset(0);
  }, []);

  return {
    limit,
    offset,
    setLimit,
    setOffset,
    reset: React.useCallback(() => setOffset(0), []),
  };
}

export type Paging = ReturnType<typeof usePaging>;

/**
 * Pagination — thanh phân trang đặt dưới bảng.
 *
 * `total` là tổng số bản ghi khớp bộ lọc do API trả về (không phải số dòng
 * đang hiển thị), nên số trang là thật chứ không phải đoán theo trang hiện tại.
 */
export function Pagination({
  total,
  paging,
  unit = "dòng",
}: {
  total?: number;
  paging: Paging;
  /** Đơn vị hiển thị trong "… / 137 voice". */
  unit?: string;
}) {
  const { limit, offset, setLimit, setOffset } = paging;

  if (total === undefined) return null;

  const from = total === 0 ? 0 : offset + 1;
  const to = Math.min(offset + limit, total);
  const page = Math.floor(offset / limit) + 1;
  const pages = Math.max(1, Math.ceil(total / limit));

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 px-4 py-3">
      <div className="flex items-center gap-2 text-sm text-slate-600">
        <span>Dòng/trang</span>
        <Select
          className="h-8 w-20 text-xs"
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
          aria-label="Số dòng mỗi trang"
        >
          {PAGE_SIZES.map((size) => (
            <option key={size} value={size}>
              {size}
            </option>
          ))}
        </Select>
      </div>

      <span className="text-sm text-slate-500">
        {from}–{to} / {total} {unit}
      </span>

      {/* Nút chỉ hiện khi có trang để đi tới — nút xám không bấm được chỉ làm
          rối, còn 1 trang thì không cần nút nào. */}
      <div className="flex items-center gap-2">
        {offset > 0 ? (
          <Button
            size="sm"
            variant="secondary"
            onClick={() => setOffset(Math.max(0, offset - limit))}
          >
            ← Trước
          </Button>
        ) : null}
        <span className="text-sm text-slate-600">
          Trang {page}/{pages}
        </span>
        {to < total ? (
          <Button size="sm" variant="secondary" onClick={() => setOffset(offset + limit)}>
            Sau →
          </Button>
        ) : null}
      </div>
    </div>
  );
}
