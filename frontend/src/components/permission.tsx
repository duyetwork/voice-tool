"use client";

import * as React from "react";

import { useMe } from "@/hooks/use-api";
import type { Permissions, Role } from "@/types/api";

const FALLBACK: Permissions = {
  can_write: false,
  can_delete: false,
  can_manage_users: false,
  can_operate: false,
};

/**
 * usePermissions đọc quyền từ /me. Trong lúc chờ, mặc định KHÔNG có quyền —
 * an toàn hơn là hiện nút rồi mới bị API từ chối.
 */
export function usePermissions(): {
  role: Role | null;
  perms: Permissions;
  loading: boolean;
} {
  const me = useMe();
  return {
    role: me.data?.role ?? null,
    perms: me.data?.permissions ?? FALLBACK,
    loading: me.isLoading,
  };
}

/** Can ẩn phần UI mà role hiện tại không được phép dùng. */
export function Can({
  permission,
  children,
  fallback = null,
}: {
  permission: keyof Permissions;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}) {
  const { perms } = usePermissions();
  return perms[permission] ? <>{children}</> : <>{fallback}</>;
}

/**
 * NoPermission — dòng thông báo thay cho nội dung mà vai trò hiện tại không
 * được xem.
 *
 * Vì sao HIỆN THÔNG BÁO chứ không ẩn sạch mục đó: editor thấy đủ nhóm "Vận
 * hành" trên sidebar, nên một trang trống hoặc một tab biến mất sẽ trông như
 * hệ thống hỏng. Một dòng nói rõ "cần quyền admin" trả lời luôn câu hỏi tiếp
 * theo của họ — hỏi ai để được vào.
 *
 * Vai trò `user` thì ngược lại, cả nhóm bị ẩn khỏi sidebar: họ không có việc gì
 * ở đây, và hiện ra một danh sách toàn mục bị chặn chỉ là nhiễu.
 */
export function NoPermission({ title, children }: { title?: string; children?: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3">
      <p className="text-sm font-medium text-slate-900">
        {title ?? "Bạn không có quyền xem mục này"}
      </p>
      {children ? <p className="mt-1 text-sm text-slate-600">{children}</p> : null}
    </div>
  );
}

/**
 * ROLE_LABELS — 3 vai trò. Không còn vai trò chỉ-xem: hệ thống không có đăng
 * ký, đăng nhập được bằng tài khoản multime nghĩa là dùng được.
 */
export const ROLE_LABELS: Record<Role, string> = {
  admin: "Admin — toàn quyền, kể cả cấp quyền và xem via/proxy của mọi người",
  editor: "Editor — toàn quyền nghiệp vụ (kể cả xoá) + via/proxy của chính mình, trừ cấp quyền",
  user: "User — tạo/chạy/đăng voice, không được xoá, không vào mục Vận hành",
};

export const ROLE_ORDER: Role[] = ["admin", "editor", "user"];
