"use client";

import * as React from "react";

import { usePermissions } from "@/components/permission";
import { Select } from "@/components/ui/field";
import { useMe, useUsers } from "@/hooks/use-api";

/**
 * CreatorFilter — lọc bảng theo người tạo.
 *
 * Danh sách tài khoản (`/users`) là route CHỈ ADMIN, nên không thể dựng một
 * dropdown người dùng chung cho mọi vai trò. Nhưng nhu cầu "chỉ xem của tôi"
 * thì ai cũng có — và nó là nhu cầu phổ biến hơn hẳn.
 *
 * Vì thế: mọi người đều có lựa chọn "Của tôi"; admin có thêm danh sách đầy đủ.
 * Không ẩn cả bộ lọc với người thường, vì như thế là lấy đi thứ họ cần nhất chỉ
 * vì không cho họ thứ họ không được xem.
 */
export function CreatorFilter({
  value,
  onChange,
  className = "w-44",
}: {
  value: string;
  onChange: (userID: string) => void;
  className?: string;
}) {
  const me = useMe();
  const { perms } = usePermissions();
  // `enabled`: người thường gọi /users sẽ ăn 403. Không chặn ở đây thì mỗi lần
  // mở bảng là một lỗi vô nghĩa trong log của cả hai phía.
  const canListUsers = perms.can_manage_users;
  const users = useUsers({ limit: 200 }, canListUsers);

  const myID = me.data?.id ?? "";
  // Bỏ chính mình khỏi danh sách: đã có mục "Của tôi" ở trên, và hai dòng cùng
  // trỏ tới một người thì người đọc phải dừng lại nghĩ xem chúng khác gì nhau.
  const others = canListUsers ? (users.data?.items ?? []).filter((u) => u.id !== myID) : [];

  return (
    <div className={className}>
      <label className="mb-1 block text-xs font-medium text-slate-500">Người tạo</label>
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">Tất cả</option>
        {myID ? <option value={myID}>Của tôi</option> : null}
        {others.map((u) => (
          <option key={u.id} value={u.id}>
            {u.email}
          </option>
        ))}
      </Select>
    </div>
  );
}
