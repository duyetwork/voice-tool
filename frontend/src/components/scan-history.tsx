"use client";

import * as React from "react";

import { ErrorNote } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import { type ChannelKind, useScanRuns } from "@/hooks/use-api";
import { formatDateTime } from "@/lib/utils";
import type { ScanRun } from "@/types/api";

/**
 * SCAN_RUN_LABELS / scanRunTone — trạng thái một vòng quét.
 *
 * Để ở đây chứ không trong badge.tsx: `statusTone` bên đó map trạng thái của
 * Bài Post/Voice, nơi "running" không tồn tại và "error" viết là "failed".
 * Nhét thêm vào sẽ là hai bộ từ vựng dùng chung một hàm.
 */
export const SCAN_RUN_LABELS: Record<string, string> = {
  running: "Đang quét",
  success: "Đã quét xong",
  error: "Lỗi",
};

export function scanRunTone(status: string): "info" | "success" | "danger" | "neutral" {
  switch (status) {
    case "running":
      return "info";
    case "success":
      return "success";
    case "error":
      return "danger";
    default:
      return "neutral";
  }
}

/**
 * ScanStatusBadge — trạng thái quét của một kênh trên bảng danh sách.
 *
 * Chuỗi rỗng = kênh chưa quét lần nào. Đây là trạng thái riêng, không phải
 * "xong": kênh vừa thêm và kênh quét xong mà không ra bài nào trông giống hệt
 * nhau nếu gộp chúng lại.
 */
export function ScanStatusBadge({ status }: { status?: string }) {
  if (!status) {
    return <span className="text-xs text-slate-400">chưa quét</span>;
  }
  return <Badge tone={scanRunTone(status)}>{SCAN_RUN_LABELS[status] ?? status}</Badge>;
}

/**
 * ScanHistoryPanel — tab "Lịch sử quét" của một kênh.
 *
 * Đây là chỗ duy nhất trả lời được những câu mà bảng kênh không trả lời nổi:
 * kênh này quét lúc nào và bao lâu một lần THẬT SỰ (khác với cấu hình), vòng
 * vừa rồi do lịch chạy hay ai đó bấm, và mấy hôm nay nó có ra bài nào không.
 * Kênh chỉ giữ được trạng thái của vòng gần nhất, và vòng sau ghi đè vòng
 * trước.
 */
export function ScanHistoryPanel({ kind, listId }: { kind: ChannelKind; listId: string }) {
  const paging = usePaging();
  const query = useScanRuns(kind, listId, paging.limit, paging.offset);
  const data = query.data;

  if (query.isLoading) {
    return <p className="py-6 text-center text-sm text-slate-500">Đang tải…</p>;
  }
  if (query.error) {
    return <ErrorNote error={query.error} />;
  }

  const items = data?.items ?? [];

  return (
    <div className="space-y-3">
      {/* Tổng kết 7 ngày: một trang 20 dòng không trả lời được "mấy hôm nay
          kênh này có ra gì không" — mà đó mới là câu người ta mở tab để hỏi. */}
      <p className="text-sm text-slate-600">
        7 ngày qua: <b>{data?.runs_7d ?? 0}</b> vòng quét, tạo <b>{data?.posts_created_7d ?? 0}</b>{" "}
        bài post và <b>{data?.voices_created_7d ?? 0}</b> voice
        {data?.failed_7d ? (
          <>
            {" — "}
            <span className="font-medium text-red-700">{data.failed_7d} vòng lỗi</span>
          </>
        ) : null}
        .
      </p>

      <Table>
        <thead>
          <tr>
            <Th>Bắt đầu</Th>
            <Th>Trạng thái</Th>
            <Th>Ai quét</Th>
            {/* "Xin" đứng NGAY TRƯỚC "xét": câu hỏi đầu tiên khi thấy một con
                số nhỏ luôn là "xin bao nhiêu", và hai cột cạnh nhau thì đọc
                một lượt là hiểu, không phải đi tra cấu hình hiện tại của kênh
                — vốn có thể đã bị sửa sau vòng quét đó. */}
            <Th className="text-right">Bài xin</Th>
            <Th className="text-right">Bài xét</Th>
            <Th className="text-right">Bài post</Th>
            <Th className="text-right">Voice</Th>
          </tr>
        </thead>
        <tbody>
          {items.length === 0 ? (
            <EmptyRow colSpan={7}>
              Kênh này chưa chạy vòng quét nào. Lịch sử chỉ giữ 30 ngày.
            </EmptyRow>
          ) : (
            items.map((run) => <ScanRunRow key={run.id} run={run} />)
          )}
        </tbody>
      </Table>

      <Pagination total={data?.total} paging={paging} unit="vòng quét" />
    </div>
  );
}

function ScanRunRow({ run }: { run: ScanRun }) {
  return (
    <tr>
      <Td className="whitespace-nowrap text-slate-900">
        {formatDateTime(run.started_at)}
        <div className="text-xs text-slate-500">{describeLength(run)}</div>
      </Td>
      <Td>
        <Badge tone={scanRunTone(run.status)}>{SCAN_RUN_LABELS[run.status] ?? run.status}</Badge>
        {run.error ? <p className="mt-1 max-w-64 text-xs text-red-700">{run.error}</p> : null}
      </Td>
      <Td className="whitespace-nowrap text-xs">
        {run.trigger_kind === "manual" ? (
          <>
            <div className="text-slate-900">Quét thử</div>
            {/* Email rỗng ở vòng thủ công nghĩa là tài khoản đã bị xoá —
                trigger_kind vẫn nói đúng rằng có người đã bấm. */}
            <div className="text-slate-500">{run.triggered_by_email || "tài khoản đã xoá"}</div>
          </>
        ) : (
          <span className="text-slate-500">Hệ thống (theo lịch)</span>
        )}
      </Td>
      <Td className="text-right tabular-nums text-slate-500">
        {/* 0 = dòng có từ trước khi cột này tồn tại. Hiện dấu gạch chứ không
            hiện số 0: "xin 0 bài" và "không rõ" là hai chuyện khác nhau. */}
        {run.requested_limit > 0 ? run.requested_limit : "—"}
      </Td>
      <Td className="text-right tabular-nums text-slate-600">
        {run.fetched}
        {/* Xin nhiều mà lấy được ít là thứ cần thấy ngay: nền tảng không trả
            đủ. Chỉ hiện khi vòng đó CHẠY ĐƯỢC — vòng lỗi thì đã có câu lỗi
            riêng rồi, thêm dòng này chỉ làm nhiễu. */}
        {run.status === "success" && run.requested_limit > run.fetched ? (
          <div className="text-xs font-normal text-amber-700">
            thiếu {run.requested_limit - run.fetched}
          </div>
        ) : null}
      </Td>
      <Td className="text-right tabular-nums font-medium text-slate-900">{run.posts_created}</Td>
      <Td className="text-right tabular-nums text-slate-900">{run.voices_created}</Td>
    </tr>
  );
}

/** describeLength: vòng đã xong thì hiện mất bao lâu, đang chạy thì nói vậy. */
function describeLength(run: ScanRun): string {
  if (!run.finished_at) return "đang chạy…";
  const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  if (ms < 1000) return "< 1 giây";
  if (ms < 60_000) return `${Math.round(ms / 1000)} giây`;
  return `${Math.round(ms / 60_000)} phút`;
}
