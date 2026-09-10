"use client";

import * as React from "react";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can, usePermissions } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, RowActions, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import { useCreatePrompt, useDeletePrompt, usePrompts } from "@/hooks/use-api";

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
                <Field label="Tên">
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
                  <Can permission="can_delete">
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
                      <Can permission="can_delete">
                        <Td className="text-right">
                          <RowActions>
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={() => {
                                if (confirm(`Xoá prompt "${prompt.name}"?`)) remove.mutate(prompt.id);
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
                  <EmptyRow colSpan={4}>Chưa có prompt nào.</EmptyRow>
                )}
              </tbody>
            </Table>

            <Pagination total={prompts.data?.total} paging={paging} unit="prompt" />
          </CardBody>
        </Card>
      </div>
    </>
  );
}
