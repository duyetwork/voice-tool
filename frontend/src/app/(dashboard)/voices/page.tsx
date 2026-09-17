"use client";

import * as React from "react";

import { AudioPreview } from "@/components/audio-preview";
import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { DuplicatePostNotice } from "@/components/duplicate-notice";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { CreatorFilter } from "@/components/creator-filter";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Combobox, MultiCombobox, type ComboOption } from "@/components/ui/combobox";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, RowActions, SortButton, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  duplicateOf,
  useCollectModes,
  useCatalog,
  useHashtagSearch,
  useCreateSourcePost,
  useCreateTextVoice,
  useDeleteVoice,
  useLastUsedChoices,
  useLLMAPISets,
  usePlatforms,
  usePrompts,
  usePublishVoice,
  usePublishRequirements,
  useRegenerateVoice,
  useUpdateVoice,
  useUploadPendingImage,
  useUploadVoiceImage,
  useVoices,
} from "@/hooks/use-api";
import { coverImageUrl } from "@/lib/api";
import {
  LANGUAGE_AUTO,
  LANGUAGE_OPTIONS,
  languageOptionsFor,
  sortLanguages,
} from "@/lib/languages";
import { COLLECT_MODE_LABELS, PUBLISH_STATUS_LABELS, platformLabel } from "@/lib/utils";
import { VoiceStyleFields, isEmptyVoiceStyle } from "@/components/voice-style-fields";
import type {
  CollectMode,
  Country,
  DuplicatePost,
  Gender,
  Hashtag,
  PublishRequirements,
  Voice,
  VoiceStyle,
} from "@/types/api";

/**
 * multimeBlockers trả về các lý do multime.ai sẽ từ chối bài đăng.
 * Hợp đồng API (/v1/seller/voice-posts/upload): bắt buộc author_id, title, ít
 * nhất 1 hashtag, và audio dài tối thiểu 15 giây. Không có trường mô tả nào
 * bắt buộc — `title` là phần chữ duy nhất của bài đăng.
 *
 * Hashtag KHÔNG còn giá trị mặc định phía server: bỏ trống là không đăng được,
 * kể cả khi có category cấu hình sẵn.
 */
function multimeBlockers(voice: Voice, req?: PublishRequirements): string[] {
  const reasons: string[] = [];
  const minDuration = req?.min_duration_seconds ?? 15;

  if (!voice.author_id) reasons.push("chưa chọn author");
  if (!voice.title?.trim()) reasons.push("chưa có tiêu đề");
  if (!voice.hashtag?.trim()) reasons.push("chưa có hashtag");
  if (voice.duration_seconds != null && voice.duration_seconds < minDuration) {
    reasons.push(`chỉ dài ${voice.duration_seconds}s (tối thiểu ${minDuration}s)`);
  }
  return reasons;
}

/**
 * CharCount — dòng đếm ký tự nằm ngay dưới ô nhập.
 *
 * Để trong nhãn thì lúc vượt giới hạn người dùng phải ngước lên mới thấy, mà
 * đó đúng là lúc cần thấy nhất. Vượt giới hạn là đỏ: API sẽ từ chối, nên phải
 * chặn được người dùng trước khi họ bấm Lưu.
 */
function CharCount({ length, max }: { length: number; max: number }) {
  const remaining = max - length;
  return (
    <p className={"mt-1 text-xs " + (remaining < 0 ? "text-red-700" : "text-slate-500")}>
      {remaining >= 0
        ? `Còn ${remaining.toLocaleString("vi-VN")} ký tự`
        : `Vượt ${(-remaining).toLocaleString("vi-VN")} ký tự`}
    </p>
  );
}

/**
 * VoiceImage là ảnh bìa đang gắn với voice, kèm việc nó nằm ở đâu: ảnh tải từ
 * máy nằm trong bucket riêng tư của tool, ảnh còn lại là link công khai của
 * bài gốc. Hai nguồn đó hiển thị bằng hai URL khác nhau.
 */
type VoiceImage = { url: string; uploaded: boolean };

/**
 * voiceImageSrc trả URL hiển thị được cho thẻ <img>.
 *
 * Ảnh tải từ máy KHÔNG dùng được `image_url` đang lưu: đó là địa chỉ nội bộ của
 * storage (`http://minio:9000/…`, bucket riêng tư) — trình duyệt không với tới,
 * nên ảnh vừa tải lên hiện ra ô trắng. Phải đi qua API như file audio.
 */
function voiceImageSrc(voiceId: string, image: VoiceImage): string {
  return image.uploaded ? coverImageUrl(voiceId, image.url) : image.url;
}

/**
 * PublishTimes gộp "Tạo lúc" và "Đăng lúc" vào chung 1 ô, mỗi mốc 1 dòng.
 *
 * Hai cột thời gian riêng đẩy bảng tràn ngang mà phần lớn thời gian chỉ 1 trong
 * 2 có giá trị (voice chưa đăng thì cột "Đăng lúc" trống rỗng suốt).
 */
function PublishTimes({ voice }: { voice: Voice }) {
  const fmt = (value: string) => {
    const d = new Date(value);
    return `${d.toLocaleDateString("vi-VN")} ${d.toLocaleTimeString("vi-VN", {
      hour: "2-digit",
      minute: "2-digit",
    })}`;
  };

  return (
    <div className="space-y-0.5 whitespace-nowrap text-xs leading-tight">
      <p>
        <span className="text-slate-400">Tạo</span> {fmt(voice.created_at)}
      </p>
      <p>
        {voice.published_at ? (
          <>
            <span className="text-slate-400">Đăng</span>{" "}
            <span className="text-slate-900">{fmt(voice.published_at)}</span>
          </>
        ) : (
          <span className="text-slate-400">Chưa đăng</span>
        )}
      </p>
    </div>
  );
}

/**
 * MAX_TITLE_LENGTH khớp `domain.MaxVoiceTitleRunes` phía backend, và cả hai đều
 * lấy theo giới hạn thật của ô tiêu đề bên multime (maxLength 200). API PATCH
 * cũng chặn, đây chỉ là để người dùng biết trước khi bấm Lưu.
 */
const MAX_TITLE_LENGTH = 200;

/**
 * MAX_TTS_TEXT_LENGTH khớp `domain.MaxTTSTextRunes` phía backend — API cũng
 * chặn, đây chỉ để người dùng thấy còn bao nhiêu ký tự trước khi bấm.
 */
const MAX_TTS_TEXT_LENGTH = 20000;

const EMPTY_FILTERS = {
  publish_status: "",
  platform: "",
  created_by: "",
  created_from: "",
  created_to: "",
  published_from: "",
  published_to: "",
};

/**
 * initialFilters cho phép mở thẳng bảng đã lọc sẵn qua URL —
 * `/voices?publish_status=failed` là link mà thanh cảnh báo dùng.
 *
 * Đọc từ `window.location` chứ không dùng `useSearchParams()`: hook đó bắt cả
 * trang phải nằm trong Suspense boundary khi build, đổi lấy một thứ chỉ cần
 * đúng MỘT lần lúc mở trang. Sau đó bộ lọc thuộc về người dùng, URL không còn
 * vai trò gì.
 */
function initialFilters() {
  if (typeof window === "undefined") return EMPTY_FILTERS;
  const status = new URLSearchParams(window.location.search).get("publish_status") ?? "";
  return { ...EMPTY_FILTERS, publish_status: status };
}

export default function VoicesPage() {
  const [filters, setFilters] = React.useState(initialFilters);
  const [editing, setEditing] = React.useState<Voice | null>(null);
  const [creating, setCreating] = React.useState(false);
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);

  // Đổi bộ lọc thì về trang đầu: đang ở trang 5 mà lọc lại còn 12 dòng thì
  // bảng sẽ trống trơn dù có dữ liệu.
  const set = (key: keyof typeof EMPTY_FILTERS) => (value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
    paging.reset();
  };

  // Voice vừa bấm Đăng: giữ id lại để bảng tiếp tục tự làm mới. Bấm Đăng chỉ
  // đưa vào hàng đợi — publish_status chưa đổi ngay, nên nhìn vào dữ liệu đang
  // có thì không biết là còn việc đang chạy.
  const [publishing, setPublishing] = React.useState<string[]>([]);

  const voices = useVoices(
    {
      publish_status: filters.publish_status || undefined,
      platform: filters.platform || undefined,
      created_by: filters.created_by || undefined,
      created_from: filters.created_from || undefined,
      created_to: filters.created_to || undefined,
      published_from: filters.published_from || undefined,
      published_to: filters.published_to || undefined,
      ...sorting.params,
      limit: paging.limit,
      offset: paging.offset,
    },
    publishing,
  );

  // Bỏ theo dõi khi voice đã tới đích (đăng xong hoặc lỗi) — hết việc thì bảng
  // ngừng hỏi lại.
  const items = voices.data?.items;
  React.useEffect(() => {
    if (!items) return;
    setPublishing((ids) => {
      const next = ids.filter((id) => {
        const voice = items.find((v) => v.id === id);
        return (
          voice != null && voice.publish_status !== "published" && voice.publish_status !== "failed"
        );
      });
      return next.length === ids.length ? ids : next;
    });
  }, [items]);

  /** watchPublish theo dõi 1 voice vừa được đưa vào hàng đợi đăng. */
  const watchPublish = React.useCallback((id: string) => {
    setPublishing((ids) => (ids.includes(id) ? ids : [...ids, id]));
  }, []);
  const platforms = usePlatforms();
  const publishReq = usePublishRequirements().data;
  const publish = usePublishVoice();
  const update = useUpdateVoice();
  const remove = useDeleteVoice();
  const selection = useSelection(voices.data?.items);

  // Nút "Xoá lọc" chỉ hiện khi thực sự có gì để xoá.
  const hasFilters = Object.values(filters).some(Boolean);

  const publishable = selection.items.filter(
    (v) =>
      v.publish_status !== "published" &&
      v.voice_file_url &&
      multimeBlockers(v, publishReq).length === 0,
  );

  return (
    <>
      <PageHeader
        title="Voice"
        description="Nghe thử và sửa metadata trước khi đăng. Sau khi đăng thành công, file voice bị xoá khỏi storage nội bộ — chỉ còn link trên multime.ai."
      >
        <Can permission="can_write">
          <Button onClick={() => setCreating(true)}>+ Tạo Voice</Button>
        </Can>
      </PageHeader>

      <Card>
        <CardBody className="flex flex-wrap items-end gap-3 border-b border-slate-200">
          <div className="w-40">
            <label className="mb-1 block text-xs font-medium text-slate-500">Trạng thái</label>
            <Select
              value={filters.publish_status}
              onChange={(e) => set("publish_status")(e.target.value)}
            >
              <option value="">Tất cả</option>
              {Object.entries(PUBLISH_STATUS_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Select>
          </div>

          <div className="w-40">
            <label className="mb-1 block text-xs font-medium text-slate-500">Nền tảng</label>
            <Select value={filters.platform} onChange={(e) => set("platform")(e.target.value)}>
              <option value="">Tất cả</option>
              {(platforms.data?.platforms ?? []).map((p) => (
                <option key={p} value={p}>
                  {platformLabel(p)}
                </option>
              ))}
            </Select>
          </div>

          <CreatorFilter value={filters.created_by} onChange={(id) => set("created_by")(id)} />

          <div>
            <label className="mb-1 block text-xs font-medium text-slate-500">Ngày tạo</label>
            <div className="flex items-center gap-1">
              <Input
                type="date"
                className="w-36"
                value={filters.created_from}
                onChange={(e) => set("created_from")(e.target.value)}
              />
              <span className="text-slate-400">→</span>
              <Input
                type="date"
                className="w-36"
                value={filters.created_to}
                onChange={(e) => set("created_to")(e.target.value)}
              />
            </div>
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-slate-500">Ngày đăng</label>
            <div className="flex items-center gap-1">
              <Input
                type="date"
                className="w-36"
                value={filters.published_from}
                onChange={(e) => set("published_from")(e.target.value)}
              />
              <span className="text-slate-400">→</span>
              <Input
                type="date"
                className="w-36"
                value={filters.published_to}
                onChange={(e) => set("published_to")(e.target.value)}
              />
            </div>
          </div>

          <Button variant="secondary" onClick={() => voices.refetch()}>
            Làm mới
          </Button>
          {hasFilters ? (
            <Button
              variant="ghost"
              onClick={() => {
                setFilters(EMPTY_FILTERS);
                paging.reset();
              }}
            >
              Xoá lọc
            </Button>
          ) : null}
          <span className="ml-auto text-sm text-slate-500">
            {voices.data ? `${voices.data.total} voice` : ""}
          </span>
        </CardBody>

        <CardBody className="p-0">
          <ErrorNote error={voices.error ?? publish.error ?? remove.error ?? update.error} />

          <Can permission="can_write">
            <BulkBar
              count={selection.count}
              ids={selection.selected}
              onDone={selection.clear}
              actions={[
                {
                  label: `Đăng (${publishable.length} đủ điều kiện)`,
                  onRun: async ([id]) => {
                    const voice = selection.items.find((v) => v.id === id);
                    if (!voice || voice.publish_status === "published") return;
                    if (multimeBlockers(voice, publishReq).length > 0) {
                      throw new Error(
                        `${voice.title ?? voice.id.slice(0, 8)}: ${multimeBlockers(voice, publishReq).join(", ")}`,
                      );
                    }
                    watchPublish(id);
                    await publish.mutateAsync(id);
                  },
                  variant: "primary",
                },
                {
                  label: "Đặt ngôn ngữ tự nhận diện",
                  onRun: async ([id]) => {
                    await update.mutateAsync({ id, language: "auto" });
                  },
                },
                {
                  label: "Xoá",
                  permission: "can_delete",
                  confirm: "Xoá các voice đã chọn?",
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
                <Th>Nội dung / nghe thử</Th>
                <Th>Nền tảng</Th>
                <Th>Người tạo</Th>
                <Th>Author</Th>
                <Th>Trạng thái</Th>
                {/* 1 cột, 2 mốc: bấm nhãn nào thì sắp xếp theo mốc đó. */}
                <Th className="p-0">
                  <div className="flex flex-col items-start gap-1 px-3 py-2">
                    <SortButton sorting={sorting} column="created_at">
                      Tạo lúc
                    </SortButton>
                    <SortButton sorting={sorting} column="published_at">
                      Đăng lúc
                    </SortButton>
                  </div>
                </Th>
                <Can permission="can_write">
                  <Th className="text-right">Hành động</Th>
                </Can>
              </tr>
            </thead>
            <tbody>
              {voices.isLoading ? (
                <EmptyRow colSpan={8}>Đang tải…</EmptyRow>
              ) : voices.data?.items.length ? (
                voices.data.items.map((voice) => {
                  // Voice đang xử lý thì chưa có file/metadata cuối cùng —
                  // cảnh báo "thiếu hashtag" lúc này chỉ là nhiễu.
                  const processing = voice.publish_status === "processing";
                  // isPending của mutation là trạng thái CHUNG cho cả bảng, nên
                  // so id đang gửi đi: bấm Đăng ở 1 dòng không được làm mờ nút
                  // của mọi dòng khác.
                  const publishing = publish.isPending && publish.variables === voice.id;
                  const updating = update.isPending && update.variables?.id === voice.id;
                  const published = voice.publish_status === "published";
                  const blockers =
                    processing || published ? [] : multimeBlockers(voice, publishReq);
                  // Không đăng được lên multime cũng là lỗi: hiện trạng thái
                  // "Lỗi" kèm lý do, thay vì badge "Nháp" trông như bình thường.
                  // Trạng thái hiển thị = đúng quy tắc API dùng để lọc
                  // (queries/voice.sql): đã thử đăng và hỏng thì là "Đăng lỗi",
                  // còn draft/ready mà thiếu điều kiện là "Chưa đủ điều kiện".
                  const status =
                    voice.publish_status !== "failed" && blockers.length > 0
                      ? "incomplete"
                      : voice.publish_status;
                  const errorNote =
                    voice.last_error ?? (blockers.length > 0 ? `${blockers.join(", ")}.` : null);
                  return (
                    <tr key={voice.id}>
                      <Can permission="can_write">
                        <Td>
                          <Checkbox
                            checked={selection.isSelected(voice.id)}
                            onChange={() => selection.toggle(voice.id)}
                            aria-label="Chọn voice"
                          />
                        </Td>
                      </Can>

                      <Td className="max-w-xs">
                        <div className="flex gap-3">
                          {voice.image_url ? (
                            // eslint-disable-next-line @next/next/no-img-element
                            <img
                              src={voiceImageSrc(voice.id, {
                                url: voice.image_url,
                                uploaded: voice.image_uploaded,
                              })}
                              alt=""
                              className="h-12 w-20 shrink-0 rounded object-cover"
                            />
                          ) : null}
                          {/* Bảng hiện phần chữ sẽ được đăng: tiêu đề và
                              hashtag, mỗi phần tối đa 2 dòng. */}
                          <div className="min-w-0">
                            <p
                              className="line-clamp-2 font-medium text-slate-900"
                              title={voice.title ?? undefined}
                            >
                              {voice.title ?? (
                                <span className="font-normal text-slate-400">
                                  (chưa có tiêu đề)
                                </span>
                              )}
                            </p>
                            {voice.hashtag ? (
                              <p
                                className="mt-1 line-clamp-2 text-xs text-indigo-700"
                                title={voice.hashtag}
                              >
                                {voice.hashtag}
                              </p>
                            ) : null}
                          </div>
                        </div>

                        {processing ? (
                          <p className="mt-2 text-xs text-sky-800">
                            Đang tải/tạo audio — dòng này tự cập nhật khi xong.
                          </p>
                        ) : voice.voice_file_url ? (
                          <AudioPreview voiceId={voice.id} />
                        ) : null}
                      </Td>

                      <Td className="whitespace-nowrap">
                        {voice.source_url ? (
                          <a
                            href={voice.source_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-indigo-700 hover:underline"
                          >
                            {platformLabel(voice.platform)}
                          </a>
                        ) : // Voice gõ tay không có bài gốc nào để mở.
                        platformLabel(voice.platform) === "—" && voice.input_text ? (
                          "Text gõ tay"
                        ) : (
                          platformLabel(voice.platform)
                        )}
                      </Td>

                      <Td className="max-w-28 text-xs text-slate-600">
                        <span className="block truncate" title={voice.created_by_email}>
                          {voice.created_by_email ?? "—"}
                        </span>
                      </Td>

                      {/* Author = tài khoản Strongbody ĐỨNG TÊN bài đăng, khác
                          "Người tạo" là người bấm nút trong tool này. Sửa được
                          ngay tại bảng: đây là trường hay phải đổi nhất và là
                          lý do phổ biến nhất khiến voice chưa đăng được. */}
                      <Td className="text-xs">
                        <Can
                          permission="can_write"
                          fallback={
                            <span className="block w-44 break-all text-slate-600">
                              {voice.author_email ?? "—"}
                            </span>
                          }
                        >
                          <AuthorCell
                            voice={voice}
                            disabled={processing || published}
                            busy={updating}
                          />
                        </Can>
                      </Td>

                      <Td className="max-w-48">
                        <Badge tone={statusTone(status)}>
                          {PUBLISH_STATUS_LABELS[status] ?? status}
                        </Badge>
                        {/* Lý do nằm ngay dưới trạng thái — đọc 1 chỗ là hiểu,
                            không phải dò trong ô nội dung. Thiếu điều kiện thì
                            là việc phải điền nốt (màu hổ phách), khác lỗi thật
                            của lần đăng hỏng (màu đỏ). */}
                        {errorNote ? (
                          <p
                            className={
                              "mt-1 text-xs " +
                              (status === "incomplete" ? "text-amber-700" : "text-red-700")
                            }
                          >
                            {errorNote}
                          </p>
                        ) : null}
                        {/* Đã đăng: file nội bộ đã bị xoá, chỗ nghe duy nhất
                            là multime — để cùng chỗ với thông báo lỗi cho nhất
                            quán "thông tin về trạng thái nằm dưới trạng thái". */}
                        {voice.multime_post_url ? (
                          <a
                            href={voice.multime_post_url}
                            target="_blank"
                            rel="noreferrer"
                            className="mt-1 block text-xs text-indigo-700 hover:underline"
                          >
                            Nghe trên multime.ai ↗
                          </a>
                        ) : null}
                      </Td>
                      <Td>
                        <PublishTimes voice={voice} />
                      </Td>

                      <Can permission="can_write">
                        <Td className="whitespace-nowrap text-right">
                          <RowActions>
                            {voice.publish_status !== "published" ? (
                              <>
                                <Button
                                  size="sm"
                                  variant="secondary"
                                  disabled={processing}
                                  title={
                                    processing
                                      ? "Đang xử lý — worker sẽ ghi metadata khi xong"
                                      : undefined
                                  }
                                  onClick={() => setEditing(voice)}
                                >
                                  Sửa metadata
                                </Button>
                                <Button
                                  size="sm"
                                  disabled={
                                    !voice.voice_file_url || publishing || blockers.length > 0
                                  }
                                  title={
                                    blockers.length > 0
                                      ? `multime.ai yêu cầu: ${blockers.join(", ")}`
                                      : undefined
                                  }
                                  onClick={() => {
                                    watchPublish(voice.id);
                                    publish.mutate(voice.id);
                                  }}
                                >
                                  {publishing ? "Đang đăng…" : "Đăng"}
                                </Button>
                              </>
                            ) : null}
                            <Can permission="can_delete">
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={() => {
                                  if (confirm("Xoá voice này?")) remove.mutate(voice.id);
                                }}
                              >
                                Xoá
                              </Button>
                            </Can>
                          </RowActions>
                        </Td>
                      </Can>
                    </tr>
                  );
                })
              ) : (
                <EmptyRow colSpan={8}>Không có Voice khớp bộ lọc.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={voices.data?.total} paging={paging} unit="voice" />
        </CardBody>
      </Card>

      {editing ? <EditVoiceDialog voice={editing} onClose={() => setEditing(null)} /> : null}
      {creating ? <CreateVoiceDialog onClose={() => setCreating(false)} /> : null}
    </>
  );
}

/**
 * LLMSetField — ô chọn Bộ API key cho hình thức C.
 *
 * Bắt buộc phải có mặt ở MỌI chỗ tạo voice bằng hình thức C, không chỉ ở màn
 * tạo lại: bỏ trống thì router rơi về provider khai trong .env của server —
 * ở production không có key nào ở đó, nên voice chạy tới bước LLM rồi hỏng.
 * Người dùng khai key ở AI Engine > LLM Model nhưng không có ô nào để chọn thì
 * đúng là "hình thức 3 không dùng được".
 */
function LLMSetField({
  value,
  onChange,
  hint = "Túi key LLM dùng để viết lại nội dung.",
}: {
  value: string;
  onChange: (next: string) => void;
  hint?: string;
}) {
  const llmSets = useLLMAPISets();
  return (
    <Field label="Bộ API" hint={hint}>
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">— Không chọn —</option>
        {llmSets.data?.items.map((set) => (
          <option key={set.id} value={set.id}>
            {set.name}
          </option>
        ))}
      </Select>
    </Field>
  );
}

/** useModeGate gom việc đọc /meta/collect-modes cho cả 2 tab. */
function useModeGate() {
  const modes = useCollectModes();
  const modeMeta = modes.data?.collect_modes ?? [];
  const metaOf = (mode: string) => modeMeta.find((m) => m.mode === mode);
  return {
    isEnabled: (mode: string) => metaOf(mode)?.enabled ?? mode === "A",
    // Mode tắt thì nói luôn thiếu gì (vd chưa có ANTHROPIC_API_KEY) — server đã
    // trả kèm lý do ở /meta/collect-modes.
    note: (mode: string) =>
      (metaOf(mode)?.enabled ?? mode === "A")
        ? ""
        : ` — ${metaOf(mode)?.reason ?? "chưa được bật"}`,
  };
}

// ---------------------------------------------------------------------------
// Danh mục cho các ô chọn có tìm kiếm
// ---------------------------------------------------------------------------

/** normalizeHashtag: bỏ '#', gộp khoảng trắng, hạ chữ thường. */
function normalizeHashtag(raw: string): string {
  return raw.replace(/#/g, "").trim().replace(/\s+/g, "").toLowerCase();
}

/**
 * splitHashtags tách chuỗi hashtag đang lưu trên Voice ("tinnong vietnam")
 * thành danh sách cho ô chọn nhiều.
 *
 * Nhận cả dấu phẩy lẫn '#' vì chuỗi cũ có thể do người dùng gõ tay theo mọi
 * kiểu; khử trùng vì hai cách gõ khác nhau ("#Tin", "tin") ra cùng một thẻ.
 */
function splitHashtags(raw: string | null | undefined): string[] {
  const out: string[] = [];
  for (const part of (raw ?? "").split(/[\s,]+/)) {
    const tag = normalizeHashtag(part);
    if (tag && !out.includes(tag)) out.push(tag);
  }
  return out;
}

/**
 * invalidURL: link không mở được. Chỉ http/https — yt-dlp không đi đường nào
 * khác, nên bắt ngay ở form thay vì để worker chạy rồi mới hỏng.
 *
 * Tự kiểm thay cho `required`/`type="url"` của trình duyệt: bong bóng mặc định
 * của trình duyệt chặn luôn sự kiện submit, nên mọi lỗi CÒN LẠI của form không
 * bao giờ hiện ra — người dùng sửa xong ô này mới thấy ô sau cũng sai.
 */
function invalidURL(raw: string): boolean {
  try {
    const u = new URL(raw);
    return u.protocol !== "http:" && u.protocol !== "https:";
  } catch {
    return true;
  }
}

/**
 * useLanguageCombo sắp ngôn ngữ theo thứ tự ưu tiên backend đưa xuống (suy ra
 * từ thứ tự quốc gia: Vietnam → vi, United States → en, …).
 *
 * Bỏ `auto` khỏi danh sách: ở modal này "không chọn" đã có nghĩa là lấy theo
 * bài gốc, nên một mục "Tự nhận diện" nữa là hai cách nói cùng một điều.
 */
function useLanguageCombo(order: string[] | undefined): ComboOption[] {
  return React.useMemo(
    () =>
      sortLanguages(
        LANGUAGE_OPTIONS.filter((o) => o.value !== LANGUAGE_AUTO),
        order,
      ).map((o) => ({ value: o.value, label: o.label })),
    [order],
  );
}

/** useCountryCombo giữ nguyên thứ tự backend trả về (đã sắp theo sort_order). */
function useCountryCombo(countries: Country[] | undefined): ComboOption[] {
  return React.useMemo(
    () => (countries ?? []).map((c) => ({ value: String(c.id), label: c.name })),
    [countries],
  );
}

/**
 * useHashtagCombo dựng danh sách cho ô Hashtag.
 *
 * KHÔNG lọc theo ngôn ngữ đang chọn — vì không có gì để lọc. Bên MultiMe
 * hashtag là `category` và entity đó không mang trường ngôn ngữ nào, nên quan
 * hệ hashtag ↔ ngôn ngữ không tồn tại. Lọc theo một tiêu chí không có thật chỉ
 * giấu mất phần lớn tag khỏi người dùng.
 *
 * Chưa gõ gì thì dùng phần đầu danh mục đã nằm sẵn trong cache (tag MultiMe
 * tuyển chọn). Gõ rồi thì hỏi server, vì danh mục có ~94.000 mục.
 */
function useHashtagCombo(featured: Hashtag[] | undefined, keyword: string): ComboOption[] {
  const search = useHashtagSearch(keyword);
  const searched = search.data?.items;
  const typing = keyword.trim().length > 0;

  return React.useMemo(
    () =>
      (typing ? (searched ?? []) : (featured ?? [])).map((h) => ({
        value: h.tag,
        label: h.tag,
        // `system` là tag MultiMe tuyển chọn — nói ra để người dùng biết cái
        // nào là danh mục chính thức, cái nào do người dùng bên đó tự đặt.
        hint: h.kind === "system" ? "chính thức" : undefined,
      })),
    [typing, searched, featured],
  );
}

/**
 * CreateVoiceDialog — MỘT form cho cả ba hình thức tạo voice.
 *
 * Trước đây là hai tab ("Thông tin" cho URL, "Nhập text" cho text gõ tay) với
 * hai bộ trường khác nhau: tab text không có tiêu đề, hashtag, ảnh bìa hay
 * author, nên voice tạo từ đó luôn thiếu đúng những thứ multime bắt buộc và
 * phải vào sửa lại từng cái. Hai tab cũng bắt người dùng tự biết hình thức nào
 * đi với tab nào — mà điều đó đã nằm sẵn ở ô "Hình thức tạo".
 *
 * Giờ ô Hình thức quyết định phần nhập nguồn, còn toàn bộ metadata dùng chung:
 *
 *   A (Extract)      — URL bài đăng; ảnh bìa lấy được từ bài gốc.
 *   B (Text → TTS)   — gõ thẳng Nội dung; TTS đọc đúng chữ đó.
 *   C (Text+Prompt)  — gõ Nội dung làm ĐẦU VÀO cho LLM; thêm Prompt mẫu + Bộ API.
 *
 * Bấm Đăng là đóng hộp thoại: worker tạo audio rồi tự đăng lên multime
 * (publish_when_ready). Không có bước bấm nút thứ hai.
 */
function CreateVoiceDialog({ onClose }: { onClose: () => void }) {
  const createPost = useCreateSourcePost();
  const createVoice = useCreateTextVoice();
  const uploadImage = useUploadPendingImage();
  const prompts = usePrompts();
  // Danh mục quốc gia/hashtag/thứ tự ngôn ngữ: lấy 1 lần rồi cache — mở modal
  // lần sau không gọi API, search chạy hoàn toàn trên dữ liệu này.
  const catalog = useCatalog();
  const gate = useModeGate();
  const lastUsed = useLastUsedChoices();

  const [collectMode, setCollectMode] = React.useState<CollectMode>("A");
  const [promptId, setPromptId] = React.useState("");
  const [llmSetId, setLlmSetId] = React.useState("");
  const [sourceUrl, setSourceUrl] = React.useState("");
  const [text, setText] = React.useState("");
  const [title, setTitle] = React.useState("");
  const [hashtags, setHashtags] = React.useState<string[]>([]);
  const [language, setLanguage] = React.useState("");
  const [countryId, setCountryId] = React.useState("");
  const [author, setAuthor] = React.useState<AuthorChoice>({
    id: null,
    email: null,
    gender: null,
  });
  // Cấu hình giọng đọc: rỗng = giọng mặc định của nhà cung cấp, đúng hành vi
  // của form trước khi có mục này.
  const [voiceStyle, setVoiceStyle] = React.useState<VoiceStyle>({});
  const [image, setImage] = React.useState<VoiceImage>({ url: "", uploaded: false });
  // Xem trước bằng chính file trên máy: URL server trả về trỏ vào storage nội bộ
  // (bucket riêng tư, host chỉ tồn tại trong mạng Docker) nên thẻ <img> không
  // tải được — mà voice chưa tồn tại thì cũng chưa có endpoint ảnh để đi qua.
  const [preview, setPreview] = React.useState("");
  // "Lấy ảnh từ nguồn" MẶC ĐỊNH TẮT: bài đăng không ảnh là hợp lệ, nên không
  // bắt người dùng phải có ảnh. Tick thì dùng ảnh bìa của bài gốc; không tick
  // và cũng không tải ảnh lên thì voice không có ảnh.
  const [useSourceImage, setUseSourceImage] = React.useState(false);
  const [duplicate, setDuplicate] = React.useState<DuplicatePost | null>(null);

  // Thu hồi object URL khi đổi ảnh hoặc đóng hộp thoại, không thì file giữ
  // trong bộ nhớ tới lúc tải lại trang.
  React.useEffect(() => () => URL.revokeObjectURL(preview), [preview]);

  // Prompt + Bộ API chọn sẵn theo lần chạy gần nhất của chính người này. Chỉ
  // điền khi ô còn trống, để không giật mất lựa chọn họ vừa bấm trong lúc
  // request còn đang bay.
  React.useEffect(() => {
    if (!lastUsed.data) return;
    setPromptId((cur) => cur || lastUsed.data.prompt_id || "");
    setLlmSetId((cur) => cur || lastUsed.data.llm_api_set_id || "");
  }, [lastUsed.data]);

  /** fromURL: chỉ hình thức A lấy nội dung từ bài đăng có sẵn. */
  const fromURL = collectMode === "A";
  const needsPrompt = collectMode === "C";
  const titleTooLong = title.length > MAX_TITLE_LENGTH;
  const textTooLong = text.length > MAX_TTS_TEXT_LENGTH;
  const pending = createPost.isPending || createVoice.isPending || uploadImage.isPending;

  const languageOptions = useLanguageCombo(catalog.data?.language_order);
  const countryOptions = useCountryCombo(catalog.data?.countries);
  const [hashtagKeyword, setHashtagKeyword] = React.useState("");
  const hashtagOptions = useHashtagCombo(catalog.data?.hashtags, hashtagKeyword);

  // Hashtag BẮT BUỘC đúng một trường hợp: hình thức B (Text -> TTS).
  //
  //   A — gộp thẻ của bài gốc vào.
  //   C — LLM trả hashtag cùng lời đọc và chúng chỉ ĐIỀN VÀO CHỖ TRỐNG
  //       (Engine.rewrite: chỉ dùng khi voice chưa có thẻ nào), nên bỏ trống ở
  //       đây là giao việc cho LLM chứ không phải bỏ sót.
  //   B — không có bài gốc, cũng không gọi LLM: không ai điền hộ, mà multime
  //       từ chối bài không hashtag.
  const hashtagRequired = collectMode === "B";

  // Lỗi chỉ hiện SAU lần bấm Đăng đầu tiên: bắt lỗi ngay lúc mở form thì người
  // dùng thấy một loạt chữ đỏ trước cả khi kịp gõ chữ nào.
  const [showErrors, setShowErrors] = React.useState(false);

  /**
   * errors — mọi lý do form chưa gửi được, gắn theo TỪNG ô.
   *
   * Nút Đăng luôn bấm được: một nút mờ đi không nói được nó mờ vì ô nào, nên
   * người dùng phải dò lại cả form. Bấm vào thì lỗi hiện ngay dưới đúng ô sai.
   */
  const errors = {
    mode: gate.isEnabled(collectMode)
      ? undefined
      : `Hình thức này chưa dùng được${gate.note(collectMode)}.`,
    prompt:
      needsPrompt && !promptId ? "Bắt buộc — chọn prompt để LLM viết lại nội dung." : undefined,
    sourceUrl: !fromURL
      ? undefined
      : !sourceUrl.trim()
        ? "Bắt buộc — dán link bài đăng cần lấy."
        : invalidURL(sourceUrl.trim())
          ? "Link không hợp lệ — phải là địa chỉ đầy đủ, bắt đầu bằng http:// hoặc https://."
          : undefined,
    text: fromURL
      ? undefined
      : !text.trim()
        ? "Bắt buộc — nhập nội dung."
        : textTooLong
          ? `Vượt quá ${MAX_TTS_TEXT_LENGTH.toLocaleString("vi-VN")} ký tự — rút ngắn lại.`
          : undefined,
    title: titleTooLong
      ? `Vượt quá ${MAX_TITLE_LENGTH.toLocaleString("vi-VN")} ký tự — rút ngắn lại.`
      : undefined,
    hashtag:
      hashtagRequired && hashtags.length === 0 ? "Bắt buộc — thêm ít nhất 1 hashtag." : undefined,
    author: author.gender ? undefined : "Bắt buộc — chọn giới tính tài khoản đứng tên bài đăng.",
  };
  const hasError = Object.values(errors).some(Boolean);
  /** errorOf: chỉ trả lỗi khi đã bấm Đăng ít nhất một lần. */
  const errorOf = (key: keyof typeof errors) => (showErrors ? errors[key] : undefined);

  async function pickImage(file: File | undefined) {
    if (!file) return;
    const uploaded = await uploadImage.mutateAsync(file);
    setImage({ url: uploaded.image_url, uploaded: true });
    setPreview(URL.createObjectURL(file));
    // Tải ảnh lên là chọn ảnh đó — bỏ tick "lấy ảnh từ nguồn" thay vì để hai
    // nguồn ảnh cùng bật rồi người dùng phải đoán cái nào thắng.
    setUseSourceImage(false);
  }

  /** voiceSeed là phần metadata dùng chung, giống nhau ở cả hai đường gửi. */
  function voiceSeed() {
    return {
      title: title.trim() || undefined,
      hashtag: hashtags.join(" ") || undefined,
      language: language || undefined,
      image_url: useSourceImage ? undefined : image.url || undefined,
      image_uploaded: image.uploaded,
      // no_image = "chủ động không ảnh". Chỉ đúng khi người dùng KHÔNG lấy ảnh
      // nguồn và cũng không tải ảnh lên; bỏ trống cả hai mà gửi false thì worker
      // lại tự điền ảnh bài gốc vào.
      no_image: !useSourceImage && !image.url,
      // author_id để trống: việc BỐC tài khoản lùi tới lúc worker đăng bài
      // (xem Engine.ensureAuthor). Ở đây chỉ gửi ý muốn: giới tính + quốc gia.
      author_gender: author.gender,
      author_country_id: countryId ? Number(countryId) : null,
      publish_when_ready: true,
      llm_api_set_id: needsPrompt && llmSetId ? llmSetId : null,
      // Hình thức A không qua TTS: gửi cấu hình giọng lên cũng chỉ là một cột
      // dữ liệu không ai đọc, và sẽ gây hiểu nhầm khi mở lại voice đó.
      tts_config: !fromURL && !isEmptyVoiceStyle(voiceStyle) ? voiceStyle : null,
    };
  }

  async function send(allowDuplicate: boolean) {
    const prompt = needsPrompt && promptId ? promptId : null;

    // Gõ text tay thì KHÔNG sinh Bài Post: không có URL, không có bài gốc nào
    // để truy vết, nên bản ghi đó chỉ là dòng rỗng làm bẩn màn Bài Post
    // (business rule #1, ngoại lệ có chủ đích — xem migration 000011).
    if (!fromURL) {
      await createVoice.mutateAsync({
        text: text.trim(),
        collect_mode: collectMode,
        prompt_id: prompt,
        llm_api_set_id: needsPrompt && llmSetId ? llmSetId : null,
        language: language || undefined,
        voice: voiceSeed(),
      });
      onClose();
      return;
    }

    try {
      await createPost.mutateAsync({
        source_url: sourceUrl.trim(),
        collect_mode: collectMode,
        prompt_id: prompt,
        language: language || undefined,
        auto_process: true,
        allow_duplicate: allowDuplicate || undefined,
        voice: voiceSeed(),
      });
      onClose();
    } catch (err) {
      const dup = duplicateOf(err);
      if (!dup) throw err;
      setDuplicate(dup);
    }
  }

  return (
    <Modal
      title="Tạo Voice"
      description="Chọn hình thức, điền nội dung và metadata rồi bấm Đăng — hệ thống tạo audio và tự đăng lên multime."
      width="3xl"
      onClose={onClose}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          // Bấm Đăng là lúc bật phần báo lỗi lên, kể cả khi form còn sai: đó
          // chính là câu trả lời cho "vì sao bấm mà không có gì xảy ra".
          setShowErrors(true);
          if (hasError) return;
          void send(false);
        }}
        className="space-y-4"
      >
        <Field label="Hình thức tạo" required error={errorOf("mode")}>
          <Select
            value={collectMode}
            onChange={(e) => setCollectMode(e.target.value as CollectMode)}
          >
            {Object.entries(COLLECT_MODE_LABELS).map(([value, label]) => (
              <option key={value} value={value} disabled={!gate.isEnabled(value)}>
                {label}
                {gate.note(value)}
              </option>
            ))}
          </Select>
        </Field>

        {needsPrompt ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Prompt mẫu"
              required
              error={errorOf("prompt")}
              hint="Chọn sẵn theo lần bạn dùng gần nhất."
            >
              <Select value={promptId} onChange={(e) => setPromptId(e.target.value)}>
                <option value="">— Chọn prompt —</option>
                {prompts.data?.items.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </Field>
            <LLMSetField
              value={llmSetId}
              onChange={setLlmSetId}
              hint="Túi key LLM viết lại nội dung. Chọn sẵn theo lần dùng gần nhất."
            />
          </div>
        ) : null}

        {/* Hình thức A lấy nội dung TỪ bài đăng có sẵn nên cần URL; B và C đọc
            đúng đoạn chữ gõ ở đây nên ô URL không còn nghĩa gì. Hiện cả hai là
            bắt người dùng đoán ô nào thắng. */}
        {fromURL ? (
          <Field label="URL bài đăng" required error={errorOf("sourceUrl")}>
            <Input
              type="text"
              inputMode="url"
              placeholder="https://www.youtube.com/watch?v=..."
              value={sourceUrl}
              onChange={(e) => setSourceUrl(e.target.value)}
              autoFocus
            />
          </Field>
        ) : (
          <Field
            label="Nội dung"
            required
            error={errorOf("text")}
            hint={
              needsPrompt
                ? "ĐẦU VÀO cho LLM, không phải lời đọc. Lời đọc là bản LLM viết lại theo Prompt mẫu."
                : "TTS đọc đúng những gì bạn gõ ở đây."
            }
          >
            <Textarea
              className="min-h-40"
              placeholder="Bản tin sáng nay…"
              value={text}
              onChange={(e) => setText(e.target.value)}
              autoFocus
            />
            <CharCount length={text.length} max={MAX_TTS_TEXT_LENGTH} />
          </Field>
        )}

        <Field label="Tiêu đề" error={errorOf("title")}>
          <Textarea
            className="min-h-20"
            placeholder={
              fromURL ? "Bỏ trống thì lấy nội dung bài gốc" : "Bỏ trống thì cắt từ nội dung"
            }
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <CharCount length={title.length} max={MAX_TITLE_LENGTH} />
        </Field>

        {/* Ngôn ngữ và Quốc gia đứng TRƯỚC Hashtag: ngôn ngữ quyết định hashtag
            nào được gợi ý, nên hỏi sau thì gợi ý tới muộn hơn lúc cần. */}
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Ngôn ngữ">
            <Combobox
              value={language}
              options={languageOptions}
              onChange={setLanguage}
              emptyLabel={fromURL ? "— Lấy theo bài gốc —" : "— Mặc định hệ thống —"}
              placeholder={fromURL ? "— Lấy theo bài gốc —" : "— Mặc định hệ thống —"}
            />
          </Field>

          <Field label="Quốc gia">
            <Combobox
              value={countryId}
              options={countryOptions}
              onChange={setCountryId}
              emptyLabel="— Tất cả quốc gia —"
              placeholder="— Tất cả quốc gia —"
            />
          </Field>
        </div>

        <Field label="Hashtag" required={hashtagRequired} error={errorOf("hashtag")}>
          <MultiCombobox
            values={hashtags}
            options={hashtagOptions}
            onChange={setHashtags}
            onSearch={setHashtagKeyword}
            placeholder="#tinnong #vietnam"
            allowCreate
            normalize={normalizeHashtag}
          />
        </Field>

        <Field label="Tài khoản đứng tên bài đăng (author)" required error={errorOf("author")}>
          <AuthorPicker value={author} onChange={setAuthor} />
        </Field>

        {/* Chỉ hình thức B và C mới qua TTS. Hình thức A lấy thẳng audio của
            bài gốc — ở đó không có giọng nào để chỉnh, hiện mục này ra chỉ là
            một nút bấm vào rồi không thấy tác dụng gì. */}
        {fromURL ? null : (
          <VoiceStyleFields
            value={voiceStyle}
            onChange={setVoiceStyle}
            authorGender={author.gender}
          />
        )}

        <Field label="Ảnh bìa">
          <div className="space-y-2">
            {/* "Lấy ảnh từ nguồn" chỉ có nghĩa khi CÓ nguồn: text gõ tay không
                có bài đăng nào để lấy ảnh bìa. */}
            {fromURL ? (
              <label className="flex items-center gap-2 text-sm text-slate-700">
                <Checkbox
                  checked={useSourceImage}
                  onChange={(e) => {
                    setUseSourceImage(e.target.checked);
                    if (e.target.checked) {
                      // Hai nguồn ảnh không cùng thắng được: chọn ảnh nguồn thì
                      // bỏ ảnh vừa tải lên.
                      setImage({ url: "", uploaded: false });
                      setPreview("");
                    }
                  }}
                />
                Lấy ảnh từ nguồn
              </label>
            ) : null}

            <div className="flex items-center gap-3">
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                disabled={uploadImage.isPending}
                onChange={(e) => {
                  void pickImage(e.target.files?.[0]);
                  // Xoá giá trị input để chọn lại đúng file vừa rồi vẫn kích hoạt onChange.
                  e.target.value = "";
                }}
                className="text-sm text-slate-600 file:mr-3 file:rounded-md file:border-0 file:bg-slate-100 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-slate-700 hover:file:bg-slate-200 disabled:text-slate-400"
              />
              {uploadImage.isPending ? (
                <span className="text-xs text-slate-500">Đang tải ảnh…</span>
              ) : null}
            </div>

            {preview ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={preview} alt="" className="h-24 rounded object-cover" />
            ) : null}
          </div>
        </Field>

        {duplicate ? (
          <DuplicatePostNotice
            existing={duplicate}
            pending={createPost.isPending}
            onSkip={onClose}
            onCreateAnyway={() => void send(true)}
          />
        ) : (
          <ErrorNote error={createPost.error ?? createVoice.error ?? uploadImage.error} />
        )}

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          {/* Chỉ mờ đi khi bấm NỮA cũng vô nghĩa: đang gửi, hoặc đang chờ trả
              lời về bài trùng. Form còn thiếu trường thì vẫn bấm được — lỗi
              hiện dưới đúng ô sai, thứ mà một nút mờ không nói được. */}
          <Button type="submit" disabled={pending || duplicate !== null}>
            {pending ? "Đang xử lý…" : "Đăng"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * EditVoiceDialog — 2 việc khác hẳn nhau nên tách thành 2 tab:
 *
 *   Thông tin — sửa phần chữ đi kèm bài đăng (tiêu đề, hashtag, ảnh bìa).
 *               Không đụng tới file audio.
 *   Nội dung  — sửa LỜI ĐỌC rồi đọc lại, ghi đè file audio cũ.
 *
 * Gộp chung một form thì bấm Lưu là mất luôn file voice cũ dù người dùng chỉ
 * định sửa cái hashtag — nên phải là hai nút, hai tab.
 */
function EditVoiceDialog({ voice, onClose }: { voice: Voice; onClose: () => void }) {
  const [tab, setTab] = React.useState<"info" | "content">("info");

  return (
    <Modal title="Sửa Voice" width="3xl" onClose={onClose}>
      {/* Nghe thử ngay trong modal: sửa tiêu đề/lời đọc mà phải đóng modal ra
          bảng mới nghe được thì không đối chiếu được với thứ mình đang sửa. */}
      {voice.voice_file_url ? (
        <div className="mb-4">
          <AudioPreview voiceId={voice.id} />
        </div>
      ) : null}

      <div className="mb-4 grid grid-cols-2 gap-2 rounded-lg bg-slate-100 p-1">
        {(
          [
            ["info", "Thông tin"],
            ["content", "Nội dung"],
          ] as ["info" | "content", string][]
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

      {tab === "info" ? (
        <VoiceInfoTab voice={voice} onClose={onClose} />
      ) : (
        <VoiceContentTab voice={voice} onClose={onClose} />
      )}
    </Modal>
  );
}

/**
 * originalTitle trả nội dung bài GỐC để điền vào ô tiêu đề.
 *
 * `voice.title` là bản đã cắt về 200 ký tự (giới hạn của multime) kèm "…", nên
 * mở modal sửa mà hiện nó thì người dùng thấy một bài cụt và không có cách nào
 * lấy lại phần đuôi. Modal tạo voice điền sẵn nguyên văn bài post, ở đây cũng
 * phải vậy.
 *
 * Chỉ trả lại bản gốc khi tiêu đề đang lưu ĐÚNG LÀ bản cắt của nó — người dùng
 * đã tự sửa tiêu đề thì giữ nguyên thứ họ viết.
 */
function originalTitle(voice: Voice): string {
  const source = voice.source_title?.trim();
  const title = voice.title ?? "";
  if (!source) return title;
  if (!title) return source;

  // So sau khi gộp khoảng trắng: tiêu đề Voice là bản bài post đã ép về MỘT
  // dòng rồi mới cắt, nên bài nào có xuống dòng thì so thô sẽ không khớp và
  // người dùng mất phần đuôi.
  const flatten = (value: string) => value.replace(/\s+/g, " ").trim();
  const cut = flatten(title.replace(/…$/, ""));
  return cut && flatten(source).startsWith(cut) ? source : title;
}

/**
 * VoiceInfoTab — phần chữ đi kèm bài đăng trên multime.
 *
 * Hashtag và Quốc gia dùng ĐÚNG hai ô của modal Tạo voice, không phải bản rút
 * gọn: trước đây Hashtag là ô chữ trống trơn (người dùng phải tự nhớ tag nào
 * có thật bên MultiMe, gõ sai thì bài đăng kèm một tag không ai theo dõi), còn
 * Quốc gia thì không có ô nào — nên voice tạo từ kênh, hoặc voice đổi ý về
 * nước, không có đường nào sửa ngoài việc xoá đi tạo lại.
 */
function VoiceInfoTab({ voice, onClose }: { voice: Voice; onClose: () => void }) {
  const update = useUpdateVoice();
  // Cùng nguồn danh mục với modal Tạo voice, và cũng đã cache — mở tab này
  // không tốn thêm request.
  const catalog = useCatalog();
  const [title, setTitle] = React.useState(originalTitle(voice));
  const [hashtags, setHashtags] = React.useState<string[]>(splitHashtags(voice.hashtag));
  const [language, setLanguage] = React.useState(voice.language);
  const [countryId, setCountryId] = React.useState(
    voice.author_country_id ? String(voice.author_country_id) : "",
  );
  const [image, setImage] = React.useState<VoiceImage>({
    url: voice.image_url ?? "",
    uploaded: voice.image_uploaded,
  });
  const [author, setAuthor] = React.useState<AuthorChoice>({
    id: voice.author_id,
    email: voice.author_email,
    gender: voice.author_gender,
  });

  const countryOptions = useCountryCombo(catalog.data?.countries);
  const [hashtagKeyword, setHashtagKeyword] = React.useState("");
  const hashtagOptions = useHashtagCombo(catalog.data?.hashtags, hashtagKeyword);

  /**
   * pickCountry đổi nước VÀ bỏ tài khoản đã bốc trước đó — đúng như ô giới
   * tính làm.
   *
   * Giữ lại thì bài lên multime dưới tên một người thuộc nước khác hẳn nước vừa
   * chọn: backend chỉ bốc lại khi request KHÔNG kèm author_id (xem
   * Voice.UpdateMetadata), mà form này luôn gửi author_id nó đang giữ.
   */
  function pickCountry(next: string) {
    setCountryId(next);
    if (next !== (voice.author_country_id ? String(voice.author_country_id) : "")) {
      setAuthor((cur) => ({ id: null, email: null, gender: cur.gender }));
    }
  }

  // Giống modal Tạo voice: nút Lưu luôn bấm được, lỗi hiện dưới đúng ô sai sau
  // lần bấm đầu tiên.
  const [showErrors, setShowErrors] = React.useState(false);
  const errors = {
    title:
      title.length > MAX_TITLE_LENGTH
        ? `Vượt quá ${MAX_TITLE_LENGTH.toLocaleString("vi-VN")} ký tự — rút ngắn lại.`
        : undefined,
    hashtag: hashtags.length === 0 ? "Bắt buộc — thêm ít nhất 1 hashtag." : undefined,
  };
  const hasError = Object.values(errors).some(Boolean);
  const errorOf = (key: keyof typeof errors) => (showErrors ? errors[key] : undefined);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setShowErrors(true);
    if (hasError) return;
    await update.mutateAsync({
      id: voice.id,
      title: title || null,
      hashtag: hashtags.join(" "),
      language,
      image_url: image.url || null,
      author_id: author.id,
      author_email: author.email,
      author_gender: author.gender,
      // Đổi quốc gia mà không kèm author_id thì backend bỏ tài khoản đã bốc để
      // bước đăng bốc lại theo nước mới (xem updateVoiceRequest).
      author_country_id: countryId ? Number(countryId) : null,
    });
    onClose();
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {/* Tiêu đề là phần chữ DUY NHẤT multime hiển thị — không có ô mô tả.
            Điền sẵn bằng nội dung bài gốc đã cắt về giới hạn ký tự. */}
      <Field label="Tiêu đề" error={errorOf("title")}>
        <Textarea className="min-h-20" value={title} onChange={(e) => setTitle(e.target.value)} />
        <CharCount length={title.length} max={MAX_TITLE_LENGTH} />
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Ngôn ngữ">
          <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
            {languageOptionsFor(language).map((o) => (
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
            onChange={pickCountry}
            emptyLabel="— Tất cả quốc gia —"
            placeholder="— Tất cả quốc gia —"
          />
        </Field>
      </div>

      <Field label="Hashtag" required error={errorOf("hashtag")}>
        <MultiCombobox
          values={hashtags}
          options={hashtagOptions}
          onChange={setHashtags}
          onSearch={setHashtagKeyword}
          placeholder="#tinnong #vietnam"
          allowCreate
          normalize={normalizeHashtag}
        />
      </Field>

      <AuthorField value={author} onChange={setAuthor} />

      <ImageField voiceId={voice.id} value={image} onChange={setImage} />

      <ErrorNote error={update.error} />

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Huỷ
        </Button>
        <Button type="submit" disabled={update.isPending}>
          Lưu
        </Button>
      </div>
    </form>
  );
}

/**
 * AuthorChoice là tài khoản Strongbody đứng tên bài đăng.
 *
 * Người dùng không chọn đích danh ai: họ chọn GIỚI TÍNH, hệ thống bốc ngẫu
 * nhiên một tài khoản khớp. `gender` vì thế là thứ người dùng quyết định, còn
 * `id`/`email` là kết quả bốc ra — cả ba luôn thuộc về cùng một người.
 */
type AuthorChoice = { id: number | null; email: string | null; gender: Gender | null };

const GENDER_LABELS: Record<Gender, string> = {
  male: "Male",
  female: "Female",
  other: "Other",
};

/**
 * AuthorPicker — chọn GIỚI TÍNH của tài khoản đứng tên bài đăng.
 *
 * Chỉ chọn giới tính, KHÔNG bốc tài khoản. Việc bốc lùi xuống lúc hệ thống đăng
 * bài (xem Engine.ensureAuthor ở backend).
 *
 * Vì sao tách: chọn giới tính là thao tác của form, còn bốc là một lần gọi sang
 * Strongbody. Gộp hai thứ khiến mỗi lần đổi ý về giới tính là một lần chờ mạng
 * — và kết quả bốc ra cũng chẳng "giữ chỗ" được gì bên Strongbody trong lúc
 * voice còn nằm trong hàng đợi.
 *
 * Danh bạ Strongbody có hàng nghìn tài khoản mà việc cần làm chỉ là "rải bài
 * đều cho nhiều người", nên chọn đích danh email vừa chậm vừa không giải quyết
 * được gì.
 */
function AuthorPicker({
  value,
  onChange,
  disabled,
  compact,
}: {
  value: AuthorChoice;
  onChange: (next: AuthorChoice) => void;
  disabled?: boolean;
  /** compact: bản gọn cho ô trong bảng. */
  compact?: boolean;
}) {
  const small = compact ? "text-xs" : "text-sm";

  return (
    <div className="space-y-1">
      {/* Giới tính + email hiện NGAY TRONG ô: đó là một giá trị duy nhất, tách
          ra 2 dòng thì mỗi lần đọc phải ghép lại. Email chỉ có khi bài đã đăng
          (lúc đó hệ thống mới bốc xong và ghi lại). */}
      <Select
        className={"truncate " + (compact ? "h-8 w-44 text-xs" : "h-10 w-full")}
        value={value.gender ?? ""}
        disabled={disabled}
        onChange={(e) =>
          // Đổi giới tính là bỏ tài khoản đã bốc trước đó: giữ lại nghĩa là bài
          // lên multime dưới tên một người không khớp giới tính vừa chọn.
          onChange({ id: null, email: null, gender: e.target.value as Gender })
        }
      >
        <option value="" disabled>
          — Giới tính —
        </option>
        {(Object.keys(GENDER_LABELS) as Gender[]).map((gender) => (
          <option key={gender} value={gender}>
            {gender === value.gender && value.email
              ? `${GENDER_LABELS[gender]} - ${value.email}`
              : GENDER_LABELS[gender]}
          </option>
        ))}
      </Select>
    </div>
  );
}

/** AuthorCell — chọn/đổi author ngay tại bảng. */
function AuthorCell({
  voice,
  disabled,
  busy,
}: {
  voice: Voice;
  disabled?: boolean;
  /** busy: chính dòng này đang lưu — do bảng truyền xuống, không lấy từ
      mutation dùng chung (nó pending cho MỌI dòng). */
  busy?: boolean;
}) {
  const update = useUpdateVoice();

  return (
    <AuthorPicker
      compact
      disabled={disabled || busy}
      value={{ id: voice.author_id, email: voice.author_email, gender: voice.author_gender }}
      onChange={(next) =>
        update.mutate({
          id: voice.id,
          author_id: next.id,
          author_email: next.email,
          author_gender: next.gender,
        })
      }
    />
  );
}

/** AuthorField — ô chọn author trong modal, kèm nhãn bắt buộc. */
function AuthorField({
  value,
  onChange,
}: {
  value: AuthorChoice;
  onChange: (next: AuthorChoice) => void;
}) {
  return (
    <Field label="Tài khoản đứng tên bài đăng (author)" required>
      <AuthorPicker value={value} onChange={onChange} />
    </Field>
  );
}

/**
 * ImageField — ảnh bìa: dán URL hoặc tải từ máy.
 *
 * Dùng chung cho modal tạo và modal sửa để hai nơi không lệch nhau về cách
 * hiển thị ảnh đã tải lên (ảnh đó nằm trong bucket riêng tư, phải đi qua API).
 */
function ImageField({
  voiceId,
  value,
  onChange,
  uploadDisabled,
}: {
  voiceId: string;
  value: VoiceImage;
  onChange: (next: VoiceImage) => void;
  /** Chưa tạo xong voice thì API từ chối ghi ảnh — khoá sẵn nút chọn file. */
  uploadDisabled?: boolean;
}) {
  const uploadImage = useUploadVoiceImage();

  async function pickImage(file: File | undefined) {
    if (!file) return;
    const updated = await uploadImage.mutateAsync({ id: voiceId, file });
    onChange({ url: updated.image_url ?? "", uploaded: updated.image_uploaded });
  }

  return (
    <>
      <Field label="Ảnh bìa">
        <div className="space-y-2">
          <Input
            value={value.url}
            onChange={(e) =>
              // Gõ/dán link là chuyển sang ảnh ngoài: ảnh đã tải lên (nếu có)
              // sẽ bị dọn khỏi storage lúc bấm Lưu.
              onChange({ url: e.target.value, uploaded: false })
            }
            placeholder="https://…"
          />
          <div className="flex items-center gap-3">
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              disabled={uploadImage.isPending || uploadDisabled}
              onChange={(e) => {
                void pickImage(e.target.files?.[0]);
                // Xoá giá trị input để chọn lại đúng file vừa rồi vẫn kích hoạt onChange.
                e.target.value = "";
              }}
              className="text-sm text-slate-600 file:mr-3 file:rounded-md file:border-0 file:bg-slate-100 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-slate-700 hover:file:bg-slate-200"
            />
            {uploadImage.isPending ? (
              <span className="text-xs text-slate-500">Đang tải ảnh…</span>
            ) : null}
          </div>
        </div>
      </Field>
      {value.url ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={voiceImageSrc(voiceId, value)} alt="" className="h-24 rounded object-cover" />
      ) : null}
      <ErrorNote error={uploadImage.error} />
    </>
  );
}

/**
 * VoiceContentTab — sửa lời đọc rồi tạo lại chính voice này.
 *
 * Nội dung điền sẵn theo nguồn của voice: text đã gõ (voice nhập tay) hoặc
 * tiêu đề Bài Post (voice lấy từ URL). Sửa ở đây KHÔNG đụng tới bài gốc trên
 * nền tảng — bài đó là dữ liệu của người khác; thứ đổi là lời TTS sẽ đọc.
 */
function VoiceContentTab({ voice, onClose }: { voice: Voice; onClose: () => void }) {
  const regenerate = useRegenerateVoice();
  const prompts = usePrompts();
  const modes = useCollectModes();

  // Hai ô, hai vai trò khác nhau — và trước đây chúng bị gộp làm một, đó là gốc
  // của chuyện "hình thức C hoạt động sai":
  //
  //   source — ĐẦU VÀO của Prompt mẫu. Chỉ hình thức C mới có.
  //   spoken — ĐẦU VÀO của TTS, tức thứ thật sự được đọc. Hình thức C thì đây
  //            là bản LLM viết ra; hình thức B thì nó chính là source.
  //
  // Thứ tự điền sẵn của source = thứ tự "gần với cái đã nghe" nhất:
  //   input_text            — đoạn người dùng tự chốt ở lần sửa trước;
  //   source_extracted_text — đúng đoạn worker đã lấy từ bài gốc;
  //   source_title/title    — voice cũ tạo trước khi lưu extracted_text.
  const initialSource =
    voice.input_text ?? voice.source_extracted_text ?? voice.source_title ?? voice.title ?? "";
  const initialMode = voice.collect_mode ?? voice.source_collect_mode ?? "B";
  const initialSpoken = voice.spoken_text ?? initialSource;

  const [source, setSource] = React.useState(initialSource);
  const [spoken, setSpoken] = React.useState(initialSpoken);
  // Mode A tách audio gốc, không đọc chữ nào -> tạo lại thì mặc định về B.
  const [collectMode, setCollectMode] = React.useState<"B" | "C">(initialMode === "C" ? "C" : "B");
  const [promptId, setPromptId] = React.useState(voice.prompt_id ?? voice.source_prompt_id ?? "");
  const [language, setLanguage] = React.useState(voice.language);
  // Giữ bộ API voice đang dùng; đổi ở đây là đổi cả hạn mức sẽ bị trừ.
  const [llmSetId, setLlmSetId] = React.useState(voice.llm_api_set_id ?? "");
  // Mở ra là thấy ĐÚNG giọng voice đang dùng, không phải mặc định: bấm Tạo lại
  // mà giọng tự đổi thì người dùng mất bản họ đã ưng mà không biết vì sao.
  const [voiceStyle, setVoiceStyle] = React.useState<VoiceStyle>(voice.tts_config ?? {});

  const needsPrompt = collectMode === "C";
  const modeMeta = modes.data?.collect_modes ?? [];
  const metaOf = (mode: string) => modeMeta.find((m) => m.mode === mode);
  const isEnabled = (mode: string) => metaOf(mode)?.enabled ?? mode === "A";

  // Sửa ô nào thì ô đó quyết định chuyện gì xảy ra khi bấm Tạo lại:
  //
  //   sửa Nội dung đọc -> đọc ĐÚNG chữ đó, không gọi LLM. Chạy prompt lên một
  //                       bản đã viết lại là ghi đè đúng thứ vừa sửa.
  //   sửa Nội dung     -> chạy prompt lại từ đầu, lời đọc cũ bị thay.
  //   không sửa gì     -> chạy lại như cũ (đổi prompt / đổi Bộ API rồi bấm).
  //
  // Suy ra từ thao tác thay vì bắt chọn thêm một ô radio, nhưng nói thẳng kết
  // quả ra màn hình bên dưới để không ai phải đoán.
  const sourceEdited = source !== initialSource;
  const spokenEdited = spoken !== initialSpoken;
  const keepSpoken = needsPrompt && spokenEdited && !sourceEdited;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await regenerate.mutateAsync({
      id: voice.id,
      // Hình thức B không có nguồn tách rời: ô lời đọc là tất cả.
      text: (needsPrompt ? source : spoken).trim(),
      spoken_text: keepSpoken ? spoken.trim() : null,
      collect_mode: collectMode,
      prompt_id: needsPrompt && promptId ? promptId : null,
      llm_api_set_id: needsPrompt && llmSetId ? llmSetId : null,
      language,
      // Luôn gửi: form này đã hiện đúng giọng đang dùng, nên thứ đang thấy
      // chính là thứ phải chạy — kể cả khi họ vừa bấm "Về mặc định" (gửi `{}`,
      // backend hiểu là xoá cấu hình).
      tts_config: voiceStyle,
    });
    onClose();
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {needsPrompt ? (
        <Field
          label={
            voice.source_post_id
              ? `Nội dung — lấy từ bài ${platformLabel(voice.platform)}`
              : "Nội dung"
          }
          required
          hint="Đầu vào của Prompt mẫu, KHÔNG phải thứ được đọc. Sửa ở đây rồi bấm Tạo lại là chạy prompt lần nữa."
        >
          <Textarea
            className="min-h-32"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            required
          />
          <CharCount length={source.length} max={MAX_TTS_TEXT_LENGTH} />
        </Field>
      ) : null}

      <Field
        label={
          needsPrompt
            ? "Nội dung đọc — bản LLM viết lại"
            : voice.source_post_id
              ? `Nội dung đọc — lấy từ bài ${platformLabel(voice.platform)}`
              : "Nội dung đọc"
        }
        required
        hint={
          needsPrompt
            ? `Đúng đoạn TTS đã đọc ra file hiện tại${voice.llm_model_used ? ` (${voice.llm_model_used})` : ""}. Sửa tay ở đây thì lần tạo lại đọc nguyên văn, không gọi LLM nữa.`
            : "TTS đọc đúng những gì có ở đây."
        }
      >
        <Textarea
          className="min-h-48"
          value={spoken}
          onChange={(e) => setSpoken(e.target.value)}
          required
        />
        <CharCount length={spoken.length} max={MAX_TTS_TEXT_LENGTH} />
      </Field>

      {/* Nói thẳng cái sắp xảy ra: hai ô trên dẫn tới hai hành vi khác nhau, và
          không ai đoán được điều đó chỉ bằng cách nhìn form. */}
      {needsPrompt ? (
        <p className="rounded-md bg-slate-50 px-3 py-2 text-xs text-slate-600">
          Bấm <b>Tạo lại voice</b> sẽ{" "}
          {keepSpoken ? (
            <>
              đọc đúng phần <b>Nội dung đọc</b> bạn vừa sửa, không chạy lại LLM.
            </>
          ) : (
            <>
              chạy Prompt mẫu trên phần <b>Nội dung</b> rồi đọc bản LLM viết ra — phần Nội dung đọc
              hiện tại sẽ bị thay.
            </>
          )}
        </p>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Hình thức" required>
          <Select value={collectMode} onChange={(e) => setCollectMode(e.target.value as "B" | "C")}>
            {(["B", "C"] as const).map((value) => (
              <option key={value} value={value} disabled={!isEnabled(value)}>
                {COLLECT_MODE_LABELS[value]}
                {isEnabled(value) ? "" : " — chưa dùng được"}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Ngôn ngữ">
          <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
            {languageOptionsFor(language).map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <VoiceStyleFields
        value={voiceStyle}
        onChange={setVoiceStyle}
        authorGender={voice.author_gender}
      />

      {/* Mode bị tắt thì nói rõ thiếu gì, không để người dùng đoán. */}
      {!isEnabled(collectMode) && metaOf(collectMode)?.reason ? (
        <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Hình thức {collectMode} {metaOf(collectMode)?.reason}
        </p>
      ) : null}

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
          <LLMSetField
            value={llmSetId}
            onChange={setLlmSetId}
            hint={voice.llm_model_used ? `Lần trước chạy bằng ${voice.llm_model_used}.` : undefined}
          />
        </div>
      ) : null}

      <ErrorNote error={regenerate.error} />

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Huỷ
        </Button>
        <Button
          type="submit"
          disabled={
            regenerate.isPending ||
            !isEnabled(collectMode) ||
            source.length > MAX_TTS_TEXT_LENGTH ||
            spoken.length > MAX_TTS_TEXT_LENGTH
          }
        >
          {regenerate.isPending ? "Đang xử lý…" : "Tạo lại voice"}
        </Button>
      </div>
    </form>
  );
}
