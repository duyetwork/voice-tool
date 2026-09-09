"use client";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { ROLE_LABELS, usePermissions } from "@/components/permission";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Select } from "@/components/ui/field";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import { useSetUserActive, useSetUserRole, useUsers } from "@/hooks/use-api";
import { formatDateTime } from "@/lib/utils";
import type { Role } from "@/types/api";

const ROLE_TONES: Record<Role, "success" | "info" | "neutral"> = {
  admin: "success",
  user: "info",
  viewer: "neutral",
};

/** Quản lý tài khoản — chỉ admin vào được (API cũng chặn bằng RequireAdmin). */
export default function UsersPage() {
  const { perms } = usePermissions();
  const users = useUsers({ limit: 100 });
  const setRole = useSetUserRole();
  const setActive = useSetUserActive();

  if (!perms.can_manage_users) {
    return (
      <>
        <PageHeader title="Quản lý tài khoản" />
        <Card>
          <CardBody>
            <p className="text-sm text-slate-600">Chỉ admin xem được trang này.</p>
          </CardBody>
        </Card>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Quản lý tài khoản"
        description="Tài khoản được tạo tự động ở lần đăng nhập đầu tiên bằng SSO của multime — admin cấp quyền tại đây."
      />

      <Card>
        <CardBody className="p-0">
          <ErrorNote error={users.error ?? setRole.error ?? setActive.error} />
          <Table>
            <thead>
              <tr>
                <Th>Email</Th>
                <Th>Họ tên</Th>
                <Th>Quyền</Th>
                <Th>Trạng thái</Th>
                <Th>Tạo lúc</Th>
                <Th className="text-right">Hành động</Th>
              </tr>
            </thead>
            <tbody>
              {users.isLoading ? (
                <EmptyRow colSpan={6}>Đang tải…</EmptyRow>
              ) : users.data?.items.length ? (
                users.data.items.map((user) => (
                  <tr key={user.id}>
                    <Td className="font-medium text-slate-900">{user.email}</Td>
                    <Td>{user.full_name ?? "—"}</Td>
                    <Td>
                      <Badge tone={ROLE_TONES[user.role]}>{user.role}</Badge>
                    </Td>
                    <Td>
                      <Badge tone={user.is_active ? "success" : "neutral"}>
                        {user.is_active ? "Đang hoạt động" : "Đã tắt"}
                      </Badge>
                    </Td>
                    <Td className="whitespace-nowrap">{formatDateTime(user.created_at)}</Td>
                    <Td className="whitespace-nowrap text-right">
                      <div className="inline-flex items-center gap-2">
                        <div className="w-28">
                          <Select
                            value={user.role}
                            disabled={setRole.isPending}
                            onChange={(e) =>
                              setRole.mutate({ id: user.id, role: e.target.value as Role })
                            }
                          >
                            {(Object.keys(ROLE_LABELS) as Role[]).map((role) => (
                              <option key={role} value={role}>
                                {role}
                              </option>
                            ))}
                          </Select>
                        </div>
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={setActive.isPending}
                          onClick={() =>
                            setActive.mutate({ id: user.id, is_active: !user.is_active })
                          }
                        >
                          {user.is_active ? "Tắt" : "Bật"}
                        </Button>
                      </div>
                    </Td>
                  </tr>
                ))
              ) : (
                <EmptyRow colSpan={6}>Chưa có tài khoản nào.</EmptyRow>
              )}
            </tbody>
          </Table>
        </CardBody>
      </Card>

      <Card className="mt-4">
        <CardBody>
          <h2 className="mb-2 text-sm font-semibold text-slate-900">Ý nghĩa từng quyền</h2>
          <ul className="space-y-1 text-sm text-slate-600">
            {(Object.entries(ROLE_LABELS) as [Role, string][]).map(([role, label]) => (
              <li key={role}>{label}</li>
            ))}
          </ul>
        </CardBody>
      </Card>
    </>
  );
}
