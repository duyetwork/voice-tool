"use client";

import * as React from "react";

import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Input, Select } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, RowActions, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import {
  useCollectModes,
  useDeleteSourcePost,
  usePlatforms,
  useRunSourcePost,
  useSourcePosts,
  useUpdateSourcePost,
} from "@/hooks/use-api";
import { LANGUAGE_OPTIONS, compactLanguageOptions } from "@/lib/languages";
import {
  COLLECT_MODE_LABELS,
  POST_STATUS_LABELS,
  SOURCE_TYPE_LABELS,
  platformLabel,
} from "@/lib/utils";
import type { CollectMode } from "@/types/api";

const EMPTY_FILTERS = {
  source_type: "",
  status: "",
  platform: "",
  language: "",
  created_from: "",
  created_to: "",
};

/**
 * Màn duyệt Bài Post — dùng để lọc/duyệt trước khi tốn chi phí AI tạo Voice
 * (đặc biệt hữu ích với Danh sách Định kỳ khi 1 lần quét ra nhiều bài).
 *
 * Metadata gốc (tiêu đề, mô tả, hashtag, ảnh bìa) hiện ngay ở đây và được
 * chuyển thẳng sang Voice khi bấm "Chạy tạo Voice".
 */
export default function SourcePostsPage() {
  const [filters, setFilters] = React.useState(EMPTY_FILTERS);
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  // Nút "Xoá lọc" chỉ hiện khi thực sự có gì để xoá.
  const hasFilters = Object.values(filters).some(Boolean);

  const set = (key: keyof typeof EMPTY_FILTERS) => (value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
    paging.reset();
  };

  const posts = useSourcePosts({
    source_type: filters.source_type || undefined,
    status: filters.status || undefined,
    platform: filters.platform || undefined,
    language: filters.language || undefined,
    created_from: filters.created_from || undefined,
    created_to: filters.created_to || undefined,
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });
  const platforms = usePlatforms();
  const modes = useCollectModes();
  // Mode B/C chỉ mở khi server bật ENABLED_COLLECT_MODES — hiện mờ thay vì để
  // người dùng chọn rồi ăn lỗi 400 lúc chạy.
  const modeEnabled = (mode: string) =>
    modes.data?.collect_modes.find((m) => m.mode === mode)?.enabled ?? mode === "A";
  const run = useRunSourcePost();
  const update = useUpdateSourcePost();
  const remove = useDeleteSourcePost();
  const selection = useSelection(posts.data?.items);

  return (
    <>
      <PageHeader
        title="Bài Post"
        description="Tầng trung gian bắt buộc giữa URL nguồn và Voice — mọi Voice đều truy vết về đúng 1 Bài Post."
      />

      <Card>
        <CardBody className="flex flex-wrap items-end gap-3 border-b border-slate-200">
          <div className="w-48">
            <label className="mb-1 block text-xs font-medium text-slate-500">Nguồn</label>
            <Select value={filters.source_type} onChange={(e) => set("source_type")(e.target.value)}>
              <option value="">Tất cả</option>
              {Object.entries(SOURCE_TYPE_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Select>
          </div>

          <div className="w-40">
            <label className="mb-1 block text-xs font-medium text-slate-500">Trạng thái</label>
            <Select value={filters.status} onChange={(e) => set("status")(e.target.value)}>
              <option value="">Tất cả</option>
              {Object.entries(POST_STATUS_LABELS).map(([value, label]) => (
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

          <Button variant="secondary" onClick={() => posts.refetch()}>
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
            {posts.data ? `${posts.data.total} bài` : ""}
          </span>
        </CardBody>

        <CardBody className="p-0">
          <ErrorNote error={posts.error ?? run.error ?? remove.error ?? update.error} />

          <Can permission="can_write">
            <BulkBar
              count={selection.count}
              ids={selection.selected}
              onDone={selection.clear}
              actions={[
                {
                  label: "Chạy tạo Voice",
                  variant: "primary",
                  onRun: async ([id]) => {
                    await run.mutateAsync(id);
                  },
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
                  confirm: "Xoá các Bài Post đã chọn? Voice liên quan cũng bị xoá.",
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
                <Th>Bài gốc</Th>
                <Th>Nền tảng</Th>
                <Th>Danh sách kênh</Th>
                <Th>Hình thức tạo voice</Th>
                <Th>Ngôn ngữ</Th>
                <Th>Người tạo</Th>
                <Th>Trạng thái</Th>
                <SortableTh sorting={sorting} column="created_at">
                  Tạo lúc
                </SortableTh>
                <Can permission="can_write">
                  <Th className="text-right">Hành động</Th>
                </Can>
              </tr>
            </thead>
            <tbody>
              {posts.isLoading ? (
                <EmptyRow colSpan={10}>Đang tải…</EmptyRow>
              ) : posts.data?.items.length ? (
                posts.data.items.map((post) => (
                  <tr key={post.id}>
                    <Can permission="can_write">
                      <Td>
                        <Checkbox
                          checked={selection.isSelected(post.id)}
                          onChange={() => selection.toggle(post.id)}
                          aria-label="Chọn bài post"
                        />
                      </Td>
                    </Can>

                    <Td className="max-w-xs">
                      <div className="flex gap-3">
                        {post.thumbnail_url ? (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img
                            src={post.thumbnail_url}
                            alt=""
                            className="h-12 w-20 shrink-0 rounded object-cover"
                          />
                        ) : null}
                        <div className="min-w-0">
                          {/* Tiêu đề = toàn bộ nội dung bài (trừ hashtag) nên
                              có thể dài; bảng chỉ hiện 3 dòng, xem đủ bằng
                              tooltip. */}
                          <p
                            className="line-clamp-3 whitespace-pre-line font-medium text-slate-900"
                            title={post.title ?? undefined}
                          >
                            {post.title ?? (
                              <span className="font-normal text-slate-400">(chưa có nội dung)</span>
                            )}
                          </p>
                          {post.hashtags?.length ? (
                            <p className="mt-0.5 line-clamp-2 text-xs text-indigo-700">
                              {post.hashtags.join(" ")}
                            </p>
                          ) : null}
                        </div>
                      </div>
                    </Td>

                    <Td className="whitespace-nowrap">
                      {/* Bài nhập tay bằng text không có URL nguồn để mở. */}
                      {post.source_url ? (
                        <a
                          href={post.source_url}
                          target="_blank"
                          rel="noreferrer"
                          className="text-indigo-700 hover:underline"
                        >
                          {platformLabel(post.platform)}
                        </a>
                      ) : (
                        platformLabel(post.platform)
                      )}
                    </Td>

                    {/* Bài này từ kênh nào ra. Kênh không có cột tên nên hiện
                        URL của nó — cũng là thứ dùng để nhận ra kênh ở màn
                        Danh sách kênh. Dòng trên là loại kênh (F2/F3) để phân
                        biệt hai kênh trùng URL nhưng khác loại. */}
                    <Td className="max-w-48 text-xs">
                      {post.list_source_url ? (
                        <>
                          <div className="text-slate-500">
                            {SOURCE_TYPE_LABELS[post.source_type] ?? post.source_type}
                          </div>
                          <a
                            href={post.list_source_url}
                            target="_blank"
                            rel="noreferrer"
                            className="block truncate text-indigo-700 hover:underline"
                            title={post.list_source_url}
                          >
                            {post.list_source_url}
                          </a>
                        </>
                      ) : (
                        <span className="text-slate-400">Nhập tay</span>
                      )}
                    </Td>

                    <Td>
                      <Can permission="can_write" fallback={<span>{post.collect_mode}</span>}>
                        <Select
                          className="h-8 w-40 text-xs"
                          value={post.collect_mode}
                          disabled={post.status === "processing" || update.isPending}
                          onChange={(e) =>
                            update.mutate({
                              id: post.id,
                              collect_mode: e.target.value as CollectMode,
                            })
                          }
                          aria-label="Hình thức tạo voice"
                        >
                          {Object.entries(COLLECT_MODE_LABELS).map(([value, label]) => (
                            <option
                              key={value}
                              value={value}
                              disabled={!modeEnabled(value) && value !== post.collect_mode}
                            >
                              {label}
                              {modeEnabled(value) ? "" : " — chưa hỗ trợ"}
                            </option>
                          ))}
                        </Select>
                      </Can>

                    </Td>

                    <Td>
                      <Can permission="can_write" fallback={<span>{post.language}</span>}>
                        <Select
                          className="h-8 w-28 text-xs"
                          value={post.language}
                          disabled={post.status === "processing" || update.isPending}
                          onChange={(e) => update.mutate({ id: post.id, language: e.target.value })}
                        >
                          {compactLanguageOptions(post.language).map((o) => (
                            <option key={o.value} value={o.value}>
                              {o.label}
                            </option>
                          ))}
                        </Select>
                      </Can>
                    </Td>

                    <Td className="max-w-28 text-xs text-slate-600">
                      <span className="block truncate" title={post.created_by_email}>
                        {post.created_by_email ?? "—"}
                      </span>
                    </Td>

                    <Td className="max-w-48">
                      <Badge tone={statusTone(post.status)}>
                        {POST_STATUS_LABELS[post.status] ?? post.status}
                      </Badge>
                      {/* Lý do lỗi nằm ngay dưới trạng thái — đọc 1 chỗ là
                          hiểu, không phải dò trong ô nội dung. */}
                      {post.last_error ? (
                        <p className="mt-1 text-xs text-red-700">{post.last_error}</p>
                      ) : null}
                    </Td>
                    <Td>
                      <DateCell value={post.created_at} />
                    </Td>

                    <Can permission="can_write">
                      <Td className="whitespace-nowrap text-right">
                        <RowActions>
                          {/* Đã tạo voice xong thì không hiện nút nữa — chạy
                              lại chỉ sinh thêm voice trùng. Lỗi thì cho chạy
                              lại, đang xử lý thì hiện mờ để biết là đang chạy. */}
                          {post.status !== "processed" ? (
                            <Button
                              size="sm"
                              variant="secondary"
                              disabled={post.status === "processing" || run.isPending}
                              onClick={() => run.mutate(post.id)}
                            >
                              {post.status === "failed"
                                ? "Chạy lại Voice"
                                : post.status === "processing"
                                  ? "Đang xử lý…"
                                  : "Chạy tạo Voice"}
                            </Button>
                          ) : null}
                          <Can permission="can_delete">
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={() => {
                                if (confirm("Xoá Bài Post này? Voice liên quan cũng bị xoá.")) {
                                  remove.mutate(post.id);
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
                <EmptyRow colSpan={10}>Không có Bài Post khớp bộ lọc.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={posts.data?.total} paging={paging} unit="bài" />
        </CardBody>
      </Card>
    </>
  );
}
