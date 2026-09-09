"use client";

import * as React from "react";

import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can, ViewerNotice, usePermissions } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Checkbox, Field, Input, Select } from "@/components/ui/field";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import {
  useBreakingLists,
  useCollectModes,
  useCreateBreakingList,
  useDeleteBreakingList,
  usePlatforms,
  usePrompts,
  useRunBreakingList,
  useUpdateBreakingList,
} from "@/hooks/use-api";
import {
  COLLECT_MODE_LABELS,
  LANGUAGE_OPTIONS,
  formatDateTime,
  formatInterval,
  platformLabel,
} from "@/lib/utils";
import type { CollectMode } from "@/types/api";

/**
 * F2 — Danh sách Breaking. Không có tần suất quét: worker quét liên tục và chỉ
 * lấy bài khớp Regex Pattern (business rule #3, #4). Mỗi kênh có thể khai nhiều
 * pattern, kết hợp OR.
 */
export default function BreakingListsPage() {
  const [search, setSearch] = React.useState("");
  const [platform, setPlatform] = React.useState("");
  const { perms } = usePermissions();
  const lists = useBreakingLists({
    search: search || undefined,
    platform: platform || undefined,
  });
  const platforms = usePlatforms();
  const run = useRunBreakingList();
  const update = useUpdateBreakingList();
  const remove = useDeleteBreakingList();
  const selection = useSelection(lists.data?.items);

  return (
    <>
      <PageHeader
        title="F2 — Danh sách Breaking"
        description="Quét liên tục, chỉ tạo Bài Post khi caption/tiêu đề khớp một trong các Regex Pattern."
      />
      <ViewerNotice />

      <div className={perms.can_write ? "grid gap-6 xl:grid-cols-[440px_1fr]" : ""}>
        <Can permission="can_write">
          <CreateBreakingForm />
        </Can>

        <Card>
          <CardHeader
            title="Kênh đang theo dõi"
            description="Search dưới đây dùng regex, chỉ lọc hiển thị — không ảnh hưởng việc thu thập."
            action={
              <div className="flex gap-2">
                <Select
                  className="w-40"
                  value={platform}
                  onChange={(e) => setPlatform(e.target.value)}
                >
                  <option value="">Mọi nền tảng</option>
                  {(platforms.data?.platforms ?? []).map((p) => (
                    <option key={p} value={p}>
                      {platformLabel(p)}
                    </option>
                  ))}
                </Select>
                <Input
                  className="w-56"
                  placeholder="regex lọc theo URL…"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </div>
            }
          />
          <CardBody className="p-0">
            <ErrorNote error={lists.error ?? run.error ?? remove.error ?? update.error} />

            <Can permission="can_write">
              <BulkBar
                count={selection.count}
                ids={selection.selected}
                onDone={selection.clear}
                actions={[
                  {
                    label: "Quét thử",
                    variant: "primary",
                    onRun: async ([id]) => {
                      await run.mutateAsync(id);
                    },
                  },
                  {
                    label: "Tạm dừng",
                    onRun: async ([id]) => {
                      await update.mutateAsync({ id, status: "paused" });
                    },
                  },
                  {
                    label: "Kích hoạt",
                    onRun: async ([id]) => {
                      await update.mutateAsync({ id, status: "active" });
                    },
                  },
                  {
                    label: "Xoá",
                    permission: "can_delete",
                    confirm: "Xoá các kênh đã chọn khỏi danh sách Breaking?",
                    variant: "ghost",
                    onRun: async ([id]) => {
                      await remove.mutateAsync(id);
                    },
                  },
                ]}
              />
            </Can>

            <Table>
              <thead>
                <tr>
                  <Can permission="can_write">
                    <Th className="w-10">
                      <SelectAllBox selection={selection} />
                    </Th>
                  </Can>
                  <Th>Kênh</Th>
                  <Th>Regex (OR)</Th>
                  <Th>Ngôn ngữ</Th>
                  <Th>Người tạo</Th>
                  <Th>Quét</Th>
                  <Th>Auto</Th>
                  <Th>Trạng thái</Th>
                  <Can permission="can_write">
                    <Th className="text-right">Hành động</Th>
                  </Can>
                </tr>
              </thead>
              <tbody>
                {lists.isLoading ? (
                  <EmptyRow colSpan={9}>Đang tải…</EmptyRow>
                ) : lists.data?.items.length ? (
                  lists.data.items.map((list) => (
                    <tr key={list.id}>
                      <Can permission="can_write">
                        <Td>
                          <Checkbox
                            checked={selection.isSelected(list.id)}
                            onChange={() => selection.toggle(list.id)}
                            aria-label="Chọn kênh"
                          />
                        </Td>
                      </Can>
                      <Td className="max-w-xs">
                        <span className="block truncate">{list.source_url}</span>
                        <p className="text-xs text-slate-500">
                          {platformLabel(list.platform)} · mode {list.collect_mode}
                        </p>
                      </Td>
                      <Td>
                        <div className="flex max-w-xs flex-wrap gap-1">
                          {list.regex_patterns.map((p) => (
                            <code
                              key={p}
                              className="rounded bg-slate-100 px-1.5 py-0.5 text-xs"
                              title={p}
                            >
                              {p.length > 28 ? p.slice(0, 28) + "…" : p}
                            </code>
                          ))}
                        </div>
                      </Td>
                      <Td>
                        <Can
                          permission="can_write"
                          fallback={<span>{list.language_default}</span>}
                        >
                          <Select
                            className="h-8 w-36 text-xs"
                            value={list.language_default}
                            disabled={update.isPending}
                            onChange={(e) =>
                              update.mutate({ id: list.id, language_default: e.target.value })
                            }
                          >
                            {LANGUAGE_OPTIONS.some((o) => o.value === list.language_default) ? null : (
                              <option value={list.language_default}>{list.language_default}</option>
                            )}
                            {LANGUAGE_OPTIONS.map((o) => (
                              <option key={o.value} value={o.value}>
                                {o.label}
                              </option>
                            ))}
                          </Select>
                        </Can>
                      </Td>
                      <Td className="whitespace-nowrap text-xs text-slate-600">
                        {list.created_by_email ?? "—"}
                      </Td>
                      <Td className="whitespace-nowrap text-xs">
                        <div>{list.scan_limit} bài/vòng</div>
                        <div className="text-slate-500">
                          {list.scan_interval?.Valid
                            ? formatInterval(list.scan_interval)
                            : "nghỉ theo hệ thống"}
                        </div>
                        <div className="text-slate-400">
                          quét: {formatDateTime(list.last_scanned_at)}
                        </div>
                      </Td>
                      <Td className="text-xs">
                        <div>process: {list.auto_process ? "bật" : "tắt"}</div>
                        <div>publish: {list.auto_publish ? "bật" : "tắt"}</div>
                      </Td>
                      <Td>
                        <Badge tone={statusTone(list.status)}>{list.status}</Badge>
                      </Td>
                      <Can permission="can_write">
                        <Td className="whitespace-nowrap text-right">
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={run.isPending}
                            onClick={() => run.mutate(list.id)}
                          >
                            Quét thử
                          </Button>{" "}
                          <Can permission="can_delete">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => {
                                if (confirm("Xoá kênh này khỏi danh sách Breaking?")) {
                                  remove.mutate(list.id);
                                }
                              }}
                            >
                              Xoá
                            </Button>
                          </Can>
                        </Td>
                      </Can>
                    </tr>
                  ))
                ) : (
                  <EmptyRow colSpan={9}>Chưa có kênh nào.</EmptyRow>
                )}
              </tbody>
            </Table>
          </CardBody>
        </Card>
      </div>
    </>
  );
}

function CreateBreakingForm() {
  const prompts = usePrompts();
  const modes = useCollectModes();
  const create = useCreateBreakingList();

  const [sourceUrl, setSourceUrl] = React.useState("");
  const [patterns, setPatterns] = React.useState<string[]>([""]);
  const [collectMode, setCollectMode] = React.useState<CollectMode>("A");
  const [promptId, setPromptId] = React.useState("");
  const [language, setLanguage] = React.useState("auto");
  const [autoProcess, setAutoProcess] = React.useState(true);
  const [autoPublish, setAutoPublish] = React.useState(false);
  const [showTuning, setShowTuning] = React.useState(false);
  const [scanLimit, setScanLimit] = React.useState("");
  const [scanInterval, setScanInterval] = React.useState("");

  const needsPrompt = collectMode === "C";
  const modeMeta = modes.data?.collect_modes ?? [];
  const isEnabled = (mode: string) =>
    modeMeta.find((m) => m.mode === mode)?.enabled ?? mode === "A";

  function setPattern(index: number, value: string) {
    setPatterns((prev) => prev.map((p, i) => (i === index ? value : p)));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const cleaned = patterns.map((p) => p.trim()).filter(Boolean);
    if (cleaned.length === 0) return;

    await create.mutateAsync({
      source_url: sourceUrl.trim(),
      regex_patterns: cleaned,
      collect_mode: collectMode,
      prompt_id: needsPrompt && promptId ? promptId : null,
      language_default: language,
      auto_process: autoProcess,
      auto_publish: autoPublish,
      scan_limit: scanLimit ? Number(scanLimit) : undefined,
      scan_interval: scanInterval || undefined,
    });
    setSourceUrl("");
    setPatterns([""]);
  }

  return (
    <Card>
      <CardHeader title="Thêm kênh" />
      <CardBody>
        <form onSubmit={submit} className="space-y-4">
          <Field label="URL kênh nguồn">
            <Input
              type="url"
              placeholder="https://www.youtube.com/@kenh"
              value={sourceUrl}
              onChange={(e) => setSourceUrl(e.target.value)}
              required
            />
          </Field>

          <div>
            <div className="mb-1.5 flex items-center justify-between">
              <label className="text-sm font-medium text-slate-700">Điều kiện bắt bài</label>
              <button
                type="button"
                className="text-xs font-medium text-indigo-700 hover:underline"
                onClick={() => setPatterns((prev) => [...prev, ""])}
              >
                + Thêm pattern
              </button>
            </div>
            <div className="space-y-2">
              {patterns.map((pattern, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    placeholder={index === 0 ? "tin nóng" : "#khancap"}
                    value={pattern}
                    onChange={(e) => setPattern(index, e.target.value)}
                    required={index === 0}
                  />
                  {patterns.length > 1 ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setPatterns((prev) => prev.filter((_, i) => i !== index))}
                    >
                      Xoá
                    </Button>
                  ) : null}
                </div>
              ))}
            </div>
            <p className="mt-1 text-xs text-slate-500">
              Khớp <strong>một</strong> pattern là bắt bài (OR). Gõ từ khoá, hashtag, hoặc regex —
              backend tự chuẩn hoá về regex.
            </p>
          </div>

          <Field
            label="Hình thức thu thập"
            hint="B và C cần TTS/LLM thật nên chưa mở — hiện chỉ dùng được A."
          >
            <Select
              value={collectMode}
              onChange={(e) => setCollectMode(e.target.value as CollectMode)}
            >
              {Object.entries(COLLECT_MODE_LABELS).map(([value, label]) => (
                <option key={value} value={value} disabled={!isEnabled(value)}>
                  {label}
                  {isEnabled(value) ? "" : " — chưa hỗ trợ"}
                </option>
              ))}
            </Select>
          </Field>

          {needsPrompt ? (
            <Field label="Prompt mẫu">
              <Select value={promptId} onChange={(e) => setPromptId(e.target.value)} required>
                <option value="">— Chọn prompt —</option>
                {prompts.data?.items.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </Field>
          ) : null}

          <Field
            label="Ngôn ngữ mặc định"
            hint="Bài lẻ trong kênh vẫn sửa lại được ở bảng Bài Post."
          >
            <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
              {LANGUAGE_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
          </Field>

          <label className="flex items-center gap-2 text-sm text-slate-700">
            <Checkbox checked={autoProcess} onChange={(e) => setAutoProcess(e.target.checked)} />
            Tự tạo Voice khi bắt được bài
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <Checkbox checked={autoPublish} onChange={(e) => setAutoPublish(e.target.checked)} />
            Tự đăng lên multime.ai (không qua duyệt)
          </label>

          <div className="rounded-md border border-slate-200 p-3">
            <button
              type="button"
              className="text-sm font-medium text-slate-700"
              onClick={() => setShowTuning((v) => !v)}
            >
              {showTuning ? "▾" : "▸"} Tham số quét (nâng cao)
            </button>
            {showTuning ? (
              <div className="mt-3 space-y-3">
                <Field label="Số bài mỗi vòng quét" hint="Bỏ trống = dùng mặc định hệ thống (20).">
                  <Input
                    type="number"
                    min={1}
                    max={200}
                    placeholder="20"
                    value={scanLimit}
                    onChange={(e) => setScanLimit(e.target.value)}
                  />
                </Field>
                <Field
                  label="Khoảng nghỉ giữa 2 vòng"
                  hint="Ví dụ 30s, 2m. Bỏ trống = theo hệ thống (60s). Tối thiểu 15s."
                >
                  <Input
                    placeholder="60s"
                    value={scanInterval}
                    onChange={(e) => setScanInterval(e.target.value)}
                  />
                </Field>
              </div>
            ) : null}
          </div>

          <ErrorNote error={create.error} />

          <Button type="submit" className="w-full" disabled={create.isPending}>
            {create.isPending ? "Đang lưu…" : "Thêm kênh"}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
