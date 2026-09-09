"use client";

import * as React from "react";

import { AudioPreview } from "@/components/audio-preview";
import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can, ViewerNotice } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Input, Select, Textarea } from "@/components/ui/field";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import {
  useDeleteVoice,
  usePlatforms,
  usePublishVoice,
  usePublishRequirements,
  useUpdateVoice,
  useVoices,
} from "@/hooks/use-api";
import {
  LANGUAGE_OPTIONS,
  PUBLISH_STATUS_LABELS,
  formatDateTime,
  platformLabel,
} from "@/lib/utils";
import type { PublishRequirements, Voice } from "@/types/api";

/**
 * multimeBlockers trả về các lý do multime.ai sẽ từ chối bài đăng.
 * Hợp đồng API (/v1/seller/voice-posts/upload): bắt buộc title, ít nhất 1
 * hashtag (hoặc category), và audio dài tối thiểu 15 giây.
 *
 * `req` là cấu hình phía server: có `MULTIME_DEFAULT_HASHTAGS` thì voice không
 * có hashtag riêng vẫn đăng được, nên không chặn.
 */
function multimeBlockers(voice: Voice, req?: PublishRequirements): string[] {
  const reasons: string[] = [];
  const hasFallbackTag = (req?.default_hashtags?.length ?? 0) > 0 || (req?.category_ids?.length ?? 0) > 0;
  const minDuration = req?.min_duration_seconds ?? 15;

  if (!voice.title?.trim() && !voice.description?.trim()) reasons.push("chưa có tiêu đề");
  if (!voice.hashtag?.trim() && !hasFallbackTag) reasons.push("chưa có hashtag");
  if (voice.duration_seconds != null && voice.duration_seconds < minDuration) {
    reasons.push(`chỉ dài ${voice.duration_seconds}s (tối thiểu ${minDuration}s)`);
  }
  return reasons;
}

export default function VoicesPage() {
  const [filters, setFilters] = React.useState({
    publish_status: "",
    platform: "",
    language: "",
    created_from: "",
    created_to: "",
    published_from: "",
    published_to: "",
  });
  const [editing, setEditing] = React.useState<Voice | null>(null);

  const set = (key: keyof typeof filters) => (value: string) =>
    setFilters((f) => ({ ...f, [key]: value }));

  const voices = useVoices({
    publish_status: filters.publish_status || undefined,
    platform: filters.platform || undefined,
    language: filters.language || undefined,
    created_from: filters.created_from || undefined,
    created_to: filters.created_to || undefined,
    published_from: filters.published_from || undefined,
    published_to: filters.published_to || undefined,
    limit: 50,
  });
  const platforms = usePlatforms();
  const publishReq = usePublishRequirements().data;
  const publish = usePublishVoice();
  const update = useUpdateVoice();
  const remove = useDeleteVoice();
  const selection = useSelection(voices.data?.items);

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
      />
      <ViewerNotice />

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

          <div className="w-40">
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
          <Button
            variant="ghost"
            onClick={() =>
              setFilters({
                publish_status: "",
                platform: "",
                language: "",
                created_from: "",
                created_to: "",
                published_from: "",
                published_to: "",
              })
            }
          >
            Xoá lọc
          </Button>
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
                      throw new Error(`${voice.title ?? voice.id.slice(0, 8)}: thiếu hashtag/tiêu đề`);
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
                <Th>Nội dung / nghe thử</Th>
                <Th>Nền tảng</Th>
                <Th>Ngôn ngữ</Th>
                <Th>Người tạo</Th>
                <Th>Trạng thái</Th>
                <Th>Tạo lúc</Th>
                <Th>Đăng lúc</Th>
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
                  const blockers = multimeBlockers(voice, publishReq);
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

                      <Td className="max-w-md">
                        <div className="flex gap-3">
                          {voice.image_url ? (
                            // eslint-disable-next-line @next/next/no-img-element
                            <img
                              src={voice.image_url}
                              alt=""
                              className="h-12 w-20 shrink-0 rounded object-cover"
                            />
                          ) : null}
                          <div className="min-w-0">
                            {voice.title ? (
                              <p className="truncate font-medium text-slate-900" title={voice.title}>
                                {voice.title}
                              </p>
                            ) : null}
                            <p
                              className="line-clamp-3 text-slate-800"
                              title={voice.description ?? undefined}
                            >
                              {voice.description ?? (
                                <span className="text-slate-400">(chưa có mô tả)</span>
                              )}
                            </p>
                            {voice.hashtag ? (
                              <p className="mt-0.5 text-xs text-indigo-700">{voice.hashtag}</p>
                            ) : null}
                          </div>
                        </div>

                        {voice.voice_file_url ? (
                          <AudioPreview voiceId={voice.id} />
                        ) : voice.multime_post_url ? (
                          <a
                            href={voice.multime_post_url}
                            target="_blank"
                            rel="noreferrer"
                            className="mt-1 inline-block text-xs text-indigo-700 hover:underline"
                          >
                            Nghe trên multime.ai ↗
                          </a>
                        ) : null}

                        {voice.last_error ? (
                          <p className="mt-1 text-xs text-red-700">{voice.last_error}</p>
                        ) : null}
                        {voice.publish_status !== "published" && blockers.length > 0 ? (
                          <p className="mt-1 text-xs text-amber-800">
                            Chưa đăng được lên multime: {blockers.join(", ")}.
                          </p>
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
                        ) : (
                          platformLabel(voice.platform)
                        )}
                      </Td>

                      <Td>
                        <Can
                          permission="can_write"
                          fallback={<span>{voice.language}</span>}
                        >
                          <Select
                            className="h-8 w-36 text-xs"
                            value={voice.language}
                            disabled={voice.publish_status === "published" || update.isPending}
                            onChange={(e) =>
                              update.mutate({ id: voice.id, language: e.target.value })
                            }
                          >
                            {LANGUAGE_OPTIONS.some((o) => o.value === voice.language) ? null : (
                              <option value={voice.language}>{voice.language}</option>
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
                        {voice.created_by_email ?? "—"}
                      </Td>

                      <Td>
                        <Badge tone={statusTone(voice.publish_status)}>
                          {PUBLISH_STATUS_LABELS[voice.publish_status] ?? voice.publish_status}
                        </Badge>
                      </Td>
                      <Td className="whitespace-nowrap">{formatDateTime(voice.created_at)}</Td>
                      <Td className="whitespace-nowrap">{formatDateTime(voice.published_at)}</Td>

                      <Can permission="can_write">
                        <Td className="whitespace-nowrap text-right">
                          {voice.publish_status !== "published" ? (
                            <>
                              <Button
                                size="sm"
                                variant="secondary"
                                title="Sửa metadata"
                                aria-label="Sửa metadata"
                                onClick={() => setEditing(voice)}
                              >
                                ✏️
                              </Button>{" "}
                              <Button
                                size="sm"
                                disabled={
                                  !voice.voice_file_url || publish.isPending || blockers.length > 0
                                }
                                title={
                                  blockers.length > 0
                                    ? `multime.ai yêu cầu: ${blockers.join(", ")}`
                                    : undefined
                                }
                                onClick={() => publish.mutate(voice.id)}
                              >
                                Đăng
                              </Button>{" "}
                            </>
                          ) : null}
                          <Can permission="can_delete">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => {
                                if (confirm("Xoá voice này?")) remove.mutate(voice.id);
                              }}
                            >
                              Xoá
                            </Button>
                          </Can>
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
        </CardBody>
      </Card>

      {editing ? <EditVoiceDialog voice={editing} onClose={() => setEditing(null)} /> : null}
    </>
  );
}

/**
 * EditVoiceDialog — form đăng bài, đã auto-fill sẵn từ metadata bài gốc
 * (tiêu đề, mô tả, hashtag, ảnh bìa) do worker lấy về khi tạo Voice.
 */
function EditVoiceDialog({ voice, onClose }: { voice: Voice; onClose: () => void }) {
  const update = useUpdateVoice();
  const [title, setTitle] = React.useState(voice.title ?? "");
  const [description, setDescription] = React.useState(voice.description ?? "");
  const [hashtag, setHashtag] = React.useState(voice.hashtag ?? "");
  const [language, setLanguage] = React.useState(voice.language);
  const [imageUrl, setImageUrl] = React.useState(voice.image_url ?? "");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await update.mutateAsync({
      id: voice.id,
      title: title || null,
      description,
      hashtag,
      language,
      image_url: imageUrl || null,
    });
    onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-slate-900/40 p-4">
      <Card className="w-full max-w-lg">
        <CardBody>
          <h2 className="mb-1 text-base font-semibold text-slate-900">Sửa metadata Voice</h2>
          <p className="mb-4 text-xs text-slate-500">
            Các ô dưới đây được điền sẵn từ bài gốc trên {platformLabel(voice.platform)} — sửa lại
            nếu cần rồi bấm Lưu.
          </p>
          <form onSubmit={submit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700">Tiêu đề</label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700">Mô tả</label>
              <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700">
                Hashtag <span className="text-red-700">*</span>
              </label>
              <Input
                value={hashtag}
                onChange={(e) => setHashtag(e.target.value)}
                placeholder="#tinnong #vietnam"
              />
              <p className="mt-1 text-xs text-slate-500">
                multime.ai yêu cầu ít nhất 1 hashtag. Bỏ trống thì hệ thống dùng hashtag mặc định
                trong cấu hình.
              </p>
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700">Ngôn ngữ</label>
              <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
                {LANGUAGE_OPTIONS.some((o) => o.value === language) ? null : (
                  <option value={language}>{language}</option>
                )}
                {LANGUAGE_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700">
                Ảnh bìa (URL)
              </label>
              <Input value={imageUrl} onChange={(e) => setImageUrl(e.target.value)} />
              {imageUrl ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={imageUrl} alt="" className="mt-2 h-24 rounded object-cover" />
              ) : null}
            </div>

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
        </CardBody>
      </Card>
    </div>
  );
}
