"use client";

import * as React from "react";

import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { ErrorNote, PageHeader } from "@/components/page-header";
import {
  ChannelTuning,
  emptyTuning,
  tuningCreatePayload,
  tuningDraftOf,
  tuningUpdatePayload,
} from "@/components/channel-tuning";
import {
  ScheduleFields,
  describeSchedule,
  emptySchedule,
  scheduleDraftOf,
  toChannelSchedule,
} from "@/components/schedule-fields";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Field, Input, Select, Toggle } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, RowActions, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  useChannelScanSupport,
  useCollectModes,
  useCreateScheduledList,
  useDeleteScheduledList,
  useLLMAPISets,
  usePlatforms,
  usePrompts,
  useScheduledLists,
  useUpdateScheduledList,
} from "@/hooks/use-api";
import { LANGUAGE_OPTIONS, compactLanguageOptions } from "@/lib/languages";
import {
  COLLECT_MODE_LABELS,
  collectModeLabel,
  formatDateTime,
  formatInterval,
  platformLabel,
} from "@/lib/utils";
import type { CollectMode, ListScheduled, PgInterval } from "@/types/api";

const FREQUENCIES = [
  { value: "15m", label: "Mỗi 15 phút", seconds: 900 },
  { value: "30m", label: "Mỗi 30 phút", seconds: 1800 },
  { value: "1h", label: "Mỗi giờ", seconds: 3600 },
  { value: "6h", label: "Mỗi 6 giờ", seconds: 21600 },
  { value: "12h", label: "Mỗi 12 giờ", seconds: 43200 },
  { value: "24h", label: "Mỗi ngày", seconds: 86400 },
];

/**
 * Đổi INTERVAL của Postgres về đúng giá trị trong ô chọn tần suất.
 *
 * Kênh cũ có thể mang tần suất không nằm trong danh sách (đặt qua API, hoặc
 * danh sách này đổi sau đó). Trả về "<số giây>s" cho trường hợp ấy thay vì
 * lặng lẽ nhảy về "1h" — sửa một thứ khác trên kênh không được phép đổi luôn
 * nhịp quét của nó.
 */
function frequencyValue(iv: PgInterval | undefined): string {
  const seconds = Math.round((iv?.Microseconds ?? 0) / 1_000_000) + (iv?.Days ?? 0) * 86_400;
  return FREQUENCIES.find((f) => f.seconds === seconds)?.value ?? `${seconds}s`;
}

/** F3 — Danh sách Định kỳ: mỗi kênh có tần suất quét riêng (business rule #5). */
export default function ScheduledListsPage() {
  const [search, setSearch] = React.useState("");
  const [platform, setPlatform] = React.useState("");
  const [status, setStatus] = React.useState("");
  const [creating, setCreating] = React.useState(false);
  // Kênh đang sửa. Giữ cả object chứ không chỉ id: dialog cần giá trị hiện tại
  // để đổ vào form, và bảng đã có sẵn chúng rồi.
  const [editing, setEditing] = React.useState<ListScheduled | null>(null);
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  // Nút "Xoá lọc" chỉ hiện khi thực sự có gì để xoá.
  const hasFilters = Boolean(search || platform || status);

  const lists = useScheduledLists({
    search: search || undefined,
    platform: platform || undefined,
    status: status || undefined,
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
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
                <Th>Tần suất</Th>
                <Th>Hình thức</Th>
                <Th>Ngôn ngữ</Th>
                <Th>Người tạo</Th>
                <Th>Kết quả</Th>
                <SortableTh sorting={sorting} column="last_scanned_at">
                  Thời gian
                </SortableTh>
                <Th>Cấu hình quét</Th>
                <Th>Trạng thái</Th>
                <Can permission="can_write">
                  <Th className="text-right">Hành động</Th>
                </Can>
              </tr>
            </thead>
            <tbody>
              {lists.isLoading ? (
                <EmptyRow colSpan={11}>Đang tải…</EmptyRow>
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
                      <p className="text-xs text-slate-500">{platformLabel(list.platform)}</p>
                    </Td>
                    <Td className="whitespace-nowrap">{formatInterval(list.scan_frequency)}</Td>
                    <Td className="text-xs">{collectModeLabel(list.collect_mode)}</Td>
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

                    {/* Kênh này đã ra được gì. Không có số ở đây thì câu hỏi
                        "kênh chạy chưa" phải trả lời bằng cách sang màn Bài
                        Post lọc theo kênh — mà cột Quét chỉ nói kênh đã chạy,
                        không nói nó có bắt được bài nào. */}
                    <Td className="whitespace-nowrap text-xs">
                      <div className="text-slate-900">{list.post_count ?? 0} bài post</div>
                      <div className="text-slate-500">{list.voice_count ?? 0} voice</div>
                    </Td>

                    {/* Hai mốc thời gian đứng cạnh nhau: "thêm lúc nào" và
                        "chạy lần cuối lúc nào" chỉ có nghĩa khi đọc cùng lúc —
                        kênh thêm hôm qua mà chưa quét lần nào là một vấn đề,
                        kênh thêm 5 phút trước thì không. */}
                    <Td className="max-w-56 text-xs">
                      <div className="whitespace-nowrap text-slate-500">
                        tạo: {formatDateTime(list.created_at)}
                      </div>
                      <div className="whitespace-nowrap text-slate-900">
                        quét: {formatDateTime(list.last_scanned_at)}
                      </div>
                      {/* Lỗi vòng quét gần nhất. Không có dòng này thì kênh
                          hỏng và kênh chưa có bài mới trông y hệt nhau. */}
                      {list.last_error ? (
                        <p className="mt-1 font-medium text-red-700">{list.last_error}</p>
                      ) : null}
                    </Td>

                    <Td className="whitespace-nowrap text-xs">
                      <div>{list.scan_limit} bài/vòng</div>
                      <div className="text-slate-500">
                        trần: {list.max_posts_per_run ?? "không giới hạn"}
                      </div>
                      <div className="text-slate-400">
                        {list.backfill_done_at
                          ? "bài cũ: đã xong"
                          : `bài cũ: ${list.backfill_limit || "không lấy"}`}
                      </div>
                      <div className="text-slate-400">lịch: {describeSchedule(list)}</div>
                      <div className="max-w-32 truncate text-slate-400">
                        sync: {list.last_synced_post_id ?? "—"}
                      </div>
                    </Td>
                    <Td>
                      {/* Trạng thái là công tắc, không phải nhãn đọc-rồi-đi-tìm-nút:
                          thứ người ta muốn làm với cột này gần như luôn là bật/tắt
                          nó. Người chỉ có quyền đọc vẫn thấy đúng trạng thái, chỉ
                          là không gạt được. */}
                      <Can
                        permission="can_write"
                        fallback={<Badge tone={statusTone(list.status)}>{list.status}</Badge>}
                      >
                        <Toggle
                          checked={list.status === "active"}
                          disabled={update.isPending}
                          onChange={(next) =>
                            update.mutate({ id: list.id, status: next ? "active" : "paused" })
                          }
                          label={list.status === "active" ? "Bật" : "Tắt"}
                        />
                      </Can>
                    </Td>
                    <Can permission="can_write">
                      <Td className="whitespace-nowrap text-right">
                        <RowActions>
                          <Button size="sm" variant="secondary" onClick={() => setEditing(list)}>
                            Sửa
                          </Button>
                          <Can permission="can_delete">
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={() => {
                                if (confirm("Xoá kênh này khỏi danh sách Định kỳ?")) {
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
                <EmptyRow colSpan={11}>Chưa có kênh nào.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={lists.data?.total} paging={paging} unit="kênh" />
        </CardBody>
      </Card>

      {creating ? <ScheduledDialog onClose={() => setCreating(false)} /> : null}
      {editing ? <ScheduledDialog list={editing} onClose={() => setEditing(null)} /> : null}
    </>
  );
}

/**
 * Dialog dùng chung cho THÊM và SỬA kênh.
 *
 * Một form chứ không hai: mọi trường ở đây đều sửa được sau khi tạo, nên tách
 * ra hai dialog chỉ tạo ra hai bản sao của cùng một danh sách trường — và bản
 * "sửa" sẽ là bản thiếu trường mỗi lần thêm tính năng mới.
 */
function ScheduledDialog({ list, onClose }: { list?: ListScheduled; onClose: () => void }) {
  const editing = list != null;
  const prompts = usePrompts();
  const modes = useCollectModes();
  const create = useCreateScheduledList();
  const update = useUpdateScheduledList();

  const [sourceUrl, setSourceUrl] = React.useState(list?.source_url ?? "");
  // Nền tảng của URL đang gõ có quét được cả kênh không (backend cũng từ chối,
  // đây chỉ là để biết trước khi bấm Lưu).
  const checkScan = useChannelScanSupport();
  const blocked = checkScan(sourceUrl);
  // Giai đoạn hiện tại ưu tiên Mode A (extract audio gốc).
  const [collectMode, setCollectMode] = React.useState<CollectMode>(list?.collect_mode ?? "A");
  const [promptId, setPromptId] = React.useState(list?.prompt_id ?? "");
  const [frequency, setFrequency] = React.useState(
    list ? frequencyValue(list.scan_frequency) : "1h",
  );
  const [language, setLanguage] = React.useState(list?.language_default ?? "auto");
  const [autoProcess, setAutoProcess] = React.useState(list?.auto_process ?? false);
  const [autoPublish, setAutoPublish] = React.useState(list?.auto_publish ?? false);
  // Sửa kênh thì mở sẵn phần tham số: người vào đây thường là để chỉnh đúng
  // mấy con số đó, giấu đi lại bắt bấm thêm một lần.
  const [showTuning, setShowTuning] = React.useState(editing);
  const [tuning, setTuning] = React.useState(list ? tuningDraftOf(list) : emptyTuning);
  // Bộ API key cho hình thức C. Quét tự động không có ai bấm nút để chọn bộ,
  // nên bộ phải nằm sẵn trên kênh — không gán thì mode C của kênh không chạy.
  const [llmSetId, setLlmSetId] = React.useState(list?.llm_api_set_id ?? "");
  const [schedule, setSchedule] = React.useState(list ? scheduleDraftOf(list) : emptySchedule);

  const llmSets = useLLMAPISets();
  const needsPrompt = collectMode === "C";
  const modeMeta = modes.data?.collect_modes ?? [];
  const isEnabled = (mode: string) =>
    modeMeta.find((m) => m.mode === mode)?.enabled ?? mode === "A";
  const pending = create.isPending || update.isPending;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const common = {
      source_url: sourceUrl.trim(),
      collect_mode: collectMode,
      prompt_id: needsPrompt && promptId ? promptId : null,
      scan_frequency: frequency,
      language_default: language,
      auto_process: autoProcess,
      auto_publish: autoPublish,
      llm_api_set_id: needsPrompt && llmSetId ? llmSetId : null,
      schedule: toChannelSchedule(schedule),
    };
    if (list) {
      await update.mutateAsync({ id: list.id, ...common, ...tuningUpdatePayload(tuning) });
    } else {
      await create.mutateAsync({ ...common, ...tuningCreatePayload(tuning) });
    }
    onClose();
  }

  return (
    <Modal
      title={editing ? "Sửa kênh Định kỳ" : "Thêm kênh Định kỳ"}
      width="2xl"
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        {/* Nhận diện được URL của một nền tảng không có nghĩa là quét được
            kênh của nó: yt-dlp lấy từng bài X/Facebook/Instagram bình thường
            nhưng không đọc được dòng thời gian. Nói ngay lúc gõ URL, chứ để
            người dùng bấm Lưu rồi mới báo thì họ đã điền xong cả form. */}
        <Field label="URL kênh nguồn" required error={blocked?.reason}>
          <Input
            type="url"
            placeholder="https://www.youtube.com/@kenh"
            value={sourceUrl}
            onChange={(e) => setSourceUrl(e.target.value)}
            required
            autoFocus
          />
        </Field>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            label="Tần suất quét"
            required
            hint="Tối thiểu 1 phút để tránh vượt rate-limit nền tảng."
          >
            <Select value={frequency} onChange={(e) => setFrequency(e.target.value)}>
              {FREQUENCIES.map((f) => (
                <option key={f.value} value={f.value}>
                  {f.label}
                </option>
              ))}
              {/* Tần suất cũ không nằm trong danh sách vẫn phải chọn lại được. */}
              {FREQUENCIES.every((f) => f.value !== frequency) ? (
                <option value={frequency}>{frequency} (đang đặt)</option>
              ) : null}
            </Select>
          </Field>

          <Field
            label="Hình thức thu thập"
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

        <Field label="Ngôn ngữ mặc định">
          <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
            {LANGUAGE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
        </Field>

        <ScheduleFields value={schedule} onChange={setSchedule} withFixedTimes />

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
            <div className="mt-3">
              <ChannelTuning
                value={tuning}
                onChange={setTuning}
                backfillDone={Boolean(list?.backfill_done_at)}
              />
            </div>
          ) : null}
        </div>

        <ErrorNote error={create.error ?? update.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu thay đổi" : "Thêm kênh"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
