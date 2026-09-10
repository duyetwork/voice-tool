"use client";

import * as React from "react";

import { DuplicatePostNotice } from "@/components/duplicate-notice";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Checkbox, Field, Input, Select } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  duplicateOf,
  useCollectModes,
  useCreateSourcePost,
  usePlatforms,
  usePrompts,
  useSourcePosts,
} from "@/hooks/use-api";
import { LANGUAGE_OPTIONS } from "@/lib/languages";
import {
  COLLECT_MODE_LABELS,
  collectModeLabel,
  POST_STATUS_LABELS,
  platformLabel,
} from "@/lib/utils";
import type { CollectMode, DuplicatePost } from "@/types/api";

/**
 * F1 — Tạo Voice theo yêu cầu.
 *
 * Luôn tạo Bài Post trước rồi mới sinh Voice (business rule #1); auto_process
 * mặc định bật nên 1 lần submit là ra voice.
 */
export default function OnDemandPage() {
  const [sourceUrl, setSourceUrl] = React.useState("");
  // Giai đoạn hiện tại ưu tiên Mode A (extract audio gốc từ URL).
  const [collectMode, setCollectMode] = React.useState<CollectMode>("A");
  const [promptId, setPromptId] = React.useState("");
  // Mặc định để hệ thống tự nhận diện ngôn ngữ.
  const [language, setLanguage] = React.useState("auto");
  const [platform, setPlatform] = React.useState("");
  const [autoProcess, setAutoProcess] = React.useState(true);
  // Bài trùng đang chờ người dùng quyết định bỏ qua hay vẫn tạo mới.
  const [duplicate, setDuplicate] = React.useState<DuplicatePost | null>(null);

  const prompts = usePrompts();
  const platforms = usePlatforms();
  const modes = useCollectModes();
  const create = useCreateSourcePost();
  const paging = usePaging(10);
  const sorting = useSorting("created_at", paging.reset);
  const recent = useSourcePosts({
    source_type: "F1",
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });

  const needsPrompt = collectMode === "C";
  const modeMeta = modes.data?.collect_modes ?? [];
  const isEnabled = (mode: string) =>
    modeMeta.find((m) => m.mode === mode)?.enabled ?? mode === "A";

  async function send(allowDuplicate: boolean) {
    try {
      await create.mutateAsync({
        source_url: sourceUrl.trim(),
        collect_mode: collectMode,
        prompt_id: needsPrompt && promptId ? promptId : null,
        language,
        auto_process: autoProcess,
        platform: platform || undefined,
        allow_duplicate: allowDuplicate || undefined,
      });
      setDuplicate(null);
      setSourceUrl("");
    } catch (err) {
      // Trùng bài thì không phải lỗi: hỏi lại người dùng.
      const dup = duplicateOf(err);
      if (!dup) throw err;
      setDuplicate(dup);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await send(false);
  }

  return (
    <>
      <PageHeader
        title="F1 — Tạo Voice theo yêu cầu"
        description="Dán 1 URL, hệ thống tự nhận diện nền tảng và ID bài đăng, tạo Bài Post rồi sinh Voice."
      />

      <div className="grid gap-6 xl:grid-cols-[420px_1fr]">
        <Can permission="can_write">
          <Card>
            <CardHeader title="Nhập URL nguồn" />
            <CardBody>
              <form onSubmit={submit} className="space-y-4">
                <Field
                  label="URL bài đăng"
                  hint="Hỗ trợ YouTube, Facebook, TikTok, Instagram, X — hệ thống tự nhận diện nền tảng từ URL."
                >
                  <Input
                    type="url"
                    placeholder="https://www.youtube.com/watch?v=..."
                    value={sourceUrl}
                    onChange={(e) => setSourceUrl(e.target.value)}
                    required
                  />
                </Field>

                <Field
                  label="Nền tảng"
                  hint="Không bắt buộc — chỉ chọn khi URL rút gọn/lạ khiến hệ thống không tự nhận ra."
                >
                  <Select value={platform} onChange={(e) => setPlatform(e.target.value)}>
                    <option value="">Tự nhận diện từ URL</option>
                    {(platforms.data?.platforms ?? []).map((p) => (
                      <option key={p} value={p}>
                        {platformLabel(p)}
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
                  <Field label="Prompt mẫu" hint="Mode C bắt buộc chọn prompt.">
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
                  label="Ngôn ngữ"
                  hint="Tự nhận diện: lấy theo nền tảng khai báo, không có thì để multime.ai nhận diện từ audio."
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
                  <Checkbox
                    checked={autoProcess}
                    onChange={(e) => setAutoProcess(e.target.checked)}
                  />
                  Chạy tạo Voice ngay sau khi tạo Bài Post
                </label>

                {duplicate ? (
                  <DuplicatePostNotice
                    existing={duplicate}
                    pending={create.isPending}
                    onSkip={() => {
                      setDuplicate(null);
                      setSourceUrl("");
                    }}
                    onCreateAnyway={() => void send(true)}
                  />
                ) : (
                  <ErrorNote error={create.error} />
                )}

                <Button type="submit" className="w-full" disabled={create.isPending}>
                  {create.isPending ? "Đang xử lý…" : "Tạo Bài Post"}
                </Button>

                {create.isSuccess ? (
                  <p className="rounded-md bg-green-50 px-3 py-2 text-sm text-green-800">
                    Đã tạo Bài Post{autoProcess ? " và đưa vào hàng đợi tạo Voice." : "."}
                  </p>
                ) : null}
              </form>
            </CardBody>
          </Card>
        </Can>

        <Card>
          <CardHeader
            title="Bài Post F1 gần đây"
            description="Voice sinh ra từ các bài này xem ở màn Voice."
            action={
              <Button variant="secondary" size="sm" onClick={() => recent.refetch()}>
                Làm mới
              </Button>
            }
          />
          <CardBody className="p-0">
            <Table>
              <thead>
                <tr>
                  <Th>Bài gốc</Th>
                  <Th>Nền tảng</Th>
                  <Th>Hình thức</Th>
                  <Th>Ngôn ngữ</Th>
                  <Th>Người tạo</Th>
                  <Th>Trạng thái</Th>
                  <SortableTh sorting={sorting} column="created_at">
                    Tạo lúc
                  </SortableTh>
                </tr>
              </thead>
              <tbody>
                {recent.isLoading ? (
                  <EmptyRow colSpan={7}>Đang tải…</EmptyRow>
                ) : recent.data?.items.length ? (
                  recent.data.items.map((post) => (
                    <tr key={post.id}>
                      <Td className="max-w-xs">
                        {post.title ? (
                          <p className="truncate font-medium text-slate-900">{post.title}</p>
                        ) : null}
                        <a
                          href={post.source_url}
                          target="_blank"
                          rel="noreferrer"
                          className="block truncate text-xs text-indigo-700 hover:underline"
                        >
                          {post.source_url}
                        </a>
                        {post.last_error ? (
                          <p className="mt-1 text-xs text-red-700">{post.last_error}</p>
                        ) : null}
                      </Td>
                      <Td className="whitespace-nowrap">{platformLabel(post.platform)}</Td>
                      <Td className="text-xs">{collectModeLabel(post.collect_mode)}</Td>
                      <Td>{post.language}</Td>
                      <Td className="text-xs text-slate-600">{post.created_by_email ?? "—"}</Td>
                      <Td>
                        <Badge tone={statusTone(post.status)}>
                          {POST_STATUS_LABELS[post.status] ?? post.status}
                        </Badge>
                      </Td>
                      <Td>
                        <DateCell value={post.created_at} />
                      </Td>
                    </tr>
                  ))
                ) : (
                  <EmptyRow colSpan={7}>Chưa có Bài Post nào.</EmptyRow>
                )}
              </tbody>
            </Table>

            <Pagination total={recent.data?.total} paging={paging} unit="bài" />
          </CardBody>
        </Card>
      </div>
    </>
  );
}
