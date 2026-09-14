"use client";

import * as React from "react";

import { cn } from "@/lib/utils";

const controlClass =
  "w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 " +
  "placeholder:text-slate-400 focus:border-indigo-600 focus:outline-2 focus:outline-offset-0 " +
  "focus:outline-indigo-600 disabled:bg-slate-50 disabled:text-slate-500";

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn("mb-1.5 block text-sm font-medium text-slate-700", className)}
      {...props}
    />
  );
}

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(controlClass, "h-10", className)} {...props} />;
}

export function Textarea({
  className,
  ...props
}: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(controlClass, "min-h-24", className)} {...props} />;
}

export function Select({ className, ...props }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(controlClass, "h-10", className)} {...props} />;
}

export function Checkbox({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      type="checkbox"
      className={cn("size-4 rounded border-slate-300 text-indigo-700", className)}
      {...props}
    />
  );
}

export function Field({
  label,
  hint,
  error,
  required,
  children,
}: {
  label: React.ReactNode;
  hint?: string;
  /**
   * Cảnh báo về CHÍNH giá trị đang nhập, hiện đỏ ngay dưới ô.
   *
   * Thay chỗ của hint chứ không xếp thêm một dòng: hai dòng chữ nhỏ nằm chồng
   * nhau thì dòng đỏ không còn nổi hơn dòng xám bao nhiêu, mà đó là cả lý do
   * nó tồn tại.
   */
  error?: React.ReactNode;
  /** Đánh dấu * đỏ cạnh nhãn — trường bắt buộc điền. */
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <Label>
        {label}
        {required ? (
          <span className="ml-0.5 text-red-600" aria-hidden="true">
            *
          </span>
        ) : null}
      </Label>
      {children}
      {error ? (
        <p className="mt-1 text-xs font-medium text-red-700">{error}</p>
      ) : hint ? (
        <p className="mt-1 text-xs text-slate-500">{hint}</p>
      ) : null}
    </div>
  );
}

/**
 * Toggle — công tắc bật/tắt. Dùng cho trạng thái nhị phân đọc-hiểu-ngay
 * (tài khoản bật/tắt), thay vì nút phải đọc chữ mới biết đang ở trạng thái nào.
 */
export function Toggle({
  checked,
  onChange,
  disabled,
  label,
  className,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  disabled?: boolean;
  /** Nhãn hiện cạnh công tắc; cũng là nhãn cho screen reader. */
  label?: string;
  className?: string;
}) {
  return (
    <label className={cn("inline-flex items-center gap-2", disabled && "opacity-50", className)}>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn(
          "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors",
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600",
          disabled ? "cursor-not-allowed" : "cursor-pointer",
          checked ? "bg-indigo-700" : "bg-slate-300",
        )}
      >
        <span
          className={cn(
            "inline-block size-4 rounded-full bg-white shadow transition-transform",
            checked ? "translate-x-4.5" : "translate-x-0.5",
          )}
        />
      </button>
      {label ? <span className="text-sm text-slate-700">{label}</span> : null}
    </label>
  );
}
