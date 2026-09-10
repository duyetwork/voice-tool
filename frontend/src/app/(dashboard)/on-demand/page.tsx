"use client";

import * as React from "react";
import { useRouter } from "next/navigation";

import { DuplicatePostNotice } from "@/components/duplicate-notice";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  duplicateOf,
  useCollectModes,
  useCreateSourcePost,
  useCreateTextVoice,
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
 * MAX_TTS_TEXT_LENGTH khớp domain.MaxTTSTextRunes phía backend — API cũng
 * chặn, đây chỉ để người dùng thấy còn bao nhiêu ký tự trước khi bấm.
 */
const MAX_TTS_TEXT_LENGTH = 20000;

/**
 * F1 — Tạo Voice theo yêu cầu.
 *
 * Hai kiểu nhập liệu loại trừ nhau, và đi 2 đường KHÁC NHAU:
 *
 *   - URL:  tạo Bài Post trước rồi mới sinh Voice (business rule #1) — có bài
 *           gốc để truy vết và để chạy lại với mode/prompt khác.
 *   - Text: tạo thẳng Voice, không sinh Bài Post. Text gõ tay không có bài gốc
 *           nào để truy vết nên Bài Post ở giữa chỉ là bản ghi rỗng.
 *
 * Cả hai đường đều kết thúc ở màn Voice, nên submit xong là chuyển sang đó.
 */
type InputMode = "url" | "text";

export default function OnDemandPage() {
  const [inputMode, setInputMode] = React.useState<InputMode>("url");
  const [sourceUrl, setSourceUrl] = React.useState("");
  const [text, setText] = React.useState("");
  const [collectMode, setCollectMode] = React.useState<CollectMode>("A");
  const [promptId, setPromptId] = React.useState("");
  // Mặc định để hệ thống tự nhận diện ngôn ngữ.
  const [language, setLanguage] = React.useState("auto");
  const [platform, setPlatform] = React.useState("");
  const [autoProcess, setAutoProcess] = React.useState(true);
  // Bài trùng đang chờ người dùng quyết định bỏ qua hay vẫn tạo mới.
  const [duplicate, setDuplicate] = React.useState<DuplicatePost | null>(null);

  const router = useRouter();
  const prompts = usePrompts();
  const platforms = usePlatforms();
  const modes = useCollectModes();
  const create = useCreateSourcePost();
  const createVoice = useCreateTextVoice();
  const paging = usePaging(10);
  const sorting = useSorting("created_at", paging.reset);
  const recent = useSourcePosts({
    source_type: "F1",
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });

  const isTextInput = inputMode === "text";
  const needsPrompt = collectMode === "C";
  const pending = create.isPending || createVoice.isPending;
  const modeMeta = modes.data?.collect_modes ?? [];
  const metaOf = (mode: string) => modeMeta.find((m) => m.mode === mode);
  const isEnabled = (mode: string) => metaOf(mode)?.enabled ?? mode === "A";
  // Mode tắt thì nói luôn thiếu gì (vd chưa có ANTHROPIC_API_KEY) — server đã
  // trả kèm lý do ở /meta/collect-modes.
  const modeNote = (mode: string) =>
    isEnabled(mode) ? "" : ` — ${metaOf(mode)?.reason ?? "chưa được bật"}`;
  // Nhập text thì không có audio gốc -> hình thức A không dùng được.
  const modeAvailable = (mode: string) => isEnabled(mode) && !(isTextInput && mode === "A");

  // Đổi sang nhập text khi đang để hình thức A: chuyển sẵn sang B thay vì bắt
  // người dùng tự sửa rồi mới submit được.
  function switchInput(mode: InputMode) {
    setInputMode(mode);
    setDuplicate(null);
    if (mode === "text" && collectMode === "A") setCollectMode("B");
  }

  async function send(allowDuplicate: boolean) {
    // Nhập text: tạo thẳng Voice, không sinh Bài Post -> cũng không có bài
    // trùng để hỏi.
    if (isTextInput) {
      await createVoice.mutateAsync({
        text: text.trim(),
        collect_mode: collectMode === "C" ? "C" : "B",
        prompt_id: needsPrompt && promptId ? promptId : null,
        language,
      });
      setText("");
      router.push("/voices");
      return;
    }

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
      // Có voice để xem thì sang thẳng màn Voice; không thì ở lại đây, bài mới
      // hiện ngay trong bảng bên cạnh.
      if (autoProcess) router.push("/voices");
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
        description="Dán 1 URL để hệ thống tự lấy nội dung, hoặc gõ thẳng text cho TTS đọc. Xong là sang màn Voice."
      />

      <div className="grid gap-6 xl:grid-cols-[420px_1fr]">
        <Can permission="can_write">
          <Card>
            <CardHeader title="Nguồn nội dung" />
            <CardBody>
              <form onSubmit={submit} className="space-y-4">
                {/* 2 kiểu nhập liệu loại trừ nhau nên dùng nút chọn thay vì 2 ô
                    cùng hiện — đỡ phải đoán ô nào thắng khi điền cả hai. */}
                <div className="grid grid-cols-2 gap-2 rounded-lg bg-slate-100 p-1">
                  {(
                    [
                      ["url", "Từ URL"],
                      ["text", "Nhập text"],
                    ] as [InputMode, string][]
                  ).map(([value, label]) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => switchInput(value)}
                      className={
                        "rounded-md px-3 py-1.5 text-sm font-medium transition " +
                        (inputMode === value
                          ? "bg-white text-slate-900 shadow-sm"
                          : "text-slate-600 hover:text-slate-900")
                      }
                    >
                      {label}
                    </button>
                  ))}
                </div>

                {isTextInput ? (
                  <Field
                    label={`Nội dung TTS đọc (còn ${(MAX_TTS_TEXT_LENGTH - text.length).toLocaleString("vi-VN")} ký tự)`}
                    required
                  >
                    <Textarea
                      className="min-h-48"
                      maxLength={MAX_TTS_TEXT_LENGTH}
                      placeholder="Bản tin sáng nay…"
                      value={text}
                      onChange={(e) => setText(e.target.value)}
                      required
                    />
                  </Field>
                ) : (
                  <Field label="URL bài đăng" required>
                    <Input
                      type="url"
                      placeholder="https://www.youtube.com/watch?v=..."
                      value={sourceUrl}
                      onChange={(e) => setSourceUrl(e.target.value)}
                      required
                    />
                  </Field>
                )}

                {isTextInput ? null : (
                  <Field label="Nền tảng">
                    <Select value={platform} onChange={(e) => setPlatform(e.target.value)}>
                      <option value="">Tự nhận diện từ URL</option>
                      {(platforms.data?.platforms ?? []).map((p) => (
                        <option key={p} value={p}>
                          {platformLabel(p)}
                        </option>
                      ))}
                    </Select>
                  </Field>
                )}

                <Field label="Hình thức thu thập" required>
                  <Select
                    value={collectMode}
                    onChange={(e) => setCollectMode(e.target.value as CollectMode)}
                  >
                    {Object.entries(COLLECT_MODE_LABELS).map(([value, label]) => (
                      <option key={value} value={value} disabled={!modeAvailable(value)}>
                        {label}
                        {modeNote(value)}
                        {isEnabled(value) && !modeAvailable(value) ? " — cần URL" : ""}
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

                <Field label="Ngôn ngữ">
                  <Select value={language} onChange={(e) => setLanguage(e.target.value)}>
                    {LANGUAGE_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </Select>
                </Field>

                {/* Nhập text không sinh Bài Post nên không có gì để "chạy
                    tiếp" — bấm là ra Voice luôn. */}
                {isTextInput ? null : (
                  <label className="flex items-center gap-2 text-sm text-slate-700">
                    <Checkbox
                      checked={autoProcess}
                      onChange={(e) => setAutoProcess(e.target.checked)}
                    />
                    Chạy tạo Voice ngay sau khi tạo Bài Post
                  </label>
                )}

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
                  <ErrorNote error={create.error ?? createVoice.error} />
                )}

                <Button type="submit" className="w-full" disabled={pending}>
                  {pending ? "Đang xử lý…" : isTextInput ? "Tạo Voice" : "Tạo Bài Post"}
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
                        {post.source_url ? (
                          <a
                            href={post.source_url}
                            target="_blank"
                            rel="noreferrer"
                            className="block truncate text-xs text-indigo-700 hover:underline"
                          >
                            {post.source_url}
                          </a>
                        ) : (
                          <p className="text-xs text-slate-500">Nội dung nhập tay</p>
                        )}
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
