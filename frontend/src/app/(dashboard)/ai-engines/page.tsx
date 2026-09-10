"use client";

import * as React from "react";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can, usePermissions } from "@/components/permission";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Checkbox, Field, Input } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, RowActions, Table, Td, Th } from "@/components/ui/table";
import {
  useAIEngines,
  useCreateAIEngine,
  useDeleteAIEngine,
  useUpdateAIEngine,
} from "@/hooks/use-api";

/**
 * A2 — Quản lý AI Engine (TTS). Hệ thống không tự xây TTS, chỉ tích hợp nhà
 * cung cấp bên thứ 3; engine active đầu tiên được dùng làm mặc định.
 */
export default function AIEnginesPage() {
  const { perms } = usePermissions();
  const engines = useAIEngines();
  const create = useCreateAIEngine();
  const update = useUpdateAIEngine();
  const remove = useDeleteAIEngine();
  const paging = usePaging();

  const [name, setName] = React.useState("");
  const [provider, setProvider] = React.useState("");
  const [languages, setLanguages] = React.useState("vi, en");
  const [isActive, setIsActive] = React.useState(true);

  // Endpoint /ai-engines trả về toàn bộ engine (bảng vài dòng, không có
  // limit/offset), nên phân trang ngay ở client.
  const all = engines.data?.items ?? [];
  const shown = all.slice(paging.offset, paging.offset + paging.limit);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await create.mutateAsync({
      name: name.trim(),
      provider: provider.trim(),
      supported_languages: languages
        .split(",")
        .map((l) => l.trim())
        .filter(Boolean),
      is_active: isActive,
    });
    setName("");
    setProvider("");
  }

  return (
    <>
      <PageHeader
        title="AI Engine (TTS)"
        description="Ngôn ngữ của Bài Post được validate với supported_languages của engine trước khi chạy TTS."
      />

      <div className={perms.can_write ? "grid gap-6 xl:grid-cols-[420px_1fr]" : ""}>
        <Can permission="can_write">
          <Card>
            <CardHeader title="Thêm engine" />
            <CardBody>
              <form onSubmit={submit} className="space-y-4">
                <Field label="Tên">
                  <Input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="3voices.win"
                    required
                  />
                </Field>
                <Field
                  label="Provider"
                  hint="Khớp với TTS_PROVIDER trong .env: 3voices | elevenlabs | mock."
                >
                  <Input
                    value={provider}
                    onChange={(e) => setProvider(e.target.value)}
                    placeholder="3voices"
                    required
                  />
                </Field>
                <Field
                  label="Ngôn ngữ hỗ trợ"
                  hint="Mã ngôn ngữ phân tách bằng dấu phẩy (vi, en, zh-CN…). Để trống = không giới hạn."
                >
                  <Input value={languages} onChange={(e) => setLanguages(e.target.value)} />
                </Field>

                <label className="flex items-center gap-2 text-sm text-slate-700">
                  <Checkbox checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                  Đang hoạt động
                </label>

                <ErrorNote error={create.error} />

                <Button type="submit" className="w-full" disabled={create.isPending}>
                  {create.isPending ? "Đang lưu…" : "Thêm engine"}
                </Button>
              </form>
            </CardBody>
          </Card>
        </Can>

        <Card>
          <CardHeader title="Engine đã tích hợp" />
          <CardBody className="p-0">
            <ErrorNote error={engines.error ?? update.error ?? remove.error} />
            <Table>
              <thead>
                <tr>
                  <Th>Tên</Th>
                  <Th>Provider</Th>
                  <Th>Ngôn ngữ</Th>
                  <Th>Trạng thái</Th>
                  <Can permission="can_write">
                    <Th className="text-right">Hành động</Th>
                  </Can>
                </tr>
              </thead>
              <tbody>
                {engines.isLoading ? (
                  <EmptyRow colSpan={5}>Đang tải…</EmptyRow>
                ) : shown.length ? (
                  shown.map((engine) => (
                    <tr key={engine.id}>
                      <Td className="font-medium text-slate-900">{engine.name}</Td>
                      <Td>
                        <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">
                          {engine.provider}
                        </code>
                      </Td>
                      <Td className="max-w-xs text-xs">
                        {engine.supported_languages?.join(", ") || "—"}
                      </Td>
                      <Td>
                        <Badge tone={engine.is_active ? "success" : "neutral"}>
                          {engine.is_active ? "Đang dùng" : "Tắt"}
                        </Badge>
                      </Td>
                      <Can permission="can_write">
                        <Td className="whitespace-nowrap text-right">
                          <RowActions>
                            <Button
                              size="sm"
                              variant="secondary"
                              disabled={update.isPending}
                              onClick={() =>
                                update.mutate({ id: engine.id, is_active: !engine.is_active })
                              }
                            >
                              {engine.is_active ? "Tắt" : "Bật"}
                            </Button>
                            <Can permission="can_delete">
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={() => {
                                  if (confirm(`Xoá engine "${engine.name}"?`))
                                    remove.mutate(engine.id);
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
                  <EmptyRow colSpan={5}>Chưa có engine nào.</EmptyRow>
                )}
              </tbody>
            </Table>

            <Pagination
              total={engines.data ? all.length : undefined}
              paging={paging}
              unit="engine"
            />
          </CardBody>
        </Card>
      </div>
    </>
  );
}
