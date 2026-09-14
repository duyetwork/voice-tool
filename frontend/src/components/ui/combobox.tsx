"use client";

import * as React from "react";

import { cn } from "@/lib/utils";

/**
 * Combobox — ô chọn có thanh tìm kiếm ngay trong chính nó.
 *
 * Vì sao không dùng `<select>`: danh sách ngôn ngữ gần 80 dòng, quốc gia hơn
 * 200, hashtag có thể vài trăm. Với `<select>` người dùng chỉ còn cách cuộn —
 * gõ để nhảy tới chỉ khớp được ký tự đầu và không tìm được theo từ ở giữa.
 *
 * Toàn bộ việc lọc chạy trên dữ liệu ĐÃ CÓ SẴN ở trình duyệt: không gọi API khi
 * mở dropdown, cũng không gọi theo từng ký tự gõ.
 */

export interface ComboOption {
  value: string;
  label: string;
  /** Dòng phụ hiện mờ bên phải (vd số lần hashtag đã dùng). */
  hint?: string;
}

/** matches: lọc không phân biệt hoa thường và bỏ dấu — "viet" khớp "Việt Nam". */
function matches(option: ComboOption, keyword: string): boolean {
  if (!keyword) return true;
  const needle = fold(keyword);
  return fold(option.label).includes(needle) || fold(option.value).includes(needle);
}

/**
 * fold bỏ dấu tiếng Việt để gõ không dấu vẫn tìm ra.
 *
 * Người dùng gõ nhanh thường không bỏ dấu; bắt họ gõ đúng "Việt Nam" mới ra kết
 * quả là làm thanh tìm kiếm mất phần lớn tác dụng.
 */
function fold(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .replace(/đ/gu, "d");
}

// searchDebounceMs — chờ ngừng gõ bao lâu rồi mới hỏi server.
const searchDebounceMs = 250;

/** useDebounced trả giá trị đã trễ, để mỗi phím gõ không thành một request. */
function useDebounced<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = React.useState(value);
  React.useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(t);
  }, [value, delay]);
  return debounced;
}

/** useOutsideClose đóng dropdown khi bấm ra ngoài — nó không có backdrop riêng. */
function useOutsideClose(open: boolean, onClose: () => void) {
  const box = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (!box.current?.contains(e.target as Node)) onClose();
    }
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open, onClose]);
  return box;
}

const controlClass =
  "flex h-10 w-full items-center justify-between gap-2 rounded-md border border-slate-300 " +
  "bg-white px-3 text-left text-sm text-slate-900 focus:border-indigo-600 " +
  "focus:outline-2 focus:outline-indigo-600 disabled:bg-slate-50 disabled:text-slate-500";

const panelClass =
  "absolute z-20 mt-1 max-h-72 w-full overflow-hidden rounded-md border border-slate-200 " +
  "bg-white shadow-lg";

/**
 * Combobox — chọn MỘT giá trị.
 *
 * Bấm vào ô là ô biến thành thanh tìm kiếm ngay tại chỗ (không mọc thêm một ô
 * thứ hai), nên chỗ người dùng đang nhìn cũng là chỗ họ gõ.
 */
export function Combobox({
  value,
  options,
  onChange,
  placeholder = "— Chọn —",
  emptyLabel,
  disabled,
  className,
}: {
  value: string;
  options: ComboOption[];
  onChange: (next: string) => void;
  placeholder?: string;
  /** Dòng đầu cho phép bỏ chọn; bỏ trống thì không có lựa chọn rỗng. */
  emptyLabel?: string;
  disabled?: boolean;
  className?: string;
}) {
  const [open, setOpen] = React.useState(false);
  const [keyword, setKeyword] = React.useState("");
  const close = React.useCallback(() => {
    setOpen(false);
    setKeyword("");
  }, []);
  const box = useOutsideClose(open, close);

  const selected = options.find((o) => o.value === value);
  const shown = React.useMemo(() => options.filter((o) => matches(o, keyword)), [options, keyword]);

  function pick(next: string) {
    onChange(next);
    close();
  }

  return (
    <div className={cn("relative", className)} ref={box}>
      {open ? (
        <input
          autoFocus
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") close();
            // Enter chọn kết quả đầu tiên: gõ xong là xong, không phải rời tay
            // khỏi bàn phím để bấm chuột.
            if (e.key === "Enter" && shown[0]) {
              e.preventDefault();
              pick(shown[0].value);
            }
          }}
          placeholder="Gõ để tìm…"
          className={controlClass}
        />
      ) : (
        <button
          type="button"
          disabled={disabled}
          onClick={() => setOpen(true)}
          className={controlClass}
        >
          <span className={cn("truncate", !selected && "text-slate-400")}>
            {selected?.label ?? placeholder}
          </span>
          <span className="shrink-0 text-slate-400">▾</span>
        </button>
      )}

      {open ? (
        <div className={panelClass}>
          <div className="max-h-72 overflow-y-auto py-1">
            {emptyLabel ? (
              <Row selected={!value} onPick={() => pick("")}>
                <span className="text-slate-500">{emptyLabel}</span>
              </Row>
            ) : null}
            {shown.length ? (
              shown.map((o) => (
                <Row key={o.value} selected={o.value === value} onPick={() => pick(o.value)}>
                  <span className="truncate">{o.label}</span>
                  {o.hint ? (
                    <span className="shrink-0 text-xs text-slate-400">{o.hint}</span>
                  ) : null}
                </Row>
              ))
            ) : (
              <p className="px-3 py-2 text-sm text-slate-500">Không có mục nào khớp.</p>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}

/**
 * MultiCombobox — chọn NHIỀU giá trị, các mục đã chọn hiện thành thẻ ngay
 * trong ô.
 *
 * `allowCreate` cho phép biến từ khoá đang gõ thành một mục mới: hashtag là
 * danh mục mở — nó lớn dần theo những gì người dùng đặt ra, nên chặn ở danh
 * sách có sẵn là chặn đúng cái việc họ đang làm.
 */
export function MultiCombobox({
  values,
  options,
  onChange,
  onSearch,
  placeholder = "— Chọn —",
  allowCreate,
  normalize = (s) => s.trim(),
  disabled,
  className,
}: {
  values: string[];
  options: ComboOption[];
  onChange: (next: string[]) => void;
  /**
   * onSearch báo từ khoá ra ngoài để người gọi tự nạp `options`.
   *
   * Dùng khi danh mục quá lớn để nằm hết trong trình duyệt (hashtag MultiMe có
   * ~94.000 mục). Có `onSearch` thì component KHÔNG tự lọc nữa — nó tin
   * `options` là kết quả đã lọc, không thì lọc hai lần và lần thứ hai loại mất
   * kết quả server vừa trả.
   */
  onSearch?: (keyword: string) => void;
  placeholder?: string;
  allowCreate?: boolean;
  /** Chuẩn hoá giá trị tự gõ trước khi thêm (bỏ '#', hạ chữ thường…). */
  normalize?: (raw: string) => string;
  disabled?: boolean;
  className?: string;
}) {
  const [open, setOpen] = React.useState(false);
  const [keyword, setKeyword] = React.useState("");
  const debounced = useDebounced(keyword, searchDebounceMs);

  // Giữ callback trong ref để effect chỉ chạy lại khi TỪ KHOÁ đổi. Đưa thẳng
  // `onSearch` vào deps thì mỗi lần cha render lại (hàm mới) là một lần gọi
  // thừa — và người gọi thường truyền thẳng hàm inline.
  const onSearchRef = React.useRef(onSearch);
  onSearchRef.current = onSearch;

  // Báo từ khoá ra ngoài sau khi ngừng gõ — mỗi phím một request là vô ích khi
  // người dùng còn đang gõ dở.
  React.useEffect(() => {
    onSearchRef.current?.(debounced);
  }, [debounced]);
  const close = React.useCallback(() => {
    setOpen(false);
    setKeyword("");
  }, []);
  const box = useOutsideClose(open, close);

  // Lọc phía ngoài (onSearch) thì `options` đã là kết quả — lọc lại ở đây sẽ
  // loại mất chính thứ server vừa trả về cho từ khoá đó.
  const shown = React.useMemo(
    () => (onSearch ? options : options.filter((o) => matches(o, keyword))),
    [options, keyword, onSearch],
  );

  // Từ khoá chưa có trong danh sách -> cho thêm mới.
  const typed = normalize(keyword);
  const canCreate =
    allowCreate &&
    typed.length > 0 &&
    !values.includes(typed) &&
    !options.some((o) => o.value === typed);

  function toggle(next: string) {
    onChange(values.includes(next) ? values.filter((v) => v !== next) : [...values, next]);
    setKeyword("");
  }

  return (
    <div className={cn("relative", className)} ref={box}>
      <div
        role="button"
        tabIndex={0}
        onClick={() => !disabled && setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") setOpen(true);
        }}
        className={cn(
          "flex min-h-10 w-full flex-wrap items-center gap-1.5 rounded-md border border-slate-300",
          "bg-white px-2 py-1.5 text-sm text-slate-900",
          disabled && "bg-slate-50 text-slate-500",
        )}
      >
        {values.length ? (
          values.map((v) => (
            <span
              key={v}
              className="inline-flex items-center gap-1 rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-800"
            >
              {v}
              <button
                type="button"
                aria-label={`Bỏ ${v}`}
                onClick={(e) => {
                  e.stopPropagation();
                  onChange(values.filter((x) => x !== v));
                }}
                className="text-indigo-500 hover:text-indigo-900"
              >
                ×
              </button>
            </span>
          ))
        ) : open ? null : (
          <span className="text-slate-400">{placeholder}</span>
        )}

        {open ? (
          <input
            autoFocus
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") close();
              if (e.key === "Enter") {
                e.preventDefault();
                if (shown[0] && !canCreate) toggle(shown[0].value);
                else if (canCreate) toggle(typed);
              }
              // Backspace ở ô rỗng gỡ thẻ cuối — thói quen từ mọi ô tag khác.
              if (e.key === "Backspace" && !keyword && values.length) {
                onChange(values.slice(0, -1));
              }
            }}
            placeholder="Gõ để tìm…"
            className="min-w-24 flex-1 border-0 p-0 text-sm outline-none placeholder:text-slate-400"
          />
        ) : null}
      </div>

      {open ? (
        <div className={panelClass}>
          <div className="max-h-72 overflow-y-auto py-1">
            {canCreate ? (
              <Row selected={false} onPick={() => toggle(typed)}>
                <span>
                  Thêm <b>{typed}</b>
                </span>
              </Row>
            ) : null}
            {shown.length ? (
              shown.map((o) => (
                <Row
                  key={o.value}
                  selected={values.includes(o.value)}
                  onPick={() => toggle(o.value)}
                >
                  <span className="truncate">{o.label}</span>
                  {o.hint ? (
                    <span className="shrink-0 text-xs text-slate-400">{o.hint}</span>
                  ) : null}
                </Row>
              ))
            ) : canCreate ? null : (
              <p className="px-3 py-2 text-sm text-slate-500">Không có mục nào khớp.</p>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function Row({
  selected,
  onPick,
  children,
}: {
  selected: boolean;
  onPick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onPick}
      className={cn(
        "flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left text-sm",
        selected ? "bg-indigo-50 font-medium text-indigo-800" : "text-slate-700 hover:bg-slate-50",
      )}
    >
      {children}
    </button>
  );
}
