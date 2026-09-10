"use client";

import * as React from "react";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Field, Input } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, RowActions, Table, Td, Th } from "@/components/ui/table";
import {
  useAIEngines,
  useCreateAIEngine,
  useDeleteAIEngine,
  useMe,
  useUpdateAIEngine,
  useUsers,
} from "@/hooks/use-api";
import type { AIEngine } from "@/types/api";

/**
 * A2 — API key TTS.
 *
 * Chỉ còn 1 nhà cung cấp (3voices) nên màn này không dùng để CHỌN engine nữa,
 * mà là sổ API key: mỗi người khai key của chính mình, worker chạy TTS bằng
 * key của người tạo voice — quota và chi phí về đúng người đó.
 *
 * Bảng vì thế chỉ còn 4 cột thật sự trả lời được câu hỏi nào đó: key nào,
 * ai khai, khai lúc nào, còn dùng không.
 *
 * Admin thấy key của tất cả mọi người (có thêm cột Người dùng) và gán được key
 * cho người khác; các vai trò khác chỉ thấy key của mình — backend ép chứ
 * không chỉ ẩn ở UI.
 */
const PROVIDER = "3voices";

export default function AIEnginesPage() {
  const me = useMe();
  const engines = useAIEngines();
  const remove = useDeleteAIEngine();
  const update = useUpdateAIEngine();
  const paging = usePaging();

  const isAdmin = me.data?.role === "admin";

  const [adding, setAdding] = React.useState(false);
  const [editing, setEditing] = React.useState<AIEngine | null>(null);

  // Endpoint /ai-engines trả toàn bộ key (mỗi người vài dòng, không có
  // limit/offset), nên phân trang ngay ở client.
  const all = engines.data?.items ?? [];
  const shown = all.slice(paging.offset, paging.offset + paging.limit);

  // API key + người tạo + 2 cột thời gian, cộng cột người dùng của admin và
  // cột hành động của người có quyền sửa.
  const columns = 4 + (isAdmin ? 1 : 0) + (me.data?.permissions.can_write ? 1 : 0);

  return (
    <>
      <PageHeader
        title="AI Engine (TTS)"
        description={`Mỗi người khai API key ${PROVIDER} của chính mình — voice bạn tạo được đọc bằng key của bạn. Key đã lưu không hiển thị lại, chỉ còn 4 ký tự cuối để đối chiếu.`}
      >
        <Can permission="can_write">
          <Button onClick={() => setAdding(true)}>Thêm API key</Button>
        </Can>
      </PageHeader>

      <Card>
        <CardBody className="p-0">
          <ErrorNote error={engines.error ?? update.error ?? remove.error} />
          <Table>
            <thead>
              <tr>
                <Th>API key</Th>
                {isAdmin ? <Th>Người dùng</Th> : null}
                <Th>Người tạo</Th>
                <Th>Thời gian tạo</Th>
                <Th>Dùng gần đây</Th>
                <Can permission="can_write">
                  <Th className="text-right">Hành động</Th>
                </Can>
              </tr>
            </thead>
            <tbody>
              {engines.isLoading ? (
                <EmptyRow colSpan={columns}>Đang tải…</EmptyRow>
              ) : shown.length ? (
                shown.map((engine) => (
                  <tr key={engine.id}>
                    <Td>
                      <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">
                        {engine.api_key_masked || "(không đọc được)"}
                      </code>
                    </Td>
                    {isAdmin ? <Td className="text-xs">{engine.user_email}</Td> : null}
                    <Td className="text-xs">{engine.created_by_email}</Td>
                    <Td>
                      <DateCell value={engine.created_at} />
                    </Td>
                    <Td>
                      {engine.last_used_at ? (
                        <DateCell value={engine.last_used_at} />
                      ) : (
                        <span className="text-xs text-slate-400">chưa dùng lần nào</span>
                      )}
                    </Td>
                    <Can permission="can_write">
                      <Td className="whitespace-nowrap text-right">
                        <RowActions>
                          <Button size="sm" variant="secondary" onClick={() => setEditing(engine)}>
                            Sửa
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => {
                              const owner = isAdmin ? ` của ${engine.user_email}` : "";
                              if (confirm(`Xoá API key ${engine.api_key_masked}${owner}?`)) {
                                remove.mutate(engine.id);
                              }
                            }}
                          >
                            Xoá
                          </Button>
                        </RowActions>
                      </Td>
                    </Can>
                  </tr>
                ))
              ) : (
                <EmptyRow colSpan={columns}>
                  Chưa có API key nào — thêm key {PROVIDER} để chạy được hình thức B/C.
                </EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={engines.data ? all.length : undefined} paging={paging} unit="key" />
        </CardBody>
      </Card>

      {adding ? <AddKeyDialog isAdmin={isAdmin} onClose={() => setAdding(false)} /> : null}
      {editing ? (
        <EditKeyDialog
          engine={editing}
          isAdmin={isAdmin}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </>
  );
}

/**
 * AddKeyDialog — thêm key.
 *
 * Admin chọn được nhiều người một lượt: mua 1 key rồi phát cho cả nhóm là
 * việc thường ngày, và mỗi người nhận một bản ghi riêng nên sau này đổi/thu
 * hồi key của từng người mà không đụng người còn lại.
 */
function AddKeyDialog({ isAdmin, onClose }: { isAdmin: boolean; onClose: () => void }) {
  const create = useCreateAIEngine();
  const [apiKey, setApiKey] = React.useState("");
  const [userIds, setUserIds] = React.useState<string[]>([]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await create.mutateAsync({
      api_key: apiKey.trim(),
      // Người không phải admin không gửi gì: backend gán cho chính họ.
      user_ids: isAdmin && userIds.length ? userIds : undefined,
    });
    onClose();
  }

  return (
    <Modal
      title="Thêm API key"
      description={
        isAdmin
          ? "Để trống ô người dùng thì key thuộc về chính bạn."
          : "Key này chỉ dùng cho voice do bạn tạo."
      }
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="API key" required>
          <Input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="sk-ov-..."
            autoComplete="off"
            autoFocus
            required
          />
        </Field>

        {isAdmin ? (
          <Field label="Người dùng">
            <UserPicker selected={userIds} onChange={setUserIds} />
          </Field>
        ) : null}

        <ErrorNote error={create.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={create.isPending}>
            {create.isPending ? "Đang lưu…" : "Thêm API key"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * EditKeyDialog — sửa key đã lưu.
 *
 * Ô API key để trống nghĩa là GIỮ key cũ: key thật không bao giờ được gửi về
 * trình duyệt nên không có gì để điền sẵn.
 */
function EditKeyDialog({
  engine,
  isAdmin,
  onClose,
}: {
  engine: AIEngine;
  isAdmin: boolean;
  onClose: () => void;
}) {
  const update = useUpdateAIEngine();
  const [apiKey, setApiKey] = React.useState("");
  // Sửa thì key chỉ thuộc về 1 người, nên ô chọn ở đây là chọn 1.
  const [userIds, setUserIds] = React.useState<string[]>([engine.user_id]);

  const owner = userIds[0] ?? engine.user_id;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await update.mutateAsync({
      id: engine.id,
      api_key: apiKey.trim() || undefined,
      user_id: isAdmin && owner !== engine.user_id ? owner : undefined,
    });
    onClose();
  }

  return (
    <Modal
      title="Sửa API key"
      description={`Của ${engine.user_email}. Bỏ trống ô API key để giữ nguyên key đang dùng (${engine.api_key_masked || "không đọc được"}).`}
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="API key mới" hint="Để trống = giữ key cũ.">
          <Input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="sk-ov-..."
            autoComplete="off"
            autoFocus
          />
        </Field>

        {isAdmin ? (
          <Field label="Người dùng" hint="Đổi người ở đây là chuyển key sang cho họ.">
            <UserPicker
              selected={userIds}
              onChange={(next) => setUserIds(next.slice(-1))}
              single
            />
          </Field>
        ) : null}

        <ErrorNote error={update.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={update.isPending}>
            {update.isPending ? "Đang lưu…" : "Lưu"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * UserPicker — dropdown tick chọn người dùng (chỉ admin thấy).
 *
 * Không dùng `<select multiple>`: nó cao bằng cả danh sách, và trên mọi trình
 * duyệt đều phải giữ Ctrl để chọn nhiều — người dùng không đoán ra được. Ở đây
 * là 1 ô bấm ra danh sách có checkbox, đọc là hiểu.
 *
 * `single` biến nó thành chọn-một cho lúc chuyển key sang người khác.
 */
function UserPicker({
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
                  <Checkbox
                    checked={selected.includes(user.id)}
                    onChange={() => toggle(user.id)}
                  />
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
