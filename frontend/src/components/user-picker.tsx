"use client";

import * as React from "react";

import { Checkbox, Input } from "@/components/ui/field";
import { useUsers } from "@/hooks/use-api";

/**
 * UserPicker — dropdown tick chọn người dùng (chỉ admin thấy).
 *
 * Không dùng `<select multiple>`: nó cao bằng cả danh sách, và trên mọi trình
 * duyệt đều phải giữ Ctrl để chọn nhiều — người dùng không đoán ra được. Ở đây
 * là 1 ô bấm ra danh sách có checkbox, đọc là hiểu.
 *
 * `single` biến nó thành chọn-một cho lúc chuyển key sang người khác.
 */
export function UserPicker({
  selected,
  onChange,
  single,
}: {
  selected: string[];
  onChange: (next: string[]) => void;
  single?: boolean;
}) {
  const [open, setOpen] = React.useState(false);
  const [keyword, setKeyword] = React.useState("");
  const box = React.useRef<HTMLDivElement>(null);
  // Danh sách tài khoản của cả hệ thống thường vài chục dòng — lấy 1 lần rồi
  // lọc tại chỗ, không phải gọi lại API theo từng ký tự gõ.
  const users = useUsers({ limit: 200 });

  // Bấm ra ngoài thì đóng — dropdown không có backdrop riêng.
  React.useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const items = users.data?.items ?? [];
  const shown = keyword
    ? items.filter((u) => u.email.toLowerCase().includes(keyword.trim().toLowerCase()))
    : items;

  const label = selected.length
    ? items
        .filter((u) => selected.includes(u.id))
        .map((u) => u.email)
        .join(", ") || `${selected.length} người đã chọn`
    : single
      ? "— Chọn người dùng —"
      : "Chính tôi";

  function toggle(id: string) {
    if (single) {
      onChange([id]);
      setOpen(false);
      return;
    }
    onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  }

  return (
    <div className="relative" ref={box}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex h-10 w-full items-center justify-between gap-2 rounded-md border border-slate-300 bg-white px-3 text-left text-sm text-slate-900 focus:border-indigo-600 focus:outline-2 focus:outline-indigo-600"
      >
        <span className="truncate">{label}</span>
        <span className="text-slate-400">▾</span>
      </button>

      {open ? (
        <div className="absolute z-10 mt-1 w-full rounded-md border border-slate-200 bg-white shadow-lg">
          <div className="border-b border-slate-100 p-2">
            <Input
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder="Tìm theo email…"
              className="h-8"
            />
          </div>
          <div className="max-h-56 overflow-y-auto py-1">
            {users.isLoading ? (
              <p className="px-3 py-2 text-sm text-slate-500">Đang tải…</p>
            ) : shown.length ? (
              shown.map((user) => (
                <label
                  key={user.id}
                  className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-50"
                >
                  <Checkbox checked={selected.includes(user.id)} onChange={() => toggle(user.id)} />
                  <span className="truncate">{user.email}</span>
                </label>
              ))
            ) : (
              <p className="px-3 py-2 text-sm text-slate-500">Không có tài khoản nào khớp.</p>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
