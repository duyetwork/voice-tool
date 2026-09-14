"use client";

import * as React from "react";

import { LLMApiSetsTab } from "@/components/llm-api-sets";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, RowActions, Table, Td, Th } from "@/components/ui/table";
import { UserPicker } from "@/components/user-picker";
import {
  useAIEngines,
  useCreateAIEngine,
  useDeleteAIEngine,
  useMe,
  useUpdateAIEngine,
} from "@/hooks/use-api";
import type { AIEngine } from "@/types/api";

/**
 * AI Engine — hai tab cho hai loại credential hoàn toàn khác nhau.
 *
 *   TTS Model — sổ API key 3voices: 1 key = 1 người. Chỉ còn 1 nhà cung cấp
 *               nên không có gì để "chọn", chỉ có "key của ai".
 *   LLM Model — BỘ API key: 1 bản ghi = túi key của nhiều nhà, dùng chung cho
 *               nhiều người, vì chuỗi dự phòng chỉ có ý nghĩa khi trong tay có
 *               key của nhiều nhà cùng lúc.
 *
 * Hai thứ này ở chung một màn vì đều là "khai credential cho AI", nhưng KHÔNG
 * chung một bảng: gộp lại là làm sống lại đúng những cột mà migration 000010 vừa
 * bỏ đi khỏi ai_engine.
 */
type Tab = "tts" | "llm";

export default function AIEnginesPage() {
  const me = useMe();
  const [tab, setTab] = React.useState<Tab>("tts");
  const isAdmin = me.data?.role === "admin";

  return (
    <>
      <PageHeader
        title="AI Engine"
        description="API key cho hai bước của pipeline: LLM viết lại nội dung (hình thức C), rồi TTS đọc thành audio."
      />

      <div className="mb-4 flex gap-1 border-b border-slate-200">
        <TabButton active={tab === "tts"} onClick={() => setTab("tts")}>
          TTS Model
        </TabButton>
        <TabButton active={tab === "llm"} onClick={() => setTab("llm")}>
          LLM Model
        </TabButton>
      </div>

      <Card>
        <CardBody className={tab === "tts" ? "p-0" : undefined}>
          {tab === "tts" ? <TTSTab /> : <LLMApiSetsTab isAdmin={isAdmin} />}
        </CardBody>
      </Card>
    </>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        "-mb-px border-b-2 px-4 py-2 text-sm font-medium " +
        (active
          ? "border-indigo-700 text-indigo-800"
          : "border-transparent text-slate-600 hover:text-slate-900")
      }
    >
      {children}
    </button>
  );
}

/**
 * TTSTab — sổ API key 3voices.
 *
 * Mỗi người khai key của chính mình, worker chạy TTS bằng key của người tạo
 * voice — quota và chi phí về đúng người đó. Admin thấy key của tất cả mọi
 * người (có thêm cột Người dùng) và gán được key cho người khác; các vai trò
 * khác chỉ thấy key của mình — backend ép chứ không chỉ ẩn ở UI.
 */
const PROVIDER = "3voices";

function TTSTab() {
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
      <div className="flex items-start justify-between gap-4 p-4">
        <p className="max-w-3xl text-sm text-slate-600">
          Mỗi người khai API key {PROVIDER} của chính mình — voice bạn tạo được đọc bằng key của
          bạn, nên hạn mức và hoá đơn rơi đúng vào người dùng nó. Key đã lưu không hiển thị lại, chỉ
          còn 4 ký tự cuối để đối chiếu.
        </p>
        <Can permission="can_write">
          <Button onClick={() => setAdding(true)}>Thêm API key</Button>
        </Can>
      </div>

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

      {adding ? <AddKeyDialog isAdmin={isAdmin} onClose={() => setAdding(false)} /> : null}
      {editing ? (
        <EditKeyDialog engine={editing} isAdmin={isAdmin} onClose={() => setEditing(null)} />
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
            <UserPicker selected={userIds} onChange={(next) => setUserIds(next.slice(-1))} single />
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
