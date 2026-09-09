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
  useCollectModes,
  useCreateScheduledList,
  useDeleteScheduledList,
  usePlatforms,
  usePrompts,
  useScheduledLists,
  useUpdateScheduledList,
} from "@/hooks/use-api";
import {
  COLLECT_MODE_LABELS,
  LANGUAGE_OPTIONS,
  formatDateTime,
  formatInterval,
  platformLabel,
} from "@/lib/utils";
import type { CollectMode } from "@/types/api";

const FREQUENCIES = [
  { value: "15m", label: "Mỗi 15 phút" },
  { value: "30m", label: "Mỗi 30 phút" },
  { value: "1h", label: "Mỗi giờ" },
  { value: "6h", label: "Mỗi 6 giờ" },
  { value: "12h", label: "Mỗi 12 giờ" },
  { value: "24h", label: "Mỗi ngày" },
];

/** F3 — Danh sách Định kỳ: mỗi kênh có tần suất quét riêng (business rule #5). */
export default function ScheduledListsPage() {
  const [search, setSearch] = React.useState("");
  const [platform, setPlatform] = React.useState("");
  const { perms } = usePermissions();
  const lists = useScheduledLists({
    search: search || undefined,
    platform: platform || undefined,
  });
  const platforms = usePlatforms();
  const update = useUpdateScheduledList();
  const remove = useDeleteScheduledList();
  const selection = useSelection(lists.data?.items);

  return (
    <>
      <PageHeader
        title="F3 — Danh sách Định kỳ"
        description="Quét theo tần suất riêng từng kênh, lấy toàn bộ bài mới hơn mốc đã sync (không lọc regex)."
      />

      <ViewerNotice />

      <div className={perms.can_write ? "grid gap-6 xl:grid-cols-[440px_1fr]" : ""}>
        <Can permission="can_write">
          <CreateScheduledForm />
        </Can>

        <Card>
          <CardHeader
            title="Kênh đang theo dõi"
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
            <ErrorNote error={lists.error ?? remove.error ?? update.error} />

            <Can permission="can_write">
              <BulkBar
                count={selection.count}
                ids={selection.selected}
                onDone={selection.clear}
                actions={[
                  {
                    label: "Tạm dừng",
                    onRun: async ([id]) => {
                      await update.mutateAsync({ id, status: "paused" });
                    },
                  },
                  {
                    label: "Kích hoạt",
                    variant: "primary",
                    onRun: async ([id]) => {
                      await update.mutateAsync({ id, status: "active" });
                    },
                  },
                  {
                    label: "Xoá",
                    permission: "can_delete",
                    confirm: "Xoá các kênh đã chọn khỏi danh sách Định kỳ?",
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
                  <Th>Tần suất</Th>
                  <Th>Mode</Th>
                  <Th>Ngôn ngữ</Th>
                  <Th>Người tạo</Th>
                  <Th>Quét</Th>
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
                          {platformLabel(list.platform)} · {formatDateTime(list.created_at)}
                        </p>
                      </Td>
                      <Td className="whitespace-nowrap">{formatInterval(list.scan_frequency)}</Td>
                      <Td>{list.collect_mode}</Td>
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
                          trần: {list.max_posts_per_run ?? "không giới hạn"}
                        </div>
                        <div className="text-slate-400">
                          quét: {formatDateTime(list.last_scanned_at)}
                        </div>
                        <div className="max-w-32 truncate text-slate-400">
                          sync: {list.last_synced_post_id ?? "—"}
                        </div>
                      </Td>
                      <Td>
                        <Badge tone={statusTone(list.status)}>{list.status}</Badge>
                      </Td>
                      <Can permission="can_write">
                        <Td className="whitespace-nowrap text-right">
                          <Can permission="can_delete">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => {
                                if (confirm("Xoá kênh này khỏi danh sách Định kỳ?")) {
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

function CreateScheduledForm() {
  const prompts = usePrompts();
  const modes = useCollectModes();
  const create = useCreateScheduledList();

  const [sourceUrl, setSourceUrl] = React.useState("");
  // Giai đoạn hiện tại ưu tiên Mode A (extract audio gốc).
  const [collectMode, setCollectMode] = React.useState<CollectMode>("A");
  const [promptId, setPromptId] = React.useState("");
  const [frequency, setFrequency] = React.useState("1h");
  const [language, setLanguage] = React.useState("auto");
  const [autoProcess, setAutoProcess] = React.useState(false);
  const [autoPublish, setAutoPublish] = React.useState(false);
  const [showTuning, setShowTuning] = React.useState(false);
  const [scanLimit, setScanLimit] = React.useState("");
  const [maxPostsPerRun, setMaxPostsPerRun] = React.useState("");

  const needsPrompt = collectMode === "C";
  const modeMeta = modes.data?.collect_modes ?? [];
  const isEnabled = (mode: string) =>
    modeMeta.find((m) => m.mode === mode)?.enabled ?? mode === "A";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await create.mutateAsync({
      source_url: sourceUrl.trim(),
      collect_mode: collectMode,
      prompt_id: needsPrompt && promptId ? promptId : null,
      scan_frequency: frequency,
      language_default: language,
      auto_process: autoProcess,
      auto_publish: autoPublish,
      scan_limit: scanLimit ? Number(scanLimit) : undefined,
      max_posts_per_run: maxPostsPerRun ? Number(maxPostsPerRun) : undefined,
    });
    setSourceUrl("");
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

          <Field label="Tần suất quét" hint="Tối thiểu 1 phút để tránh vượt rate-limit nền tảng.">
            <Select value={frequency} onChange={(e) => setFrequency(e.target.value)}>
              {FREQUENCIES.map((f) => (
                <option key={f.value} value={f.value}>
                  {f.label}
                </option>
              ))}
            </Select>
          </Field>

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
            Tự tạo Voice ngay (tắt để gom bài duyệt hàng loạt, tiết kiệm chi phí AI)
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <Checkbox checked={autoPublish} onChange={(e) => setAutoPublish(e.target.checked)} />
            Tự đăng lên multime.ai
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
                <Field label="Số bài mỗi vòng quét" hint="Bỏ trống = mặc định hệ thống (20).">
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
                  label="Trần Bài Post mỗi vòng"
                  hint="Chặn nổ chi phí AI khi kênh đăng ồ ạt. Bỏ trống = mặc định hệ thống (50), 0 = không giới hạn."
                >
                  <Input
                    type="number"
                    min={0}
                    placeholder="50"
                    value={maxPostsPerRun}
                    onChange={(e) => setMaxPostsPerRun(e.target.value)}
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
