"use client";

import * as React from "react";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can, usePermissions } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, RowActions, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import { Modal } from "@/components/ui/modal";
import { useCreatePrompt, useDeletePrompt, usePrompts, useUpdatePrompt } from "@/hooks/use-api";
import type { Prompt } from "@/types/api";

/** A1 — Quản lý Prompt mẫu, dùng cho Mode C. */
export default function PromptsPage() {
  const { perms } = usePermissions();
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  const prompts = usePrompts({
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });
  const create = useCreatePrompt();
  const remove = useDeletePrompt();

  const [name, setName] = React.useState("");
  const [content, setContent] = React.useState("");
  const [editing, setEditing] = React.useState<Prompt | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await create.mutateAsync({ name: name.trim(), content: content.trim() });
    setName("");
    setContent("");
  }

  return (
    <>
      <PageHeader
        title="Prompt mẫu"
        description="Chỉ dẫn biên tập cho Mode C — LLM viết lại text nguồn theo prompt trước khi đưa qua TTS."
      />

      <div className={perms.can_write ? "grid gap-6 xl:grid-cols-[420px_1fr]" : ""}>
        <Can permission="can_write">
          <Card>
            <CardHeader title="Thêm prompt" />
            <CardBody>
              <form onSubmit={submit} className="space-y-4">
                <Field label="Tên" required>
                  <Input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Tin nhanh 30 giây"
                    required
                  />
                </Field>
                <Field
                  label="Nội dung chỉ dẫn"
                  hint="Mô tả rõ độ dài, giọng điệu, những gì cần bỏ. Không cần nhắc lại yêu cầu 'chỉ trả về kịch bản' — hệ thống đã có sẵn."
                >
                  <Textarea
                    className="min-h-40"
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                    required
                  />
                </Field>

                <ErrorNote error={create.error} />

                <Button type="submit" className="w-full" disabled={create.isPending}>
                  {create.isPending ? "Đang lưu…" : "Thêm prompt"}
                </Button>
              </form>
            </CardBody>
          </Card>
        </Can>

        <Card>
          <CardHeader title="Thư viện prompt" />
          <CardBody className="p-0">
            <ErrorNote error={prompts.error ?? remove.error} />
            <Table>
              <thead>
                <tr>
                  <Th>Tên</Th>
                  <Th>Nội dung</Th>
                  <SortableTh sorting={sorting} column="created_at">
                    Tạo lúc
                  </SortableTh>
                  <Can permission="can_write">
                    <Th className="text-right">Hành động</Th>
                  </Can>
                </tr>
              </thead>
              <tbody>
                {prompts.isLoading ? (
                  <EmptyRow colSpan={4}>Đang tải…</EmptyRow>
                ) : prompts.data?.items.length ? (
                  prompts.data.items.map((prompt) => (
                    <tr key={prompt.id}>
                      <Td className="font-medium text-slate-900">{prompt.name}</Td>
                      <Td className="max-w-md">
                        <p className="line-clamp-3 text-slate-600">{prompt.content}</p>
                      </Td>
                      <Td>
                        <DateCell value={prompt.created_at} />
                      </Td>
                      <Can permission="can_write">
                        <Td className="text-right">
                          <RowActions>
                            <Button
                              size="sm"
                              variant="secondary"
                              onClick={() => setEditing(prompt)}
                            >
                              Sửa
                            </Button>
                            <Can permission="can_delete">
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={() => {
                                  if (confirm(`Xoá prompt "${prompt.name}"?`)) remove.mutate(prompt.id);
                                }}
                              >
                                Xoá
                              </Button>
                            </Can>
                          </RowActions>
                        </Td>
                      </Can>
                    </tr>
                  ))
                ) : (
                  <EmptyRow colSpan={4}>Chưa có prompt nào.</EmptyRow>
                )}
              </tbody>
            </Table>

            <Pagination total={prompts.data?.total} paging={paging} unit="prompt" />
          </CardBody>
        </Card>
      </div>

      {editing ? <EditPromptDialog prompt={editing} onClose={() => setEditing(null)} /> : null}
    </>
  );
}

/**
 * EditPromptDialog — sửa prompt tại chỗ.
 *
 * Sửa chứ không "xoá rồi thêm lại": prompt_id đang được các kênh và các voice
 * trỏ tới, nên xoá đi là cắt đứt hết những liên kết đó rồi phải đi gán lại từng
 * cái. Và prompt là thứ phải chỉnh đi chỉnh lại nhiều lần mới ra giọng văn
 * đúng ý — đó là công việc bình thường của nó, không phải ngoại lệ.
 *
 * Sửa nội dung KHÔNG đụng tới voice đã tạo: chúng đã đọc xong bằng bản cũ. Bản
 * mới áp dụng từ lần chạy kế tiếp.
 */
function EditPromptDialog({ prompt, onClose }: { prompt: Prompt; onClose: () => void }) {
  const update = useUpdatePrompt();
  const [name, setName] = React.useState(prompt.name);
  const [content, setContent] = React.useState(prompt.content);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await update.mutateAsync({ id: prompt.id, name: name.trim(), content: content.trim() });
    onClose();
  }

  return (
    <Modal title="Sửa prompt" width="2xl" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Tên" required>
          <Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <Field
          label="Nội dung chỉ dẫn"
          hint="Voice đã tạo giữ nguyên lời đọc cũ — bản sửa này áp dụng từ lần chạy kế tiếp."
        >
          <Textarea
            className="min-h-56"
            value={content}
            onChange={(e) => setContent(e.target.value)}
            required
          />
        </Field>

        <ErrorNote error={update.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={update.isPending}>
            {update.isPending ? "Đang lưu…" : "Lưu thay đổi"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
