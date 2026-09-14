"use client";

import * as React from "react";

import { ErrorNote } from "@/components/page-header";
import { Can } from "@/components/permission";
import { UserPicker } from "@/components/user-picker";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea, Toggle } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { DateCell, EmptyRow, RowActions, Table, Td, Th } from "@/components/ui/table";
import {
  useAddLLMAPIKey,
  useCreateLLMAPISet,
  useDeleteLLMAPIKey,
  useDeleteLLMAPISet,
  useLLMAPISet,
  useLLMAPISets,
  useUpdateLLMAPIKey,
  useUpdateLLMAPISet,
  type LLMAPIKeyInput,
} from "@/hooks/use-api";
import type { LLMAPIKey, LLMAPISet, LLMProvider } from "@/types/api";

/**
 * Tab "LLM Model" của màn AI Engine.
 *
 * Khác hẳn tab TTS dù nhìn qua thì giống. Tab TTS là sổ key: 1 key của 1 người,
 * 1 nhà cung cấp. Ở đây 1 bản ghi là một BỘ — túi key của nhiều nhà, dùng chung
 * cho nhiều người — vì chuỗi dự phòng chỉ có ý nghĩa khi trong tay có key của
 * nhiều nhà cùng lúc: hết hạn mức Gemini thì còn OpenAI, hết OpenAI thì còn
 * Anthropic.
 *
 * Key thật KHÔNG BAO GIỜ được gửi về trình duyệt — bảng chỉ có 4 ký tự cuối.
 */

const PROVIDERS: { value: LLMProvider; label: string }[] = [
  { value: "gemini", label: "Gemini" },
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
];

const PROVIDER_LABELS: Record<string, string> = Object.fromEntries(
  PROVIDERS.map((p) => [p.value, p.label]),
);

export function LLMApiSetsTab({ isAdmin }: { isAdmin: boolean }) {
  const sets = useLLMAPISets();
  const remove = useDeleteLLMAPISet();
  const update = useUpdateLLMAPISet();

  const [adding, setAdding] = React.useState(false);
  const [editing, setEditing] = React.useState<LLMAPISet | null>(null);
  const [managing, setManaging] = React.useState<LLMAPISet | null>(null);

  const items = sets.data?.items ?? [];

  return (
    <>
      <div className="mb-4 flex items-start justify-between gap-4">
        <p className="max-w-3xl text-sm text-slate-600">
          Một <strong>bộ API</strong> là túi key của nhiều nhà LLM, dùng chung cho một hoặc nhiều
          người. Khi tạo voice hình thức C, hệ thống thử lần lượt theo chuỗi dự phòng (Cài đặt) và
          tự chuyển sang nhà kế tiếp khi hết hạn mức — nên bộ càng nhiều nhà càng ít đứt. Key đã lưu
          không hiển thị lại, chỉ còn 4 ký tự cuối để đối chiếu.
        </p>
        <Can permission="can_write">
          <Button onClick={() => setAdding(true)}>Thêm bộ API</Button>
        </Can>
      </div>

      <ErrorNote error={sets.error ?? update.error ?? remove.error} />

      <Table>
        <thead>
          <tr>
            <Th>Tên bộ</Th>
            <Th>Key theo nhà</Th>
            <Th>Dùng chung</Th>
            <Th>Hiện với mọi người</Th>
            <Th>Người tạo</Th>
            <Th>Dùng gần đây</Th>
            <Th className="text-right">Hành động</Th>
          </tr>
        </thead>
        <tbody>
          {sets.isLoading ? (
            <EmptyRow colSpan={7}>Đang tải…</EmptyRow>
          ) : items.length ? (
            items.map((set) => (
              <tr key={set.id}>
                <Td>
                  <div className="font-medium text-slate-900">{set.name}</div>
                  {set.note ? <div className="text-xs text-slate-500">{set.note}</div> : null}
                </Td>
                <Td>
                  <KeyCounts counts={set.key_counts} />
                </Td>
                <Td className="text-xs">
                  {set.users.length ? (
                    set.users.map((u) => u.email).join(", ")
                  ) : (
                    <span className="text-slate-400">chỉ người tạo</span>
                  )}
                </Td>
                <Td>
                  {/* Toggle của admin: bật lên là mở hạn mức của nhóm này cho
                      cả hệ thống dùng, nên người khác chỉ được nhìn. */}
                  <Toggle
                    checked={set.visible_to_users}
                    disabled={!isAdmin || update.isPending}
                    label={set.visible_to_users ? "Đang hiện" : "Đang ẩn"}
                    onChange={(next) => update.mutate({ id: set.id, visible_to_users: next })}
                  />
                </Td>
                <Td className="text-xs">{set.created_by_email}</Td>
                <Td>
                  {set.last_used_at ? (
                    <DateCell value={set.last_used_at} />
                  ) : (
                    <span className="text-xs text-slate-400">chưa dùng lần nào</span>
                  )}
                </Td>
                <Td className="whitespace-nowrap text-right">
                  <RowActions>
                    <Button size="sm" variant="secondary" onClick={() => setManaging(set)}>
                      Quản lý key
                    </Button>
                    {set.can_manage ? (
                      <>
                        <Button size="sm" variant="secondary" onClick={() => setEditing(set)}>
                          Sửa
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={() => {
                            if (confirm(`Xoá bộ API "${set.name}" cùng toàn bộ key bên trong?`)) {
                              remove.mutate(set.id);
                            }
                          }}
                        >
                          Xoá
                        </Button>
                      </>
                    ) : null}
                  </RowActions>
                </Td>
              </tr>
            ))
          ) : (
            <EmptyRow colSpan={7}>
              Chưa có bộ API nào — thêm một bộ để chạy được hình thức C.
            </EmptyRow>
          )}
        </tbody>
      </Table>

      {adding ? <SetDialog isAdmin={isAdmin} onClose={() => setAdding(false)} /> : null}
      {editing ? (
        <SetDialog set={editing} isAdmin={isAdmin} onClose={() => setEditing(null)} />
      ) : null}
      {managing ? <KeysDialog set={managing} onClose={() => setManaging(null)} /> : null}
    </>
  );
}

/** KeyCounts hiện "gemini 2 · openai 1" mà không phải tải toàn bộ key về. */
function KeyCounts({ counts }: { counts: Record<string, number> }) {
  const entries = Object.entries(counts ?? {});
  if (!entries.length) {
    return <span className="text-xs text-amber-700">chưa có key nào</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {entries.map(([provider, n]) => (
        <Badge key={provider} tone="info">
          {PROVIDER_LABELS[provider] ?? provider} {n}
        </Badge>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tạo / sửa bộ
// ---------------------------------------------------------------------------

/**
 * SetDialog dùng chung cho tạo và sửa.
 *
 * Lúc TẠO có thêm phần khai key ngay tại chỗ: tạo bộ rỗng rồi mở tiếp một hộp
 * thoại nữa chỉ để dán key là hai bước cho một việc.
 */
function SetDialog({
  set,
  isAdmin,
  onClose,
}: {
  set?: LLMAPISet;
  isAdmin: boolean;
  onClose: () => void;
}) {
  const create = useCreateLLMAPISet();
  const update = useUpdateLLMAPISet();
  const editing = !!set;

  const [name, setName] = React.useState(set?.name ?? "");
  const [note, setNote] = React.useState(set?.note ?? "");
  const [visible, setVisible] = React.useState(set?.visible_to_users ?? false);
  const [userIds, setUserIds] = React.useState<string[]>(set?.users.map((u) => u.user_id) ?? []);
  const [keys, setKeys] = React.useState<LLMAPIKeyInput[]>([]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (editing) {
      await update.mutateAsync({
        id: set!.id,
        name: name.trim(),
        note: note.trim(),
        // Người không phải admin không gửi cờ này — backend cũng chặn.
        visible_to_users: isAdmin ? visible : undefined,
        user_ids: userIds,
      });
    } else {
      await create.mutateAsync({
        name: name.trim(),
        note: note.trim() || undefined,
        visible_to_users: isAdmin ? visible : undefined,
        user_ids: userIds.length ? userIds : undefined,
        keys: keys.filter((k) => k.api_key.trim()),
      });
    }
    onClose();
  }

  const pending = create.isPending || update.isPending;

  return (
    <Modal
      width="2xl"
      title={editing ? "Sửa bộ API" : "Thêm bộ API"}
      description="Bộ gồm key của một hoặc nhiều nhà LLM. Càng nhiều nhà thì chuỗi dự phòng càng ít đứt khi một nhà hết hạn mức."
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="Tên bộ" required>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="vd: Bộ chung phòng nội dung"
            autoFocus
            required
          />
        </Field>

        <Field label="Ghi chú" hint="Tuỳ chọn — dùng để nhớ bộ này của ai, mua ở đâu.">
          <Textarea value={note} onChange={(e) => setNote(e.target.value)} className="min-h-16" />
        </Field>

        <Field
          label="Dùng chung với"
          hint="Người được chọn dùng được bộ này khi tạo voice, nhưng không sửa được key."
        >
          <UserPicker selected={userIds} onChange={setUserIds} />
        </Field>

        {isAdmin ? (
          <Field
            label="Hiện với mọi người"
            hint="Bật lên là mọi tài khoản đều chọn được bộ này — tức là dùng chung hạn mức và chi phí của nó."
          >
            <Toggle
              checked={visible}
              onChange={setVisible}
              label={visible ? "Đang hiện với mọi người" : "Chỉ người được chia sẻ"}
            />
          </Field>
        ) : null}

        {editing ? null : <NewKeysEditor keys={keys} onChange={setKeys} />}

        <ErrorNote error={create.error ?? update.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu" : "Thêm bộ API"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/** NewKeysEditor — khai key ngay trong hộp thoại tạo bộ. */
function NewKeysEditor({
  keys,
  onChange,
}: {
  keys: LLMAPIKeyInput[];
  onChange: (next: LLMAPIKeyInput[]) => void;
}) {
  function set(i: number, patch: Partial<LLMAPIKeyInput>) {
    onChange(keys.map((k, idx) => (idx === i ? { ...k, ...patch } : k)));
  }

  return (
    <div className="rounded-md border border-slate-200 p-3">
      <div className="mb-2 flex items-center justify-between">
        <p className="text-sm font-medium text-slate-700">API key</p>
        <Button
          type="button"
          size="sm"
          variant="secondary"
          onClick={() => onChange([...keys, { provider: "gemini", api_key: "", priority: 0 }])}
        >
          Thêm key
        </Button>
      </div>

      {keys.length ? (
        <div className="space-y-2">
          {keys.map((k, i) => (
            <div key={i} className="flex gap-2">
              <Select
                value={k.provider}
                onChange={(e) => set(i, { provider: e.target.value as LLMProvider })}
                className="w-36"
              >
                {PROVIDERS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </Select>
              <Input
                type="password"
                value={k.api_key}
                onChange={(e) => set(i, { api_key: e.target.value })}
                placeholder="API key"
                autoComplete="off"
              />
              <Input
                value={k.label ?? ""}
                onChange={(e) => set(i, { label: e.target.value })}
                placeholder="nhãn (tuỳ chọn)"
                className="w-40"
              />
              <Button
                type="button"
                size="sm"
                variant="danger"
                onClick={() => onChange(keys.filter((_, idx) => idx !== i))}
              >
                Bỏ
              </Button>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-xs text-slate-500">
          Có thể tạo bộ rỗng rồi thêm key sau, nhưng bộ chưa có key thì chưa chạy được.
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Key bên trong bộ
// ---------------------------------------------------------------------------

/**
 * KeysDialog — CRUD key và xem SỨC KHOẺ key.
 *
 * Sức khoẻ là thứ duy nhất ở màn này mà người dùng không tự điền: router ghi
 * vào. Nó tách làm hai loại vì cách xử lý khác hẳn nhau — "đang nghỉ" thì chỉ
 * cần chờ, còn "đã tắt" thì chờ bao lâu cũng không tự khỏi, phải dán key mới.
 */
function KeysDialog({ set, onClose }: { set: LLMAPISet; onClose: () => void }) {
  const detail = useLLMAPISet(set.id);
  const add = useAddLLMAPIKey();
  const remove = useDeleteLLMAPIKey();
  const [editing, setEditing] = React.useState<LLMAPIKey | null>(null);
  const [draft, setDraft] = React.useState<LLMAPIKeyInput>({
    provider: "gemini",
    api_key: "",
    label: "",
    priority: 0,
  });

  const keys = detail.data?.keys ?? [];
  const canManage = detail.data?.can_manage ?? set.can_manage;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await add.mutateAsync({ setId: set.id, ...draft, api_key: draft.api_key.trim() });
    setDraft({ provider: draft.provider, api_key: "", label: "", priority: 0 });
  }

  return (
    <Modal
      width="3xl"
      title={`Key trong bộ "${set.name}"`}
      description="Trong cùng một nhà, số ưu tiên nhỏ hơn được thử trước. Thứ tự giữa các nhà do chuỗi dự phòng ở mục Cài đặt quyết định."
      onClose={onClose}
    >
      <div className="space-y-4">
        <ErrorNote error={detail.error ?? add.error ?? remove.error} />

        <Table>
          <thead>
            <tr>
              <Th>Nhà</Th>
              <Th>Key</Th>
              <Th>Nhãn</Th>
              <Th>Ưu tiên</Th>
              <Th>Sức khoẻ</Th>
              <Th>Dùng gần đây</Th>
              {canManage ? <Th className="text-right">Hành động</Th> : null}
            </tr>
          </thead>
          <tbody>
            {detail.isLoading ? (
              <EmptyRow colSpan={7}>Đang tải…</EmptyRow>
            ) : keys.length ? (
              keys.map((key) => (
                <tr key={key.id}>
                  <Td>{PROVIDER_LABELS[key.provider] ?? key.provider}</Td>
                  <Td>
                    <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">
                      {key.api_key_masked || "(không đọc được)"}
                    </code>
                  </Td>
                  <Td className="text-xs">{key.label ?? "—"}</Td>
                  <Td className="text-xs">{key.priority}</Td>
                  <Td>
                    <KeyHealth apiKey={key} />
                  </Td>
                  <Td>
                    {key.last_used_at ? (
                      <DateCell value={key.last_used_at} />
                    ) : (
                      <span className="text-xs text-slate-400">chưa dùng</span>
                    )}
                  </Td>
                  {canManage ? (
                    <Td className="whitespace-nowrap text-right">
                      <RowActions>
                        <Button size="sm" variant="secondary" onClick={() => setEditing(key)}>
                          Sửa
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={() => {
                            if (confirm(`Xoá key ${key.api_key_masked} (${key.provider})?`)) {
                              remove.mutate(key.id);
                            }
                          }}
                        >
                          Xoá
                        </Button>
                      </RowActions>
                    </Td>
                  ) : null}
                </tr>
              ))
            ) : (
              <EmptyRow colSpan={7}>Bộ này chưa có key nào.</EmptyRow>
            )}
          </tbody>
        </Table>

        {canManage ? (
          <form onSubmit={submit} className="flex items-end gap-2 border-t border-slate-200 pt-4">
            <div className="w-36">
              <Field label="Nhà">
                <Select
                  value={draft.provider}
                  onChange={(e) => setDraft({ ...draft, provider: e.target.value as LLMProvider })}
                >
                  {PROVIDERS.map((p) => (
                    <option key={p.value} value={p.value}>
                      {p.label}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <div className="flex-1">
              <Field label="API key" required>
                <Input
                  type="password"
                  value={draft.api_key}
                  onChange={(e) => setDraft({ ...draft, api_key: e.target.value })}
                  autoComplete="off"
                  required
                />
              </Field>
            </div>
            <div className="w-40">
              <Field label="Nhãn">
                <Input
                  value={draft.label ?? ""}
                  onChange={(e) => setDraft({ ...draft, label: e.target.value })}
                  placeholder="vd: công ty"
                />
              </Field>
            </div>
            <div className="w-24">
              <Field label="Ưu tiên">
                <Input
                  type="number"
                  value={draft.priority ?? 0}
                  onChange={(e) => setDraft({ ...draft, priority: Number(e.target.value) })}
                />
              </Field>
            </div>
            <Button type="submit" disabled={add.isPending}>
              {add.isPending ? "Đang thêm…" : "Thêm"}
            </Button>
          </form>
        ) : (
          <p className="border-t border-slate-200 pt-4 text-xs text-slate-500">
            Bộ này được chia sẻ cho bạn: dùng được khi tạo voice, nhưng chỉ người tạo bộ hoặc admin
            mới sửa được key.
          </p>
        )}

        <div className="flex justify-end">
          <Button type="button" variant="secondary" onClick={onClose}>
            Đóng
          </Button>
        </div>
      </div>

      {editing ? <EditKeyDialog apiKey={editing} onClose={() => setEditing(null)} /> : null}
    </Modal>
  );
}

/** KeyHealth nói rõ NÊN LÀM GÌ, không chỉ nói trạng thái. */
function KeyHealth({ apiKey }: { apiKey: LLMAPIKey }) {
  if (apiKey.health === "disabled") {
    return (
      <div>
        <Badge tone="danger">Đã tắt</Badge>
        <p className="mt-0.5 max-w-56 text-xs text-slate-500">
          {apiKey.last_error ?? "key sai hoặc bị thu hồi"} — dán key mới để bật lại.
        </p>
      </div>
    );
  }
  if (apiKey.health === "cooldown") {
    return (
      <div>
        <Badge tone="warning">Đang nghỉ</Badge>
        <p className="mt-0.5 text-xs text-slate-500">
          hết hạn mức, tự chạy lại lúc{" "}
          {apiKey.cooldown_until ? new Date(apiKey.cooldown_until).toLocaleString("vi-VN") : "—"}
        </p>
      </div>
    );
  }
  return <Badge tone="success">Bình thường</Badge>;
}

/**
 * EditKeyDialog — ô API key để trống nghĩa là GIỮ key cũ.
 *
 * Dán key mới thì backend cũng xoá luôn cờ "đã tắt"/"đang nghỉ": người ta vào
 * đây chính vì key hỏng, dán key mới mà vẫn bị bỏ qua thì không ai hiểu vì sao.
 */
function EditKeyDialog({ apiKey, onClose }: { apiKey: LLMAPIKey; onClose: () => void }) {
  const update = useUpdateLLMAPIKey();
  const [value, setValue] = React.useState("");
  const [label, setLabel] = React.useState(apiKey.label ?? "");
  const [priority, setPriority] = React.useState(apiKey.priority);
  const [provider, setProvider] = React.useState<LLMProvider>(apiKey.provider);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await update.mutateAsync({
      id: apiKey.id,
      provider,
      api_key: value.trim() || undefined,
      label,
      priority,
    });
    onClose();
  }

  return (
    <Modal
      title="Sửa API key"
      description={`Bỏ trống ô API key để giữ nguyên key đang dùng (${apiKey.api_key_masked || "không đọc được"}).`}
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="Nhà cung cấp">
          <Select value={provider} onChange={(e) => setProvider(e.target.value as LLMProvider)}>
            {PROVIDERS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </Select>
        </Field>

        <Field
          label="API key mới"
          hint="Để trống = giữ key cũ. Dán key mới sẽ bật lại key đang bị tắt hoặc đang nghỉ."
        >
          <Input
            type="password"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            autoComplete="off"
            autoFocus
          />
        </Field>

        <Field label="Nhãn">
          <Input value={label} onChange={(e) => setLabel(e.target.value)} />
        </Field>

        <Field label="Ưu tiên" hint="Nhỏ hơn = thử trước, trong cùng một nhà.">
          <Input
            type="number"
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
          />
        </Field>

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
