"use client";

import { Button } from "@/components/ui/button";
import { POST_STATUS_LABELS, collectModeLabel } from "@/lib/utils";
import type { DuplicatePost } from "@/types/api";

/**
 * DuplicatePostNotice hiện khi URL vừa dán trùng ID bài đăng đã có.
 *
 * Không tự bỏ qua cũng không tự tạo thêm: cả 2 hướng đều có lúc đúng — dán lại
 * cùng bài do quên là muốn bỏ qua, còn chạy lại với hình thức/prompt khác là
 * muốn có bài mới. Bài cũ KHÔNG bị sửa gì trong cả 2 trường hợp.
 */
export function DuplicatePostNotice({
  existing,
  pending,
  onSkip,
  onCreateAnyway,
}: {
  existing: DuplicatePost;
  pending?: boolean;
  onSkip: () => void;
  onCreateAnyway: () => void;
}) {
  const created = new Date(existing.created_at).toLocaleString("vi-VN");

  return (
    <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2.5 text-sm text-amber-900">
      <p className="font-medium">Bài đăng này đã có trong hệ thống</p>

      <dl className="mt-1.5 space-y-0.5 text-xs">
        <div className="flex gap-1.5">
          <dt className="shrink-0 text-amber-700">Tiêu đề:</dt>
          <dd className="line-clamp-2">{existing.title || "(chưa có tiêu đề)"}</dd>
        </div>
        <div className="flex gap-1.5">
          <dt className="shrink-0 text-amber-700">Trạng thái:</dt>
          <dd>
            {POST_STATUS_LABELS[existing.status] ?? existing.status} ·{" "}
            {collectModeLabel(existing.collect_mode)} · tạo lúc {created}
          </dd>
        </div>
        {existing.post_id_extracted ? (
          <div className="flex gap-1.5">
            <dt className="shrink-0 text-amber-700">ID bài:</dt>
            <dd>
              <code>{existing.post_id_extracted}</code>
            </dd>
          </div>
        ) : null}
      </dl>

      <div className="mt-2.5 flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" onClick={onSkip} disabled={pending}>
          Bỏ qua
        </Button>
        <Button size="sm" onClick={onCreateAnyway} disabled={pending}>
          {pending ? "Đang tạo…" : "Vẫn tạo Bài Post mới"}
        </Button>
      </div>
    </div>
  );
}
