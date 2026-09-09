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
  role: Role;
  perms: Permissions;
  loading: boolean;
} {
  const me = useMe();
  return {
    role: me.data?.role ?? "viewer",
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

/** ViewerNotice giải thích cho viewer vì sao không thấy nút thao tác. */
export function ViewerNotice() {
  const { perms, loading } = usePermissions();
  if (loading || perms.can_write) return null;
  return (
    <p className="mb-4 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900">
      Bạn đang ở quyền <strong>viewer</strong> — chỉ xem được dữ liệu. Liên hệ admin để được nâng
      quyền <strong>user</strong> nếu cần tạo/chạy/đăng voice.
    </p>
  );
}

export const ROLE_LABELS: Record<Role, string> = {
  admin: "Admin — toàn quyền, kể cả xoá và cấp quyền",
  user: "User — xem tất cả + tạo/chạy/đăng voice, không được xoá",
  viewer: "Viewer — chỉ xem",
};
