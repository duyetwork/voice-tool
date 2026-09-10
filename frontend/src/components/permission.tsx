"use client";

import * as React from "react";

import { useMe } from "@/hooks/use-api";
import type { Permissions, Role } from "@/types/api";

const FALLBACK: Permissions = {
  can_write: false,
  can_delete: false,
  can_manage_users: false,
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
 * ROLE_LABELS — 3 vai trò. Không còn vai trò chỉ-xem: hệ thống không có đăng
 * ký, đăng nhập được bằng tài khoản multime nghĩa là dùng được.
 */
export const ROLE_LABELS: Record<Role, string> = {
  admin: "Admin — toàn quyền, kể cả cấp quyền cho người khác",
  editor: "Editor — toàn quyền nghiệp vụ (kể cả xoá), trừ cấp quyền",
  user: "User — tạo/chạy/đăng voice, không được xoá",
};

export const ROLE_ORDER: Role[] = ["admin", "editor", "user"];
