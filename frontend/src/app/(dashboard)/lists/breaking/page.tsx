"use client";

import * as React from "react";

import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { ErrorNote, PageHeader } from "@/components/page-header";
import {
  ScheduleFields,
  describeSchedule,
  emptySchedule,
  toChannelSchedule,
} from "@/components/schedule-fields";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Field, Input, Select } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, RowActions, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  useBreakingLists,
  useCollectModes,
  useCreateBreakingList,
  useLLMAPISets,
  useDeleteBreakingList,
  usePlatforms,
  usePrompts,
  useRunBreakingList,
  useUpdateBreakingList,
} from "@/hooks/use-api";
import { LANGUAGE_OPTIONS, compactLanguageOptions } from "@/lib/languages";
import {
  COLLECT_MODE_LABELS,
  collectModeLabel,
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
  const [status, setStatus] = React.useState("");
  const [creating, setCreating] = React.useState(false);
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  // Nút "Xoá lọc" chỉ hiện khi thực sự có gì để xoá.
  const hasFilters = Boolean(search || platform || status);

  const lists = useBreakingLists({
    search: search || undefined,
    platform: platform || undefined,
    status: status || undefined,
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
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
      >
        <Can permission="can_write">
          <Button onClick={() => setCreating(true)}>+ Thêm kênh</Button>
        </Can>
      </PageHeader>

      <Card>
        <CardBody className="flex flex-wrap items-end gap-3 border-b border-slate-200">
          <div className="w-40">
            <label className="mb-1 block text-xs font-medium text-slate-500">Nền tảng</label>
            <Select
              value={platform}
              onChange={(e) => {
                setPlatform(e.target.value);
                paging.reset();
              }}
            >
              <option value="">Tất cả</option>
              {(platforms.data?.platforms ?? []).map((p) => (
                <option key={p} value={p}>
                  {platformLabel(p)}
                </option>
              ))}
            </Select>
          </div>

          <div className="w-40">
            <label className="mb-1 block text-xs font-medium text-slate-500">Trạng thái</label>
            <Select
              value={status}
              onChange={(e) => {
                setStatus(e.target.value);
                paging.reset();
              }}
            >
              <option value="">Tất cả</option>
              <option value="active">active</option>
              <option value="paused">paused</option>
            </Select>
          </div>

          <div className="w-64">
            <label className="mb-1 block text-xs font-medium text-slate-500">Tìm theo URL</label>
            <Input
              placeholder="regex lọc theo URL…"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                paging.reset();
              }}
            />
          </div>

          <Button variant="secondary" onClick={() => lists.refetch()}>
            Làm mới
          </Button>
          {hasFilters ? (
            <Button
              variant="ghost"
              onClick={() => {
                setSearch("");
                setPlatform("");
                setStatus("");
                paging.reset();
              }}
            >
              Xoá lọc
            </Button>
          ) : null}
          <span className="ml-auto text-sm text-slate-500">
            {lists.data ? `${lists.data.total} kênh` : ""}
          </span>
        </CardBody>

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
                  variant: "danger",
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
                <SortableTh sorting={sorting} column="last_scanned_at">
                  Quét
                </SortableTh>
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
                    <Td className="max-w-48">
                      <a
                        href={list.source_url}
                        target="_blank"
                        rel="noreferrer"
                        className="block truncate text-indigo-700 hover:underline"
                      >
                        {list.source_url}
                      </a>
                      <p className="text-xs text-slate-500">
                        {platformLabel(list.platform)} · {collectModeLabel(list.collect_mode)}
                      </p>
                    </Td>
                    <Td>
                      <div className="flex max-w-44 flex-wrap gap-1">
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
                      <Can permission="can_write" fallback={<span>{list.language_default}</span>}>
                        <Select
                          className="h-8 w-28 text-xs"
                          value={list.language_default}
                          disabled={update.isPending}
                          onChange={(e) =>
                            update.mutate({ id: list.id, language_default: e.target.value })
                          }
                        >
                          {compactLanguageOptions(list.language_default).map((o) => (
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
                      <div className="text-slate-400">lịch: {describeSchedule(list)}</div>
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
                        <RowActions>
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={run.isPending}
                            onClick={() => run.mutate(list.id)}
                          >
                            Quét thử
                          </Button>
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={update.isPending}
                            onClick={() =>
                              update.mutate({
                                id: list.id,
                                status: list.status === "active" ? "paused" : "active",
                              })
                            }
                          >
                            {list.status === "active" ? "Tạm dừng" : "Kích hoạt"}
                          </Button>
                          <Can permission="can_delete">
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={() => {
                                if (confirm("Xoá kênh này khỏi danh sách Breaking?")) {
                                  remove.mutate(list.id);
                                }
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
                <EmptyRow colSpan={9}>Chưa có kênh nào.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={lists.data?.total} paging={paging} unit="kênh" />
        </CardBody>
      </Card>

      {creating ? <CreateBreakingDialog onClose={() => setCreating(false)} /> : null}
    </>
  );
}

function CreateBreakingDialog({ onClose }: { onClose: () => void }) {
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
  // Bộ API key cho hình thức C. Quét tự động không có ai bấm nút để chọn bộ,
  // nên bộ phải nằm sẵn trên kênh.
  const [llmSetId, setLlmSetId] = React.useState("");
  const [schedule, setSchedule] = React.useState(emptySchedule);

  const llmSets = useLLMAPISets();
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
      llm_api_set_id: needsPrompt && llmSetId ? llmSetId : null,
      schedule: toChannelSchedule(schedule),
    });
    onClose();
  }

  return (
    <Modal
      title="Thêm kênh Breaking"
      description="Kênh sẽ được quét liên tục; chỉ bài khớp một trong các pattern bên dưới mới tạo Bài Post."
      width="2xl"
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="URL kênh nguồn" required>
          <Input
            type="url"
            placeholder="https://www.youtube.com/@kenh"
            value={sourceUrl}
            onChange={(e) => setSourceUrl(e.target.value)}
            required
            autoFocus
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
                    variant="danger"
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

        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            label="Hình thức thu thập"
            hint="B và C đọc bằng TTS 3voices — cần API key khai ở mục AI Engine (C cần thêm Prompt mẫu)."
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
        </div>

        {needsPrompt ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Prompt mẫu" required>
              <Select value={promptId} onChange={(e) => setPromptId(e.target.value)} required>
                <option value="">— Chọn prompt —</option>
                {prompts.data?.items.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field
              label="Bộ API"
              hint="Quét tự động không có ai bấm nút để chọn bộ — không gán thì hình thức C của kênh này không chạy."
            >
              <Select value={llmSetId} onChange={(e) => setLlmSetId(e.target.value)}>
                <option value="">— Không chọn —</option>
                {llmSets.data?.items.map((set) => (
                  <option key={set.id} value={set.id}>
                    {set.name}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
        ) : null}

        <ScheduleFields value={schedule} onChange={setSchedule} />

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
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
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

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={create.isPending}>
            {create.isPending ? "Đang lưu…" : "Thêm kênh"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
