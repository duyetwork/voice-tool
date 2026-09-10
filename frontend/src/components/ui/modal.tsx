"use client";

import * as React from "react";

import { Card, CardBody } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const WIDTHS = {
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-xl",
  "2xl": "max-w-2xl",
  "3xl": "max-w-3xl",
} as const;

/**
 * Modal — hộp thoại dùng chung cho các form không đáng chiếm 1 trang riêng
 * (thêm kênh, tạo voice, sửa metadata).
 *
 * Đóng được bằng Esc và bằng cách bấm ra ngoài; bấm bên trong không đóng để
 * người dùng không mất form đang gõ nửa vời.
 */
export function Modal({
  title,
  description,
  width = "lg",
  onClose,
  children,
}: {
  title: string;
  description?: React.ReactNode;
  width?: keyof typeof WIDTHS;
  onClose: () => void;
  children: React.ReactNode;
}) {
  React.useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-900/40 p-4 sm:items-center"
      onClick={onClose}
    >
      <Card
        className={cn("my-auto w-full", WIDTHS[width])}
        onClick={(e) => e.stopPropagation()}
      >
        <CardBody>
          <div className="mb-4 flex items-start justify-between gap-4">
            <div>
              <h2 className="text-base font-semibold text-slate-900">{title}</h2>
              {description ? (
                <p className="mt-0.5 text-xs text-slate-500">{description}</p>
              ) : null}
            </div>
            <button
              type="button"
              onClick={onClose}
              aria-label="Đóng"
              className="-mt-1 rounded-md px-2 py-1 text-lg leading-none text-slate-400 hover:bg-slate-100 hover:text-slate-700"
            >
              ×
            </button>
          </div>
          {children}
        </CardBody>
      </Card>
    </div>
  );
}
