"use client";

import * as React from "react";

import { AudioPreview } from "@/components/audio-preview";
import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { DuplicatePostNotice } from "@/components/duplicate-notice";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, RowActions, SortButton, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  duplicateOf,
  useCollectModes,
  useCreateSourcePost,
  useCreateTextVoice,
  useDeleteVoice,
  usePlatforms,
  usePrompts,
  usePublishVoice,
  usePublishRequirements,
  useRandomAuthor,
  useRegenerateVoice,
  useSourcePost,
  useUpdateVoice,
  useUploadVoiceImage,
  useVoiceOfSourcePost,
  useVoices,
} from "@/hooks/use-api";
import { coverImageUrl } from "@/lib/api";
import { LANGUAGE_OPTIONS, compactLanguageOptions, languageOptionsFor } from "@/lib/languages";
import { COLLECT_MODE_LABELS, PUBLISH_STATUS_LABELS, platformLabel } from "@/lib/utils";
import type { CollectMode, DuplicatePost, Gender, PublishRequirements, Voice } from "@/types/api";

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
  language: "",
  created_from: "",
  created_to: "",
  published_from: "",
  published_to: "",
};

export default function VoicesPage() {
  const [filters, setFilters] = React.useState(EMPTY_FILTERS);
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

  const voices = useVoices({
    publish_status: filters.publish_status || undefined,
    platform: filters.platform || undefined,
    language: filters.language || undefined,
    created_from: filters.created_from || undefined,
    created_to: filters.created_to || undefined,
    published_from: filters.published_from || undefined,
    published_to: filters.published_to || undefined,
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });
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

          <div className="w-48">
            <label className="mb-1 block text-xs font-medium text-slate-500">Ngôn ngữ</label>
            <Select value={filters.language} onChange={(e) => set("language")(e.target.value)}>
              <option value="">Tất cả</option>
              {LANGUAGE_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
          </div>

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
                <Th>Ngôn ngữ</Th>
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
                <EmptyRow colSpan={9}>Đang tải…</EmptyRow>
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
                    voice.last_error ??
                    (blockers.length > 0
                      ? `Chưa đủ điều kiện đăng lên multime: ${blockers.join(", ")}.`
                      : null);
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

                      <Td>
                        <Can permission="can_write" fallback={<span>{voice.language}</span>}>
                          <Select
                            className="h-8 w-28 text-xs"
                            value={voice.language}
                            disabled={
                              processing || voice.publish_status === "published" || updating
                            }
                            onChange={(e) =>
                              update.mutate({ id: voice.id, language: e.target.value })
                            }
                          >
                            {compactLanguageOptions(voice.language).map((o) => (
                              <option key={o.value} value={o.value}>
                                {o.label}
                              </option>
                            ))}
                          </Select>
                        </Can>
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
                                  onClick={() => publish.mutate(voice.id)}
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
                <EmptyRow colSpan={9}>Không có Voice khớp bộ lọc.</EmptyRow>
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
 * CreateVoiceDialog — tạo Voice ngay từ màn Voice (luồng F1).
 *
 * Vẫn đi qua Bài Post như mọi luồng khác (business rule #1): tab "Thông tin"
 * tạo Bài Post rồi bật `auto_process` để worker sinh Voice luôn, chứ không có
 * đường tắt tạo Voice trực tiếp.
 *
 * Hai kiểu nhập liệu loại trừ nhau và đi 2 đường khác nhau, giống hệt màn F1:
 *   - Thông tin: dán URL -> tạo Bài Post -> Voice, rồi sửa metadata và đăng
 *                ngay trong hộp thoại này.
 *   - Nhập text: tạo thẳng Voice, không sinh Bài Post — text gõ tay không có
 *                bài gốc nào để truy vết.
 */
type VoiceInputMode = "url" | "text";

function CreateVoiceDialog({ onClose }: { onClose: () => void }) {
  const [inputMode, setInputMode] = React.useState<VoiceInputMode>("url");
  // Đã dán URL và tạo Bài Post thì không cho đổi tab nữa: voice đang được tạo
  // ở tab này, nhảy sang tab kia là bỏ lại nó giữa chừng mà không thấy đâu.
  const [locked, setLocked] = React.useState(false);

  return (
    <Modal
      title="Tạo Voice"
      description="Dán URL bài đăng để hệ thống tự lấy nội dung, hoặc gõ thẳng text cho TTS đọc."
      width="3xl"
      onClose={onClose}
    >
      {/* 2 kiểu nhập liệu loại trừ nhau nên dùng nút chọn thay vì 2 ô cùng
          hiện — đỡ phải đoán ô nào thắng khi điền cả hai. */}
      <div className="mb-4 grid grid-cols-2 gap-2 rounded-lg bg-slate-100 p-1">
        {(
          [
            ["url", "Thông tin"],
            ["text", "Nhập text"],
          ] as [VoiceInputMode, string][]
        ).map(([value, label]) => (
          <button
            key={value}
            type="button"
            disabled={locked && inputMode !== value}
            title={
              locked && inputMode !== value
                ? "Đang tạo voice từ URL — đóng hộp thoại nếu muốn làm việc khác"
                : undefined
            }
            onClick={() => setInputMode(value)}
            className={
              "rounded-md px-3 py-1.5 text-sm font-medium transition " +
              (inputMode === value
                ? "bg-white text-slate-900 shadow-sm"
                : "text-slate-600 hover:text-slate-900 disabled:text-slate-300 disabled:hover:text-slate-300")
            }
          >
            {label}
          </button>
        ))}
      </div>

      {inputMode === "url" ? (
        <FromURLTab onClose={onClose} onCreated={() => setLocked(true)} />
      ) : (
        <FromTextTab onClose={onClose} />
      )}
    </Modal>
  );
}

/** enabledMode gom việc đọc /meta/collect-modes cho cả 2 tab. */
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

/**
 * FromURLTab — dán URL, hệ thống tạo bài post + audio, rồi sửa metadata và
 * đăng luôn tại chỗ.
 *
 * Voice được tạo NGAY khi bấm Enter chứ không đợi bấm "Đăng voice": đóng hộp
 * thoại giữa chừng thì voice vẫn nằm trong bảng như mọi voice khác (nháp/chưa
 * đủ điều kiện), không mất công đã chờ.
 */
function FromURLTab({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const create = useCreateSourcePost();
  const prompts = usePrompts();
  const gate = useModeGate();

  const [collectMode, setCollectMode] = React.useState<CollectMode>("A");
  const [promptId, setPromptId] = React.useState("");
  const [sourceUrl, setSourceUrl] = React.useState("");
  const [duplicate, setDuplicate] = React.useState<DuplicatePost | null>(null);
  const [postId, setPostId] = React.useState<string | null>(null);

  const needsPrompt = collectMode === "C";

  async function send(allowDuplicate: boolean) {
    try {
      const post = await create.mutateAsync({
        source_url: sourceUrl.trim(),
        collect_mode: collectMode,
        prompt_id: needsPrompt && promptId ? promptId : null,
        auto_process: true,
        allow_duplicate: allowDuplicate || undefined,
      });
      setDuplicate(null);
      setPostId(post.id);
      onCreated();
    } catch (err) {
      const dup = duplicateOf(err);
      if (!dup) throw err;
      setDuplicate(dup);
    }
  }

  // Đã tạo bài post -> phần còn lại là metadata của chính voice đó.
  if (postId) {
    return (
      <NewVoicePanel
        sourcePostId={postId}
        collectMode={collectMode}
        sourceUrl={sourceUrl.trim()}
        onClose={onClose}
      />
    );
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void send(false);
      }}
      className="space-y-4"
    >
      <Field label="Hình thức tạo" required>
        <Select
          value={collectMode}
          onChange={(e) => setCollectMode(e.target.value as CollectMode)}
          required
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
      ) : null}

      <Field label="URL bài đăng" required>
        <Input
          type="url"
          placeholder="https://www.youtube.com/watch?v=..."
          value={sourceUrl}
          onChange={(e) => setSourceUrl(e.target.value)}
          required
          autoFocus
        />
      </Field>

      {duplicate ? (
        <DuplicatePostNotice
          existing={duplicate}
          pending={create.isPending}
          onSkip={onClose}
          onCreateAnyway={() => void send(true)}
        />
      ) : (
        <ErrorNote error={create.error} />
      )}

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Huỷ
        </Button>
        <Button type="submit" disabled={create.isPending || duplicate !== null}>
          {create.isPending ? "Đang xử lý…" : "Tạo Voice"}
        </Button>
      </div>
    </form>
  );
}

/**
 * NewVoicePanel — metadata của voice vừa tạo, nghe thử và đăng ngay.
 *
 * Metadata và audio về ở hai nhịp khác hẳn nhau: task `post:metadata` chỉ đọc
 * thông tin bài (vài giây), còn audio phải tải/đọc cả video (có khi hàng phút).
 * Nên form điền sẵn từ BÀI POST và hiện ngay, chỗ nghe thử tự quay vòng riêng
 * — bắt người dùng ngồi nhìn màn trống tới khi có audio là phí mất quãng họ
 * có thể sửa tiêu đề, chọn author.
 */
function NewVoicePanel({
  sourcePostId,
  collectMode,
  sourceUrl,
  onClose,
}: {
  sourcePostId: string;
  collectMode: CollectMode;
  sourceUrl: string;
  onClose: () => void;
}) {
  const post = useSourcePost(sourcePostId);
  const created = useVoiceOfSourcePost(sourcePostId);
  const update = useUpdateVoice();
  const publish = usePublishVoice();
  const publishReq = usePublishRequirements().data;

  const voice = created.data?.items[0];
  // Voice còn `processing` thì chưa sửa/đăng được (API chặn) — mọi thao tác ghi
  // phải đợi đúng mốc này, dù phần chữ đã hiện từ trước.
  const ready = voice != null && voice.publish_status !== "processing";

  const [form, setForm] = React.useState<{
    title: string;
    hashtag: string;
    language: string;
    image: VoiceImage;
    author: AuthorChoice;
  } | null>(null);

  // Điền form ngay khi Bài Post có metadata. Voice xong sau đó chỉ bù vào ô còn
  // trống — không đè lên thứ người dùng đang gõ dở.
  React.useEffect(() => {
    const meta = post.data;
    if (!meta?.title && !ready) return;

    setForm((current) => {
      if (current) {
        if (!voice || !ready) return current;
        return {
          ...current,
          title: current.title || (voice.title ?? ""),
          hashtag: current.hashtag || (voice.hashtag ?? ""),
          image: current.image.url ? current.image : { url: voice.image_url ?? "", uploaded: voice.image_uploaded },
        };
      }
      return {
        title: meta?.title ?? voice?.title ?? "",
        hashtag: (meta?.hashtags ?? []).join(" ") || (voice?.hashtag ?? ""),
        language: voice?.language ?? meta?.language ?? "auto",
        image: { url: meta?.thumbnail_url ?? voice?.image_url ?? "", uploaded: false },
        author: {
          id: voice?.author_id ?? null,
          email: voice?.author_email ?? null,
          gender: voice?.author_gender ?? null,
        },
      };
    });
  }, [post.data, voice, ready]);

  async function publishNow() {
    if (!voice || !form) return;
    await update.mutateAsync({
      id: voice.id,
      title: form.title || null,
      hashtag: form.hashtag,
      language: form.language,
      image_url: form.image.url || null,
      author_id: form.author.id,
      author_email: form.author.email,
      author_gender: form.author.gender,
    });
    await publish.mutateAsync(voice.id);
    onClose();
  }

  // Điều kiện multime đòi, tính trên giá trị người dùng ĐANG sửa chứ không phải
  // bản đã lưu — nút không được sáng lên rồi mới báo lỗi.
  const blockers =
    voice && form
      ? multimeBlockers(
          {
            ...voice,
            title: form.title,
            hashtag: form.hashtag,
            author_id: form.author.id,
          },
          publishReq,
        )
      : [];
  const posting = update.isPending || publish.isPending;
  // Tiêu đề dài quá thì API từ chối — chặn ngay ở nút thay vì để người dùng
  // bấm rồi mới nhận lỗi.
  const tooLong = (form?.title.length ?? 0) > MAX_TITLE_LENGTH;

  return (
    <div className="space-y-4">
      <div className="rounded-md bg-slate-50 px-3 py-2 text-sm">
        <p className="text-slate-600">
          Hình thức tạo: <span className="text-slate-900">{COLLECT_MODE_LABELS[collectMode]}</span>
        </p>
        <p className="mt-0.5 break-all text-slate-600">
          URL:{" "}
          <a
            href={sourceUrl}
            target="_blank"
            rel="noreferrer"
            className="text-indigo-700 hover:underline"
          >
            {sourceUrl}
          </a>
        </p>
      </div>

      {/* Chỗ nghe thử: chờ riêng, không chặn phần chữ bên dưới. */}
      {voice?.voice_file_url ? (
        <AudioPreview voiceId={voice.id} />
      ) : (
        <p className="flex items-center gap-2 rounded-md bg-sky-50 px-3 py-2 text-sm text-sky-800">
          <span className="size-3 animate-spin rounded-full border-2 border-sky-300 border-t-sky-700" />
          Đang tạo audio…
        </p>
      )}

      {!form ? (
        <p className="rounded-md bg-slate-50 px-3 py-3 text-sm text-slate-600">
          Đang lấy nội dung bài…
        </p>
      ) : (
        <>
          <Field label="Tiêu đề">
            <Textarea
              className="min-h-20"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
            />
            <CharCount length={form.title.length} max={MAX_TITLE_LENGTH} />
          </Field>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Hashtag" required>
              <Input
                value={form.hashtag}
                onChange={(e) => setForm({ ...form, hashtag: e.target.value })}
                placeholder="#tinnong #vietnam"
                required
              />
            </Field>

            <Field label="Ngôn ngữ">
              <Select
                value={form.language}
                onChange={(e) => setForm({ ...form, language: e.target.value })}
              >
                {languageOptionsFor(form.language).map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
            </Field>
          </div>

          <AuthorField value={form.author} onChange={(author) => setForm({ ...form, author })} />

          {voice ? (
            <ImageField
              voiceId={voice.id}
              value={form.image}
              onChange={(image) => setForm({ ...form, image })}
              // Voice chưa xử lý xong thì API từ chối ghi ảnh — chặn ở đây để
              // người dùng không bấm rồi nhận lỗi khó hiểu.
              uploadDisabled={!ready}
            />
          ) : null}
        </>
      )}

      <ErrorNote error={post.error ?? created.error ?? update.error ?? publish.error} />

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Đóng
        </Button>
        <Button
          type="button"
          disabled={!ready || !form || blockers.length > 0 || tooLong || posting}
          title={blockers.length > 0 ? `multime.ai yêu cầu: ${blockers.join(", ")}` : undefined}
          onClick={() => void publishNow()}
        >
          {posting ? "Đang đăng…" : "Đăng voice"}
        </Button>
      </div>
    </div>
  );
}

/** FromTextTab — gõ thẳng text cho TTS đọc, không sinh Bài Post. */
function FromTextTab({ onClose }: { onClose: () => void }) {
  const createVoice = useCreateTextVoice();
  const prompts = usePrompts();
  const gate = useModeGate();

  const [text, setText] = React.useState("");
  const [collectMode, setCollectMode] = React.useState<"B" | "C">("B");
  const [promptId, setPromptId] = React.useState("");

  const needsPrompt = collectMode === "C";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await createVoice.mutateAsync({
      text: text.trim(),
      collect_mode: collectMode,
      prompt_id: needsPrompt && promptId ? promptId : null,
    });
    onClose();
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {/* Nhập text thì không có audio gốc -> hình thức A không dùng được. */}
      <Field label="Hình thức tạo" required>
        <Select
          value={collectMode}
          onChange={(e) => setCollectMode(e.target.value as "B" | "C")}
          required
        >
          {(["B", "C"] as const).map((value) => (
            <option key={value} value={value} disabled={!gate.isEnabled(value)}>
              {COLLECT_MODE_LABELS[value]}
              {gate.note(value)}
            </option>
          ))}
        </Select>
      </Field>

      {needsPrompt ? (
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
      ) : null}

      <Field label="Nội dung TTS đọc" required>
        <Textarea
          className="min-h-40"
          placeholder="Bản tin sáng nay…"
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
          autoFocus
        />
        <CharCount length={text.length} max={MAX_TTS_TEXT_LENGTH} />
      </Field>

      <ErrorNote error={createVoice.error} />

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Huỷ
        </Button>
        <Button
          type="submit"
          disabled={
            createVoice.isPending ||
            !gate.isEnabled(collectMode) ||
            text.length > MAX_TTS_TEXT_LENGTH
          }
        >
          {createVoice.isPending ? "Đang xử lý…" : "Tạo Voice"}
        </Button>
      </div>
    </form>
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

/** VoiceInfoTab — phần chữ đi kèm bài đăng trên multime. */
function VoiceInfoTab({ voice, onClose }: { voice: Voice; onClose: () => void }) {
  const update = useUpdateVoice();
  const [title, setTitle] = React.useState(originalTitle(voice));
  const [hashtag, setHashtag] = React.useState(voice.hashtag ?? "");
  const [language, setLanguage] = React.useState(voice.language);
  const [image, setImage] = React.useState<VoiceImage>({
    url: voice.image_url ?? "",
    uploaded: voice.image_uploaded,
  });
  const [author, setAuthor] = React.useState<AuthorChoice>({
    id: voice.author_id,
    email: voice.author_email,
    gender: voice.author_gender,
  });

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await update.mutateAsync({
      id: voice.id,
      title: title || null,
      hashtag,
      language,
      image_url: image.url || null,
      author_id: author.id,
      author_email: author.email,
      author_gender: author.gender,
    });
    onClose();
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {/* Tiêu đề là phần chữ DUY NHẤT multime hiển thị — không có ô mô tả.
            Điền sẵn bằng nội dung bài gốc đã cắt về giới hạn ký tự. */}
      <Field label="Tiêu đề">
        <Textarea className="min-h-20" value={title} onChange={(e) => setTitle(e.target.value)} />
        <CharCount length={title.length} max={MAX_TITLE_LENGTH} />
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Hashtag" required>
          <Input
            value={hashtag}
            onChange={(e) => setHashtag(e.target.value)}
            placeholder="#tinnong #vietnam"
            required
          />
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

      <AuthorField value={author} onChange={setAuthor} />

      <ImageField voiceId={voice.id} value={image} onChange={setImage} />

      <ErrorNote error={update.error} />

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Huỷ
        </Button>
        <Button type="submit" disabled={update.isPending || title.length > MAX_TITLE_LENGTH}>
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
 * AuthorPicker — chọn tài khoản đứng tên bài đăng bằng giới tính.
 *
 * Chọn một giới tính là bốc ngay một tài khoản khớp; nút random bốc lại người
 * khác trong cùng giới tính. Danh bạ Strongbody có hàng nghìn tài khoản mà
 * việc cần làm chỉ là "rải bài đều cho nhiều người", nên chọn đích danh email
 * vừa chậm vừa không giải quyết được gì.
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
  const random = useRandomAuthor();

  async function pick(gender: Gender) {
    const { author } = await random.mutateAsync(gender);
    onChange({ id: author.id, email: author.email, gender: author.gender });
  }

  const busy = random.isPending;
  const small = compact ? "text-xs" : "text-sm";

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        {/* Giới tính + email hiện NGAY TRONG ô: đó là một giá trị duy nhất, tách
            ra 2 dòng thì mỗi lần đọc phải ghép lại. Ô giữ nguyên bề ngang, chuỗi
            dài thì trình duyệt tự cắt — bảng không được co giãn theo độ dài
            email của từng dòng. */}
        <Select
          className={"truncate " + (compact ? "h-8 w-44 text-xs" : "h-10 w-full")}
          value={value.gender ?? ""}
          disabled={disabled || busy}
          onChange={(e) => void pick(e.target.value as Gender)}
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

        {/* Random lại chỉ có nghĩa khi đã chọn giới tính — bốc lại trong đúng
            giới tính đó, không đổi lựa chọn của người dùng. */}
        <button
          type="button"
          disabled={disabled || busy || !value.gender}
          onClick={() => value.gender && void pick(value.gender)}
          title="Bốc lại một tài khoản khác cùng giới tính"
          className={
            "shrink-0 rounded-md border border-slate-300 px-2 py-1 font-medium text-slate-600 " +
            "hover:border-slate-400 hover:text-slate-900 disabled:text-slate-300 " +
            small
          }
        >
          ⟳
        </button>

        {busy ? <span className={small + " shrink-0 text-slate-500"}>Đang bốc…</span> : null}
      </div>

      {random.error ? <p className={small + " text-red-700"}>{random.error.message}</p> : null}
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

  // Thứ tự điền sẵn = thứ tự "gần với cái đã nghe" nhất:
  //   input_text            — lời đọc người dùng tự chốt ở lần sửa trước;
  //   source_extracted_text — đúng đoạn worker đã đưa cho TTS;
  //   source_title/title    — voice cũ tạo trước khi lưu extracted_text.
  const initialText =
    voice.input_text ?? voice.source_extracted_text ?? voice.source_title ?? voice.title ?? "";
  const initialMode = voice.collect_mode ?? voice.source_collect_mode ?? "B";

  const [text, setText] = React.useState(initialText);
  // Mode A tách audio gốc, không đọc chữ nào -> tạo lại thì mặc định về B.
  const [collectMode, setCollectMode] = React.useState<"B" | "C">(initialMode === "C" ? "C" : "B");
  const [promptId, setPromptId] = React.useState(voice.prompt_id ?? voice.source_prompt_id ?? "");
  const [language, setLanguage] = React.useState(voice.language);

  const needsPrompt = collectMode === "C";
  const modeMeta = modes.data?.collect_modes ?? [];
  const metaOf = (mode: string) => modeMeta.find((m) => m.mode === mode);
  const isEnabled = (mode: string) => metaOf(mode)?.enabled ?? mode === "A";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await regenerate.mutateAsync({
      id: voice.id,
      text: text.trim(),
      collect_mode: collectMode,
      prompt_id: needsPrompt && promptId ? promptId : null,
      language,
    });
    onClose();
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <Field
        label={
          voice.source_post_id
            ? `Nội dung đọc — lấy từ nội dung bài ${platformLabel(voice.platform)}`
            : "Nội dung đọc"
        }
        required
      >
        <Textarea
          className="min-h-48"
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
        />
        <CharCount length={text.length} max={MAX_TTS_TEXT_LENGTH} />
      </Field>

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

      {/* Mode bị tắt thì nói rõ thiếu gì, không để người dùng đoán. */}
      {!isEnabled(collectMode) && metaOf(collectMode)?.reason ? (
        <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Hình thức {collectMode} {metaOf(collectMode)?.reason}
        </p>
      ) : null}

      {needsPrompt ? (
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
      ) : null}

      <ErrorNote error={regenerate.error} />

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onClose}>
          Huỷ
        </Button>
        <Button
          type="submit"
          disabled={
            regenerate.isPending || !isEnabled(collectMode) || text.length > MAX_TTS_TEXT_LENGTH
          }
        >
          {regenerate.isPending ? "Đang xử lý…" : "Tạo lại voice"}
        </Button>
      </div>
    </form>
  );
}
