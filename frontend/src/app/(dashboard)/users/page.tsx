"use client";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { ROLE_LABELS, ROLE_ORDER, usePermissions } from "@/components/permission";
import { Card, CardBody } from "@/components/ui/card";
import { Select, Toggle } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import { useSetUserActive, useSetUserRole, useUsers } from "@/hooks/use-api";
import type { Role } from "@/types/api";

/**
 * Quản lý tài khoản — chỉ admin vào được (API cũng chặn bằng RequireAdmin).
 *
 * Bảng này không có cột "Hành động": hai thao tác duy nhất là đổi quyền và
 * bật/tắt tài khoản, nên chúng nằm ngay trong cột Quyền và Trạng thái — sửa
 * đúng chỗ đang đọc, không phải nhìn sang cuối dòng.
 */
export default function UsersPage() {
  const { perms } = usePermissions();
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  const users = useUsers({ ...sorting.params, limit: paging.limit, offset: paging.offset });
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
                <Th>Trạng thái tài khoản</Th>
                <SortableTh sorting={sorting} column="created_at">
                  Tạo lúc
                </SortableTh>
              </tr>
            </thead>
            <tbody>
              {users.isLoading ? (
                <EmptyRow colSpan={5}>Đang tải…</EmptyRow>
              ) : users.data?.items.length ? (
                users.data.items.map((user) => (
                  <tr key={user.id}>
                    <Td className="font-medium text-slate-900">{user.email}</Td>
                    <Td>{user.full_name ?? "—"}</Td>
                    <Td>
                      <Select
                        className="h-8 w-28 text-xs"
                        value={user.role}
                        disabled={setRole.isPending}
                        onChange={(e) =>
                          setRole.mutate({ id: user.id, role: e.target.value as Role })
                        }
                        aria-label={`Quyền của ${user.email}`}
                      >
                        {ROLE_ORDER.map((role) => (
                          <option key={role} value={role}>
                            {role}
                          </option>
                        ))}
                      </Select>
                    </Td>
                    <Td>
                      <Toggle
                        checked={user.is_active ?? false}
                        disabled={setActive.isPending}
                        onChange={(next) => setActive.mutate({ id: user.id, is_active: next })}
                        label={user.is_active ? "Bật" : "Tắt"}
                      />
                    </Td>
                    <Td>
                      <DateCell value={user.created_at} />
                    </Td>
                  </tr>
                ))
              ) : (
                <EmptyRow colSpan={5}>Chưa có tài khoản nào.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={users.data?.total} paging={paging} unit="tài khoản" />
        </CardBody>
      </Card>

      <Card className="mt-4">
        <CardBody>
          <h2 className="mb-2 text-sm font-semibold text-slate-900">Ý nghĩa từng quyền</h2>
          <ul className="space-y-1 text-sm text-slate-600">
            {ROLE_ORDER.map((role) => (
              <li key={role}>{ROLE_LABELS[role]}</li>
            ))}
          </ul>
        </CardBody>
      </Card>
    </>
  );
}
