"use client";

import * as React from "react";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Select } from "@/components/ui/field";
import { Pagination, usePaging } from "@/components/ui/pagination";
import { DateCell, EmptyRow, SortableTh, Table, Td, Th, useSorting } from "@/components/ui/table";
import { useAuditLog } from "@/hooks/use-api";

const OBJECT_TYPES = [
  { value: "list_breaking", label: "Danh sách Breaking" },
  { value: "list_scheduled", label: "Danh sách Định kỳ" },
  { value: "source_post", label: "Bài Post" },
  { value: "voice", label: "Voice" },
];

const ACTION_TONES: Record<string, "info" | "success" | "warning" | "danger" | "neutral"> = {
  create: "success",
  update: "info",
  delete: "danger",
  run: "warning",
  publish: "success",
};

/** L1/L2/L3 — Nhật ký thao tác. Append-only, chỉ đọc. */
export default function AuditLogPage() {
  const [objectType, setObjectType] = React.useState("");
  const paging = usePaging();
  const sorting = useSorting("created_at", paging.reset);
  const log = useAuditLog({
    object_type: objectType || undefined,
    ...sorting.params,
    limit: paging.limit,
    offset: paging.offset,
  });

  return (
    <>
      <PageHeader
        title="Nhật ký thao tác"
        description="Ghi tự động mọi create/update/delete/run/publish trên Danh sách, Bài Post và Voice. Append-only."
      />

      <Card>
        <CardBody className="flex flex-wrap items-end gap-3 border-b border-slate-200">
          <div className="w-56">
            <Select
              value={objectType}
              onChange={(e) => {
                setObjectType(e.target.value);
                paging.reset();
              }}
            >
              <option value="">Tất cả đối tượng</option>
              {OBJECT_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </Select>
          </div>
          <Button variant="secondary" onClick={() => log.refetch()}>
            Làm mới
          </Button>
          <span className="ml-auto text-sm text-slate-500">
            {log.data ? `${log.data.total} bản ghi` : ""}
          </span>
        </CardBody>

        <CardBody className="p-0">
          <ErrorNote error={log.error} />
          <Table>
            <thead>
              <tr>
                <SortableTh sorting={sorting} column="created_at">
                  Thời gian
                </SortableTh>
                <Th>Người dùng</Th>
                <Th>Hành động</Th>
                <Th>Đối tượng</Th>
              </tr>
            </thead>
            <tbody>
              {log.isLoading ? (
                <EmptyRow colSpan={4}>Đang tải…</EmptyRow>
              ) : log.data?.items.length ? (
                log.data.items.map((entry) => (
                  <tr key={entry.id}>
                    <Td>
                      <DateCell value={entry.created_at} />
                    </Td>
                    <Td className="whitespace-nowrap">
                      {entry.user_email ?? (
                        <code className="text-xs text-slate-500">{entry.user_id}</code>
                      )}
                    </Td>
                    <Td>
                      <Badge tone={ACTION_TONES[entry.action] ?? "neutral"}>{entry.action}</Badge>
                    </Td>
                    <Td>
                      <div>{entry.object_type}</div>
                      <code className="text-xs text-slate-500">{entry.object_id}</code>
                    </Td>
                  </tr>
                ))
              ) : (
                <EmptyRow colSpan={4}>Chưa có bản ghi nào.</EmptyRow>
              )}
            </tbody>
          </Table>

          <Pagination total={log.data?.total} paging={paging} unit="bản ghi" />
        </CardBody>
      </Card>
    </>
  );
}
