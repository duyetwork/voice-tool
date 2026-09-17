"use client";

import * as React from "react";

import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { CreatorFilter } from "@/components/creator-filter";
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
import { ScanHistoryPanel, ScanStatusBadge } from "@/components/scan-history";
import { Badge, statusTone } from "@/components/ui/badge";
import { Combobox } from "@/components/ui/combobox";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Field, Input, Select, Toggle } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, RowActions, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  useCatalog,
  platformFromURL,
  useChannelScanSupport,
  useCollectModes,
  useCreateScheduledList,
  useDeleteScheduledList,
  useLLMAPISets,
  usePlatforms,
  usePrompts,
  useRunScheduledList,
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

/** Các tab trong modal chi tiết kênh. */
type ChannelTab = "config" | "scans";

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
  const [createdBy, setCreatedBy] = React.useState("");
  const [creating, setCreating] = React.useState(false);
  // Kênh đang sửa. Giữ cả object chứ không chỉ id: dialog cần giá trị hiện tại
  // để đổ vào form, và bảng đã có sẵn chúng rồi.
  // Kênh đang mở, kèm tab mở sẵn — xem ghi chú ở màn Breaking.
  const [editing, setEditing] = React.useState<{ list: ListScheduled; tab: ChannelTab } | null>(
    null,
  );
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  // Nút "Xoá lọc" chỉ hiện khi thực sự có gì để xoá.
  const hasFilters = Boolean(search || platform || status || createdBy);

  const lists = useScheduledLists({
    search: search || undefined,
    platform: platform || undefined,
    status: status || undefined,
    created_by: createdBy || undefined,
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });
  const platforms = usePlatforms();
  const run = useRunScheduledList();
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

          <CreatorFilter
            value={createdBy}
            onChange={(id) => {
              setCreatedBy(id);
              paging.reset();
            }}
          />

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
                setCreatedBy("");
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
                <Th>Vòng quét</Th>
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
                <EmptyRow colSpan={12}>Đang tải…</EmptyRow>
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

                    {/* Trạng thái vòng quét gần nhất. Không có cột này thì
                        "kênh đang chạy dở" và "kênh đã xong từ lâu" trông y
                        hệt nhau — cùng một last_scanned_at cũ. */}
                    <Td className="whitespace-nowrap">
                      <ScanStatusBadge status={list.last_run_status} />
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
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => setEditing({ list, tab: "config" })}
                          >
                            Sửa
                          </Button>
                          {/* Lối vào riêng cho lịch sử quét: không ai bấm Sửa
                              để XEM. */}
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => setEditing({ list, tab: "scans" })}
                          >
                            Lịch sử
                          </Button>
                          {/* Kênh đặt tần suất 6 tiếng thì không có cách nào
                              thử cấu hình vừa sửa ngoài việc ngồi chờ. */}
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={run.isPending}
                            onClick={() => run.mutate(list.id)}
                          >
                            Quét thử
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
                <EmptyRow colSpan={12}>Chưa có kênh nào.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={lists.data?.total} paging={paging} unit="kênh" />
        </CardBody>
      </Card>

      {creating ? <ScheduledDialog onClose={() => setCreating(false)} /> : null}
      {editing ? (
        <ScheduledDialog
          list={editing.list}
          initialTab={editing.tab}
          onClose={() => setEditing(null)}
        />
      ) : null}
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
function ScheduledDialog({
  list,
  initialTab = "config",
  onClose,
}: {
  list?: ListScheduled;
  initialTab?: ChannelTab;
  onClose: () => void;
}) {
  const editing = list != null;
  // Kênh mới chưa quét lần nào nên không có gì để xem — tab chỉ hiện khi sửa.
  const [tab, setTab] = React.useState<ChannelTab>(initialTab);
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
  // auto_publish không còn ô riêng — nó đi theo autoProcess (xem CreateScheduled
  // ở backend). randomAuthor mới là thứ quyết định voice của kênh có đứng tên
  // được ai không.
  const [randomAuthor, setRandomAuthor] = React.useState(list?.random_author ?? false);
  // Sửa kênh thì mở sẵn phần tham số: người vào đây thường là để chỉnh đúng
  // mấy con số đó, giấu đi lại bắt bấm thêm một lần.
  const [showTuning, setShowTuning] = React.useState(editing);
  const [tuning, setTuning] = React.useState(list ? tuningDraftOf(list) : emptyTuning);
  // Bộ API key cho hình thức C. Quét tự động không có ai bấm nút để chọn bộ,
  // nên bộ phải nằm sẵn trên kênh — không gán thì mode C của kênh không chạy.
  const [llmSetId, setLlmSetId] = React.useState(list?.llm_api_set_id ?? "");
  const [schedule, setSchedule] = React.useState(list ? scheduleDraftOf(list) : emptySchedule);
  // Quốc gia của kênh: mọi Bài Post và Voice của kênh thuộc về nước này, và đó
  // là thứ lọc danh bạ tài khoản ở bước đăng. Để trống thì hệ thống suy từ ngôn
  // ngữ như trước — đoán đúng với tiếng Việt, đoán bừa với tiếng Anh.
  const [countryId, setCountryId] = React.useState(list?.country_id ? String(list.country_id) : "");
  const catalog = useCatalog();
  const countryOptions = React.useMemo(
    () => (catalog.data?.countries ?? []).map((c) => ({ value: String(c.id), label: c.name })),
    [catalog.data?.countries],
  );

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
      random_author: randomAuthor,
      llm_api_set_id: needsPrompt && llmSetId ? llmSetId : null,
      schedule: toChannelSchedule(schedule),
      // 0 = gỡ quốc gia (id của Strongbody luôn > 0) — xem scanConfigRequest ở
      // backend. Không gửi 0 thì người dùng bỏ chọn nước mà kênh vẫn giữ nước cũ.
      country_id: countryId ? Number(countryId) : 0,
    };
    if (list) {
      await update.mutateAsync({ id: list.id, ...common, ...tuningUpdatePayload(tuning) });
    } else {
      await create.mutateAsync({ ...common, ...tuningCreatePayload(tuning) });
    }
    onClose();
  }

  return (
    <Modal title={editing ? "Kênh Định kỳ" : "Thêm kênh Định kỳ"} width="2xl" onClose={onClose}>
      {editing ? (
        <div className="mb-4 grid grid-cols-2 gap-2 rounded-lg bg-slate-100 p-1">
          {(
            [
              ["config", "Cấu hình"],
              ["scans", "Lịch sử quét"],
            ] as [ChannelTab, string][]
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              onClick={() => setTab(value)}
              className={
                "rounded-md px-3 py-1.5 text-sm font-medium transition " +
                (tab === value
                  ? "bg-white text-slate-900 shadow-sm"
                  : "text-slate-600 hover:text-slate-900")
              }
            >
              {label}
            </button>
          ))}
        </div>
      ) : null}

      {editing && tab === "scans" ? (
        <ScanHistoryPanel kind="scheduled" listId={list.id} />
      ) : (
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
            <Field label="Tần suất quét" required>
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

            <Field label="Hình thức thu thập">
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
              <Field label="Bộ API">
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

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Ngôn ngữ mặc định">
              <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
                {LANGUAGE_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
            </Field>

            <Field label="Quốc gia">
              <Combobox
                value={countryId}
                options={countryOptions}
                onChange={setCountryId}
                emptyLabel="Chọn quốc gia"
                placeholder="Chọn quốc gia"
              />
            </Field>
          </div>

          <ScheduleFields value={schedule} onChange={setSchedule} />

          {/* Chỉ còn HAI ô, và "tự đăng lên multime" không nằm trong số đó:
            tạo voice tự động mà không đăng thì bài nằm lại ở nháp và vẫn phải
            vào bấm tay từng cái — tức là không tự động. Tick ô thứ nhất là
            tạo xong đăng luôn. */}
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <Checkbox
              checked={autoProcess}
              onChange={(e) => {
                setAutoProcess(e.target.checked);
                // Bật tự động thì bật luôn Random author: không có ai ngồi chọn
                // tài khoản đứng tên cho từng voice của kênh, mà multime bắt buộc
                // phải có — bỏ trống là voice chạy xong rồi hỏng ở bước đăng.
                // Vẫn bỏ tick lại được nếu muốn tự gán tay sau.
                if (e.target.checked) setRandomAuthor(true);
              }}
            />
            Tự động tạo Voice và đăng lên multime.ai (tắt để gom bài duyệt hàng loạt)
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <Checkbox checked={randomAuthor} onChange={(e) => setRandomAuthor(e.target.checked)} />
            Random author — bốc tài khoản đứng tên bài theo ngôn ngữ của kênh
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
                  platform={list?.platform ?? platformFromURL(sourceUrl)}
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
      )}
    </Modal>
  );
}
