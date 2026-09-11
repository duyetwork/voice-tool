"use client";

import * as React from "react";

import { cn } from "@/lib/utils";

export function Table({ children }: { children: React.ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">{children}</table>
    </div>
  );
}

export function Th({ className, ...props }: React.ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      className={cn(
        "border-b border-slate-200 px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500",
        className,
      )}
      {...props}
    />
  );
}

export function Td({ className, ...props }: React.TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td
      className={cn("border-b border-slate-100 px-3 py-2.5 align-top text-slate-700", className)}
      {...props}
    />
  );
}

export function EmptyRow({ colSpan, children }: { colSpan: number; children: React.ReactNode }) {
  return (
    <tr>
      <td colSpan={colSpan} className="px-4 py-10 text-center text-sm text-slate-500">
        {children}
      </td>
    </tr>
  );
}

/**
 * DateCell hiện ngày và giờ trên 2 dòng.
 *
 * Bảng có tới 2 cột thời gian (tạo lúc / đăng lúc); để 1 dòng
 * "09/09/2026, 14:05" thì mỗi cột chiếm ~140px và bảng tràn ngang trên màn
 * hình thường. Tách 2 dòng còn ~70px mà vẫn đọc đủ.
 */
export function DateCell({ value }: { value?: string | null }) {
  if (!value) return <span className="text-slate-400">—</span>;
  const d = new Date(value);
  return (
    <span className="block whitespace-nowrap leading-tight" title={d.toLocaleString("vi-VN")}>
      {d.toLocaleDateString("vi-VN")}
      <span className="block text-xs text-slate-500">
        {d.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit" })}
      </span>
    </span>
  );
}

/**
 * RowActions xếp các nút thao tác của 1 dòng thành 1 cột căn phải, mọi nút
 * rộng bằng nhau nên các dòng thẳng hàng với nhau thành 1 cột nút gọn.
 */
export function RowActions({ children }: { children: React.ReactNode }) {
  return <div className="ml-auto flex w-max flex-col items-stretch gap-1.5">{children}</div>;
}

// ---------------------------------------------------------------------------
// Sắp xếp
// ---------------------------------------------------------------------------

export type SortDir = "asc" | "desc";

/**
 * useSorting giữ cột + chiều sắp xếp của 1 bảng và trả ra `params` để ghép
 * thẳng vào filter của query.
 *
 * `onChange` dùng để reset phân trang: đổi thứ tự mà vẫn ở trang 5 thì người
 * dùng nhìn vào giữa danh sách, không hiểu đang xem gì.
 */
export function useSorting(defaultColumn: string, onChange?: () => void) {
  const [sort, setSort] = React.useState(defaultColumn);
  const [dir, setDir] = React.useState<SortDir>("desc");

  const toggle = React.useCallback(
    (column: string) => {
      if (column === sort) {
        setDir((d) => (d === "desc" ? "asc" : "desc"));
      } else {
        setSort(column);
        // Cột thời gian: lần đầu bấm luôn là mới nhất trước.
        setDir("desc");
      }
      onChange?.();
    },
    [sort, onChange],
  );

  return { sort, dir, toggle, params: { sort, dir } };
}

export type Sorting = ReturnType<typeof useSorting>;

/**
 * SortButton — nhãn bấm được để đổi thứ tự, KHÔNG kèm ô <th>.
 *
 * Tách khỏi SortableTh để một ô tiêu đề chứa được nhiều mốc sắp xếp: bảng
 * Voice gộp "Tạo lúc" và "Đăng lúc" vào chung 1 cột nhưng vẫn phải sắp xếp
 * được theo từng mốc.
 */
export function SortButton({
  sorting,
  column,
  className,
  children,
}: {
  sorting: Sorting;
  column: string;
  className?: string;
  children: React.ReactNode;
}) {
  const active = sorting.sort === column;
  return (
    <button
      type="button"
      onClick={() => sorting.toggle(column)}
      className={cn(
        "flex items-center gap-1 text-left uppercase tracking-wide hover:text-slate-900",
        className,
      )}
      aria-label={`Sắp xếp theo ${column}`}
    >
      {children}
      <span className={active ? "text-indigo-700" : "text-slate-300"} aria-hidden>
        {active ? (sorting.dir === "desc" ? "▼" : "▲") : "⇅"}
      </span>
    </button>
  );
}

/** SortableTh — ô tiêu đề bấm được để đổi thứ tự. */
export function SortableTh({
  sorting,
  column,
  className,
  children,
}: {
  sorting: Sorting;
  column: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Th className={cn("p-0", className)}>
      <SortButton sorting={sorting} column={column} className="w-full px-3 py-2">
        {children}
      </SortButton>
    </Th>
  );
}
