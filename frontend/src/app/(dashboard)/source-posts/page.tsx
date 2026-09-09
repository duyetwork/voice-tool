"use client";

import * as React from "react";

import { BulkBar, SelectAllBox, useSelection } from "@/components/bulk";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { Can, ViewerNotice } from "@/components/permission";
import { Badge, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Checkbox, Input, Select } from "@/components/ui/field";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import {
  useDeleteSourcePost,
  usePlatforms,
  useRunSourcePost,
  useSourcePosts,
  useUpdateSourcePost,
} from "@/hooks/use-api";
import {
  LANGUAGE_OPTIONS,
  POST_STATUS_LABELS,
  SOURCE_TYPE_LABELS,
  formatDateTime,
  platformLabel,
} from "@/lib/utils";

/**
 * Màn duyệt Bài Post — dùng để lọc/duyệt trước khi tốn chi phí AI tạo Voice
 * (đặc biệt hữu ích với Danh sách Định kỳ khi 1 lần quét ra nhiều bài).
 *
 * Metadata gốc (tiêu đề, mô tả, hashtag, ảnh bìa) hiện ngay ở đây và được
 * chuyển thẳng sang Voice khi bấm "Chạy tạo Voice".
 */
export default function SourcePostsPage() {
  const empty = {
    source_type: "",
    status: "",
    platform: "",
    language: "",
    created_from: "",
    created_to: "",
  };
  const [filters, setFilters] = React.useState(empty);
  const set = (key: keyof typeof empty) => (value: string) =>
    setFilters((f) => ({ ...f, [key]: value }));

  const posts = useSourcePosts({
    source_type: filters.source_type || undefined,
    status: filters.status || undefined,
    platform: filters.platform || undefined,
    language: filters.language || undefined,
    created_from: filters.created_from || undefined,
    created_to: filters.created_to || undefined,
    limit: 50,
  });
  const platforms = usePlatforms();
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
      <ViewerNotice />

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

          <div className="w-36">
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
          <Button variant="ghost" onClick={() => setFilters(empty)}>
            Xoá lọc
          </Button>
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
                <Th>Bài gốc</Th>
                <Th>Nền tảng</Th>
                <Th>Ngôn ngữ</Th>
                <Th>Người tạo</Th>
                <Th>Trạng thái</Th>
                <Th>Tạo lúc</Th>
                <Can permission="can_write">
                  <Th className="text-right">Hành động</Th>
                </Can>
              </tr>
            </thead>
            <tbody>
              {posts.isLoading ? (
                <EmptyRow colSpan={8}>Đang tải…</EmptyRow>
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

                    <Td className="max-w-md">
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
                          <p className="mt-0.5 text-xs text-slate-500">
                            {SOURCE_TYPE_LABELS[post.source_type] ?? post.source_type}
                            {post.author_name ? ` · ${post.author_name}` : ""}
                            {post.content_type ? ` · ${post.content_type}` : ""}
                          </p>
                          {post.hashtags?.length ? (
                            <p className="mt-0.5 truncate text-xs text-indigo-700">
                              {post.hashtags.join(" ")}
                            </p>
                          ) : null}
                        </div>
                      </div>
                      {post.last_error ? (
                        <p className="mt-1 text-xs text-red-700">{post.last_error}</p>
                      ) : null}
                    </Td>

                    <Td className="whitespace-nowrap">{platformLabel(post.platform)}</Td>

                    <Td>
                      <Can permission="can_write" fallback={<span>{post.language}</span>}>
                        <Select
                          className="h-8 w-36 text-xs"
                          value={post.language}
                          disabled={post.status === "processing" || update.isPending}
                          onChange={(e) => update.mutate({ id: post.id, language: e.target.value })}
                        >
                          {LANGUAGE_OPTIONS.some((o) => o.value === post.language) ? null : (
                            <option value={post.language}>{post.language}</option>
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
                      {post.created_by_email ?? "—"}
                    </Td>

                    <Td>
                      <Badge tone={statusTone(post.status)}>
                        {POST_STATUS_LABELS[post.status] ?? post.status}
                      </Badge>
                    </Td>
                    <Td className="whitespace-nowrap">{formatDateTime(post.created_at)}</Td>

                    <Can permission="can_write">
                      <Td className="whitespace-nowrap text-right">
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={post.status === "processing" || run.isPending}
                          onClick={() => run.mutate(post.id)}
                        >
                          Chạy tạo Voice
                        </Button>{" "}
                        <Can permission="can_delete">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              if (confirm("Xoá Bài Post này? Voice liên quan cũng bị xoá.")) {
                                remove.mutate(post.id);
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
                <EmptyRow colSpan={8}>Không có Bài Post khớp bộ lọc.</EmptyRow>
              )}
            </tbody>
          </Table>
        </CardBody>
      </Card>
    </>
  );
}
