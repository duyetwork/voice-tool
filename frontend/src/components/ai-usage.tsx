"use client";

import * as React from "react";

import { ErrorNote } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Input, Select } from "@/components/ui/field";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import { useAIUsage, useSettings, useUpdateSettings } from "@/hooks/use-api";
import type { AIKind, AIPrice, AIUsageModel } from "@/types/api";

/**
 * AIUsageSection — nửa còn thiếu của "tối ưu chi phí AI".
 *
 * Màn Cài đặt vốn có hai cần gạt ảnh hưởng thẳng tới hoá đơn (chuỗi dự phòng
 * LLM, batch) nhưng không có con số nào để biết gạt xong rẻ hơn hay đắt hơn.
 * Phần này đo: token/ký tự đã tiêu theo từng model, quy ra tiền theo đơn giá
 * admin tự khai.
 *
 * Vì sao đơn giá phải tự khai mà không ghim sẵn trong code: giá của cả ba nhà
 * đổi vài lần một năm và khác nhau theo hợp đồng. Một bảng giá cũ không báo lỗi
 * — nó vẫn cho ra con số, chỉ là con số sai.
 */
export function AIUsageSection() {
  const [days, setDays] = React.useState(30);
  const report = useAIUsage(days);
  const data = report.data;

  return (
    <Card>
      <CardBody>
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">Chi phí AI</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Token và ký tự đã tiêu, theo từng model. Quy ra tiền theo đơn giá khai ở dưới — chưa
              khai thì vẫn đếm đủ, chỉ là không ra tiền.
            </p>
          </div>
          <Select value={days} onChange={(e) => setDays(Number(e.target.value))} className="w-36">
            <option value={7}>7 ngày qua</option>
            <option value={30}>30 ngày qua</option>
            <option value={90}>90 ngày qua</option>
          </Select>
        </div>

        <ErrorNote error={report.error} />

        {data && data.total_usd > 0 ? (
          <p className="mt-3 text-sm text-slate-700">
            Tổng đã quy ra tiền: <b>{money(data.total_usd)}</b>
            {data.missing_prices.length > 0 ? (
              <span className="text-slate-500">
                {" "}
                — chưa gồm {data.missing_prices.length} model chưa khai đơn giá.
              </span>
            ) : null}
          </p>
        ) : null}

        <div className="mt-4">
          <Table>
            <thead>
              <tr>
                <Th>Model</Th>
                <Th>Lượt gọi</Th>
                <Th>Lượng dùng</Th>
                <Th>Tiền</Th>
              </tr>
            </thead>
            <tbody>
              {report.isLoading ? (
                <EmptyRow colSpan={4}>Đang tải…</EmptyRow>
              ) : data?.models.length ? (
                data.models.map((row) => (
                  <tr key={`${row.kind}-${row.provider}-${row.model}`}>
                    <Td>
                      <div className="flex items-center gap-2">
                        <Badge tone="neutral">{KIND_LABELS[row.kind] ?? row.kind}</Badge>
                        <span className="text-xs text-slate-700">
                          {row.provider}
                          {row.model ? ` / ${row.model}` : ""}
                        </span>
                      </div>
                    </Td>
                    <Td className="text-sm">
                      {row.calls.toLocaleString("vi-VN")}
                      {row.failed > 0 ? (
                        <span className="ml-1 text-xs text-rose-600">({row.failed} lỗi)</span>
                      ) : null}
                    </Td>
                    <Td className="text-xs text-slate-600">{usageOf(row)}</Td>
                    <Td className="text-sm font-medium">
                      {row.has_price ? (
                        money(row.cost_usd)
                      ) : (
                        <span className="text-xs font-normal text-amber-600">chưa có đơn giá</span>
                      )}
                    </Td>
                  </tr>
                ))
              ) : (
                <EmptyRow colSpan={4}>
                  Chưa ghi nhận lần gọi AI nào trong khoảng này.
                </EmptyRow>
              )}
            </tbody>
          </Table>
        </div>

        <PriceEditor
          known={data?.prices ?? []}
          missing={data?.missing_prices ?? []}
          loading={report.isLoading}
        />
      </CardBody>
    </Card>
  );
}

const KIND_LABELS: Record<string, string> = {
  llm: "LLM",
  tts: "TTS",
  stt: "STT",
};

function money(usd: number) {
  // 4 chữ số thập phân: một voice lẻ tốn cỡ phần nghìn đô, làm tròn 2 số là ra
  // "$0.00" cho mọi dòng và bảng mất sạch ý nghĩa.
  return `$${usd.toFixed(usd >= 1 ? 2 : 4)}`;
}

function usageOf(row: AIUsageModel) {
  switch (row.kind) {
    case "llm":
      return `${row.input_tokens.toLocaleString("vi-VN")} vào / ${row.output_tokens.toLocaleString("vi-VN")} ra`;
    case "tts":
      return `${row.characters.toLocaleString("vi-VN")} ký tự`;
    case "stt":
      return `${Math.round(row.audio_seconds / 60).toLocaleString("vi-VN")} phút`;
    default:
      return "—";
  }
}

/**
 * PriceEditor — khai đơn giá cho đúng những model ĐANG được dùng.
 *
 * Danh sách dòng lấy từ chính dữ liệu đã đo (`missing`) cộng với các dòng đã
 * khai (`known`), nên admin không phải tự gõ tên nhà cung cấp và model — gõ tay
 * thì sai một ký tự là dòng giá không khớp với dòng usage nào, và không có gì
 * báo lỗi cả.
 */
function PriceEditor({
  known,
  missing,
  loading,
}: {
  known: AIPrice[];
  missing: AIPrice[];
  loading: boolean;
}) {
  const settings = useSettings();
  const save = useUpdateSettings();
  const [draft, setDraft] = React.useState<AIPrice[] | null>(null);

  // Gộp dòng đã khai với dòng đang thiếu, giữ thứ tự: đã khai trước, thiếu sau.
  const rows = React.useMemo(() => {
    const seen = new Set(known.map(keyOf));
    return [...known, ...missing.filter((p) => !seen.has(keyOf(p)))];
  }, [known, missing]);

  // Nạp 1 lần rồi form là của người dùng — nạp lại theo mỗi lần fetch sẽ xoá
  // con số họ đang gõ dở.
  React.useEffect(() => {
    if (draft === null && !loading) setDraft(rows);
  }, [draft, loading, rows]);

  if (draft === null) return null;

  function setCell(i: number, patch: Partial<AIPrice>) {
    setDraft((prev) => (prev ?? []).map((p, idx) => (idx === i ? { ...p, ...patch } : p)));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await save.mutateAsync({ ai_prices: draft ?? [] });
  }

  return (
    <form onSubmit={submit} className="mt-6 border-t border-slate-200 pt-4">
      <h3 className="text-sm font-semibold text-slate-900">Đơn giá</h3>
      <p className="mt-1 max-w-3xl text-sm text-slate-600">
        Khai theo hoá đơn của chính bạn. Đơn giá không nằm sẵn trong code vì nhà cung cấp đổi giá
        vài lần một năm — một con số ghim cứng sẽ sai mà không có gì báo.
      </p>

      {draft.length === 0 ? (
        <p className="mt-3 text-sm text-slate-500">
          Chưa có model nào được dùng — chạy thử một voice hình thức C rồi quay lại.
        </p>
      ) : (
        <div className="mt-3 space-y-2">
          {draft.map((p, i) => (
            <div key={keyOf(p)} className="flex flex-wrap items-center gap-2">
              <Badge tone="neutral">{KIND_LABELS[p.kind] ?? p.kind}</Badge>
              <span className="min-w-56 text-xs text-slate-700">
                {p.provider}
                {p.model ? ` / ${p.model}` : ""}
              </span>
              {p.kind === "llm" ? (
                <>
                  <PriceInput
                    label="USD / 1M token vào"
                    value={p.input_per_mtok}
                    onChange={(v) => setCell(i, { input_per_mtok: v })}
                  />
                  <PriceInput
                    label="USD / 1M token ra"
                    value={p.output_per_mtok}
                    onChange={(v) => setCell(i, { output_per_mtok: v })}
                  />
                </>
              ) : null}
              {p.kind === "tts" ? (
                <PriceInput
                  label="USD / 1M ký tự"
                  value={p.per_mchars}
                  onChange={(v) => setCell(i, { per_mchars: v })}
                />
              ) : null}
              {p.kind === "stt" ? (
                <PriceInput
                  label="USD / phút"
                  value={p.per_minute}
                  onChange={(v) => setCell(i, { per_minute: v })}
                />
              ) : null}
            </div>
          ))}
        </div>
      )}

      <ErrorNote error={save.error ?? settings.error} />

      {draft.length > 0 ? (
        <div className="mt-3 flex justify-end">
          <Button type="submit" disabled={save.isPending}>
            {save.isPending ? "Đang lưu…" : "Lưu đơn giá"}
          </Button>
        </div>
      ) : null}
    </form>
  );
}

function PriceInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <label className="flex items-center gap-1.5 text-xs text-slate-500">
      {label}
      <Input
        type="number"
        min={0}
        step="0.000001"
        className="h-8 w-28 text-xs"
        value={value}
        onChange={(e) => onChange(Number(e.target.value) || 0)}
      />
    </label>
  );
}

function keyOf(p: { kind: AIKind; provider: string; model: string }) {
  return `${p.kind}|${p.provider.toLowerCase()}|${p.model}`;
}
