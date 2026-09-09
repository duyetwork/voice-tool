"use client";

import * as React from "react";

import { ErrorNote, PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Select } from "@/components/ui/field";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import { useAuditLog } from "@/hooks/use-api";
import { formatDateTime } from "@/lib/utils";

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
  const log = useAuditLog({ object_type: objectType || undefined });

  return (
    <>
      <PageHeader
        title="Nhật ký thao tác"
        description="Ghi tự động mọi create/update/delete/run/publish trên Danh sách, Bài Post và Voice. Append-only."
      />

      <Card>
        <CardBody className="flex flex-wrap items-end gap-3 border-b border-slate-200">
          <div className="w-56">
            <Select value={objectType} onChange={(e) => setObjectType(e.target.value)}>
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
        </CardBody>

        <CardBody className="p-0">
          <ErrorNote error={log.error} />
          <Table>
            <thead>
              <tr>
                <Th>Thời gian</Th>
                <Th>Hành động</Th>
                <Th>Đối tượng</Th>
                <Th>Thay đổi</Th>
              </tr>
            </thead>
            <tbody>
              {log.isLoading ? (
                <EmptyRow colSpan={4}>Đang tải…</EmptyRow>
              ) : log.data?.items.length ? (
                log.data.items.map((entry) => (
                  <tr key={entry.id}>
                    <Td className="whitespace-nowrap">{formatDateTime(entry.created_at)}</Td>
                    <Td>
                      <Badge tone={ACTION_TONES[entry.action] ?? "neutral"}>{entry.action}</Badge>
                    </Td>
                    <Td>
                      <div>{entry.object_type}</div>
                      <code className="text-xs text-slate-500">{entry.object_id}</code>
                    </Td>
                    <Td className="max-w-lg">
                      {entry.changes ? (
                        <pre className="overflow-x-auto rounded bg-slate-50 p-2 text-xs text-slate-600">
                          {JSON.stringify(entry.changes, null, 2)}
                        </pre>
                      ) : (
                        "—"
                      )}
                    </Td>
                  </tr>
                ))
              ) : (
                <EmptyRow colSpan={4}>Chưa có bản ghi nào.</EmptyRow>
              )}
            </tbody>
          </Table>
        </CardBody>
      </Card>
    </>
  );
}
