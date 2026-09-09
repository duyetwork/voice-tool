"use client";

import * as React from "react";

import { usePermissions } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/field";
import type { Permissions } from "@/types/api";

/**
 * useSelection quản lý chọn nhiều dòng để thao tác hàng loạt (prompt.md mục 8).
 *
 * Lựa chọn tự loại bỏ dòng không còn trong danh sách hiện tại (đổi bộ lọc, xoá)
 * để không lỡ tay chạy hành động trên id đã biến mất.
 */
export function useSelection<T extends { id: string }>(items: T[] | undefined) {
  const [selected, setSelected] = React.useState<string[]>([]);
  const ids = React.useMemo(() => (items ?? []).map((i) => i.id), [items]);

  React.useEffect(() => {
    setSelected((prev) => prev.filter((id) => ids.includes(id)));
  }, [ids]);

  const toggle = React.useCallback((id: string) => {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }, []);

  const toggleAll = React.useCallback(() => {
    setSelected((prev) => (prev.length === ids.length ? [] : ids));
  }, [ids]);

  return {
    selected,
    count: selected.length,
    isSelected: (id: string) => selected.includes(id),
    allSelected: ids.length > 0 && selected.length === ids.length,
    toggle,
    toggleAll,
    clear: () => setSelected([]),
    /** Các phần tử đang chọn, theo đúng thứ tự đang hiển thị. */
    items: (items ?? []).filter((i) => selected.includes(i.id)),
  };
}

export type Selection<T extends { id: string }> = ReturnType<typeof useSelection<T>>;

/** Ô checkbox ở header bảng — chọn/bỏ chọn tất cả dòng đang hiển thị. */
export function SelectAllBox({ selection }: { selection: { allSelected: boolean; toggleAll: () => void } }) {
  return (
    <Checkbox
      checked={selection.allSelected}
      onChange={selection.toggleAll}
      aria-label="Chọn tất cả"
    />
  );
}

export interface BulkAction {
  label: string;
  onRun: (ids: string[]) => Promise<void> | void;
  /** confirm hiện hộp xác nhận trước khi chạy (dùng cho hành động xoá). */
  confirm?: string;
  variant?: "primary" | "secondary" | "ghost";
  /** Quyền cần có để thấy hành động này — ví dụ xoá cần `can_delete`. */
  permission?: keyof Permissions;
}

/**
 * BulkBar hiện khi có dòng được chọn. Mỗi hành động chạy tuần tự trên từng id
 * và báo lại số thành công/thất bại — không dừng giữa chừng vì 1 dòng lỗi.
 */
export function BulkBar({
  count,
  ids,
  actions,
  onDone,
}: {
  count: number;
  ids: string[];
  actions: BulkAction[];
  onDone: () => void;
}) {
  const [running, setRunning] = React.useState<string | null>(null);
  const [result, setResult] = React.useState<string | null>(null);
  const { perms } = usePermissions();

  // Ẩn hành động vượt quyền (user không được xoá) thay vì để bấm rồi ăn 403.
  const allowed = actions.filter((a) => !a.permission || perms[a.permission]);

  if (count === 0 || allowed.length === 0) return null;

  async function run(action: BulkAction) {
    if (action.confirm && !confirm(`${action.confirm} (${ids.length} mục)`)) return;

    setRunning(action.label);
    setResult(null);

    let ok = 0;
    const errors: string[] = [];
    for (const id of ids) {
      try {
        await action.onRun([id]);
        ok += 1;
      } catch (e) {
        errors.push(e instanceof Error ? e.message : String(e));
      }
    }

    setRunning(null);
    setResult(
      errors.length === 0
        ? `${action.label}: xong ${ok} mục.`
        : `${action.label}: xong ${ok}, lỗi ${errors.length} — ${errors[0]}`,
    );
    onDone();
  }

  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-indigo-200 bg-indigo-50 px-4 py-2.5">
      <span className="text-sm font-medium text-indigo-900">Đã chọn {count} mục</span>
      {allowed.map((action) => (
        <Button
          key={action.label}
          size="sm"
          variant={action.variant ?? "secondary"}
          disabled={running !== null}
          onClick={() => run(action)}
        >
          {running === action.label ? "Đang chạy…" : action.label}
        </Button>
      ))}
      {result ? <span className="text-sm text-indigo-900">{result}</span> : null}
    </div>
  );
}
