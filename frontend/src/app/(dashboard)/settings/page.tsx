"use client";

import * as React from "react";

import { AIUsageSection } from "@/components/ai-usage";
import { ErrorNote, PageHeader } from "@/components/page-header";
import { usePermissions } from "@/components/permission";
import { ScrapeInfraSection } from "@/components/scrape-infra";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Field, Input, Select, Toggle } from "@/components/ui/field";
import { DateCell, EmptyRow, Table, Td, Th } from "@/components/ui/table";
import { useFetchStats, useSettings, useUpdateSettings } from "@/hooks/use-api";
import { platformLabel } from "@/lib/utils";
import type { LLMBatchConfig, LLMChainStep, LLMProvider } from "@/types/api";

/**
 * Cài đặt — cấu hình ảnh hưởng HẠN MỨC VÀ CHI PHÍ của cả hệ thống, nên chỉ
 * admin vào được (backend chặn bằng middleware, đây chỉ là lớp hiển thị).
 *
 * Bốn tab, bốn mối quan tâm tách rời nhau:
 *
 *   1. LLM        — chuỗi dự phòng khi một nhà hết hạn mức, và cấu hình batch.
 *   2. Chi phí AI — token/ký tự đã tiêu, để biết mục trên có rẻ đi thật không.
 *   3. Via & Proxy— phiên đăng nhập + lối ra mạng để quét Facebook/X/Instagram.
 *   4. Bị chặn    — số lần nền tảng chặn ta, dữ liệu để quyết định mua proxy.
 */
/**
 * Bốn nhóm cài đặt, mỗi nhóm một tab.
 *
 * Trước đây cả bốn xếp dọc trên một trang: bảng chi phí AI và biểu đồ lượt quét
 * đều dài, nên thứ cần xem gần như luôn nằm dưới màn hình và phải cuộn đi tìm.
 * Tệ hơn, chúng thuộc bốn mối quan tâm khác hẳn nhau — không ai vào đây để xem
 * cả bốn cùng lúc.
 *
 * `description` đi kèm từng tab chứ không phải một câu chung ở đầu trang: câu
 * chung phải gộp bốn việc lại và cuối cùng không mô tả đúng việc nào.
 */
const TABS = [
  {
    id: "llm",
    label: "LLM",
    description: "Chuỗi dự phòng khi một nhà cung cấp hết hạn mức, và cấu hình gom batch.",
  },
  {
    id: "cost",
    label: "Chi phí AI",
    description: "Token và ký tự đã tiêu, theo model và theo người dùng.",
  },
  {
    id: "scrape",
    label: "Via & Proxy",
    description:
      "Phiên đăng nhập và lối ra mạng dùng để quét kênh Facebook / X / Instagram. " +
      "Via chết dần theo thời gian dùng — đây là chỗ để nhìn thấy nó và thay.",
  },
  {
    id: "blocked",
    label: "Bị chặn",
    description: "Số lần từng nền tảng chặn hệ thống, đếm theo ngày.",
  },
] as const;

type TabID = (typeof TABS)[number]["id"];

export default function SettingsPage() {
  const { role, loading } = usePermissions();
  const [tab, setTab] = React.useState<TabID>("llm");

  if (loading) {
    return <p className="text-sm text-slate-500">Đang tải…</p>;
  }
  if (role !== "admin") {
    return (
      <>
        <PageHeader title="Cài đặt" />
        <Card>
          <CardBody>
            <p className="text-sm text-slate-600">
              Mục này chỉ dành cho admin: chuỗi dự phòng và batch quyết định hạn mức và chi phí của
              cả hệ thống, không phải cấu hình của riêng ai.
            </p>
          </CardBody>
        </Card>
      </>
    );
  }

  const current = TABS.find((t) => t.id === tab) ?? TABS[0];

  return (
    <>
      <PageHeader title="Cài đặt" description={current.description} />

      <div className="mb-4 flex flex-wrap gap-1 rounded-lg bg-slate-100 p-1">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={
              "rounded-md px-3 py-1.5 text-sm font-medium transition " +
              (tab === t.id
                ? "bg-white text-slate-900 shadow-sm"
                : "text-slate-600 hover:text-slate-900")
            }
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Chỉ dựng tab đang mở: mỗi tab kéo dữ liệu của riêng nó, và ba tab ẩn
          mà vẫn gọi API là ba lượt gọi không ai nhìn — trong đó có hai bảng tự
          làm mới mỗi 30 giây. */}
      <div className="space-y-6">
        {tab === "llm" ? <LLMSection /> : null}
        {tab === "cost" ? <AIUsageSection /> : null}
        {tab === "scrape" ? <ScrapeInfraSection /> : null}
        {tab === "blocked" ? <FetchStatsSection /> : null}
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Chuỗi dự phòng + batch
// ---------------------------------------------------------------------------

function LLMSection() {
  const settings = useSettings();
  const save = useUpdateSettings();

  const [chain, setChain] = React.useState<LLMChainStep[] | null>(null);
  const [batch, setBatch] = React.useState<LLMBatchConfig | null>(null);

  // Nạp giá trị đang lưu 1 lần; sau đó form là của người dùng.
  React.useEffect(() => {
    if (settings.data && chain === null) {
      setChain(settings.data.llm_chain);
      setBatch(settings.data.llm_batch);
    }
  }, [settings.data, chain]);

  const allowed = settings.data?.allowed_models;
  const providers = settings.data?.providers ?? [];

  if (settings.isLoading || !chain || !batch || !allowed) {
    return (
      <Card>
        <CardBody>
          <ErrorNote error={settings.error} />
          {settings.isLoading ? <p className="text-sm text-slate-500">Đang tải…</p> : null}
        </CardBody>
      </Card>
    );
  }

  function setStep(i: number, patch: Partial<LLMChainStep>) {
    setChain((prev) =>
      (prev ?? []).map((s, idx) => {
        if (idx !== i) return s;
        const next = { ...s, ...patch };
        // Đổi nhà thì model cũ gần như chắc chắn không còn hợp lệ — nhảy về
        // model đầu tiên của nhà mới thay vì để một cặp sai chờ bị từ chối.
        if (patch.provider && patch.provider !== s.provider) {
          next.model = allowed![patch.provider]?.[0] ?? "";
        }
        return next;
      }),
    );
  }

  function move(i: number, delta: number) {
    const j = i + delta;
    if (j < 0 || j >= chain!.length) return;
    const next = [...chain!];
    [next[i], next[j]] = [next[j], next[i]];
    setChain(next);
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await save.mutateAsync({ llm_chain: chain!, llm_batch: batch! });
  }

  return (
    <Card>
      <CardBody>
        <form onSubmit={submit} className="space-y-6">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">Chuỗi dự phòng LLM</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Thứ tự thử khi tạo voice hình thức C: hết hạn mức mắt xích này thì tự chuyển xuống mắt
              xích dưới. Xếp <strong>rẻ trước, đắt sau</strong> — mắt xích cuối chỉ chạy khi mọi mắt
              xích trên đã cạn.
            </p>
            <p className="mt-1 max-w-3xl text-xs text-amber-800">
              Danh sách model chỉ có bản ghim ID đầy đủ, không có alias — cố ý. Alias{" "}
              <code>gpt-5.6</code> không trỏ về Luna mà trỏ về Sol, đắt hơn khoảng 25 lần, và vì mắt
              xích này chỉ chạy khi Gemini đã cạn nên rất lâu mới có ai nhận ra.
            </p>

            <div className="mt-3 space-y-2">
              {chain.map((step, i) => (
                <div key={i} className="flex items-center gap-2">
                  <span className="w-6 text-sm text-slate-500">{i + 1}.</span>
                  <Select
                    value={step.provider}
                    onChange={(e) => setStep(i, { provider: e.target.value as LLMProvider })}
                    className="w-40"
                  >
                    {providers.map((p) => (
                      <option key={p} value={p}>
                        {p}
                      </option>
                    ))}
                  </Select>
                  <Select
                    value={step.model}
                    onChange={(e) => setStep(i, { model: e.target.value })}
                    className="flex-1"
                  >
                    {(allowed[step.provider] ?? []).map((m) => (
                      <option key={m} value={m}>
                        {m}
                      </option>
                    ))}
                  </Select>
                  <Button
                    type="button"
                    size="sm"
                    variant="secondary"
                    onClick={() => move(i, -1)}
                    disabled={i === 0}
                  >
                    ↑
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="secondary"
                    onClick={() => move(i, 1)}
                    disabled={i === chain.length - 1}
                  >
                    ↓
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="danger"
                    onClick={() => setChain(chain.filter((_, idx) => idx !== i))}
                    disabled={chain.length <= 1}
                  >
                    Bỏ
                  </Button>
                </div>
              ))}
            </div>

            <Button
              type="button"
              size="sm"
              variant="secondary"
              className="mt-2"
              onClick={() => {
                const provider = providers[0];
                setChain([...chain, { provider, model: allowed[provider]?.[0] ?? "" }]);
              }}
            >
              Thêm mắt xích
            </Button>
          </div>

          <div className="border-t border-slate-200 pt-6">
            <h2 className="text-sm font-semibold text-slate-900">Batch</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Gộp nhiều mẩu text vào 1 request để giảm chi phí và số lần gọi. Chỉ có tác dụng ở
              luồng tự động (quét ra nhiều bài một lượt); tạo voice lẻ trên UI bản chất là lô 1 mẩu.
              Lô hỏng thì hệ thống tự hạ về gọi lẻ từng mẩu, không bỏ cả lô.
            </p>
            <p className="mt-1 max-w-3xl text-xs text-slate-500">
              Hai con số dưới đây là <strong>điểm khởi đầu phải đo lại</strong>, không phải hằng số
              đúng sẵn: chạy thử trên text thật rồi so chất lượng với chi phí.
            </p>

            <div className="mt-3 grid max-w-2xl grid-cols-1 gap-4 sm:grid-cols-4">
              <div className="sm:col-span-4">
                <Toggle
                  checked={batch.enabled}
                  onChange={(enabled) => setBatch({ ...batch, enabled })}
                  label={batch.enabled ? "Đang bật batch" : "Đang tắt batch"}
                />
              </div>
              <Field label="Số mẩu/lô" hint="Mặc định 5.">
                <Input
                  type="number"
                  min={1}
                  max={50}
                  value={batch.size}
                  onChange={(e) => setBatch({ ...batch, size: Number(e.target.value) })}
                />
              </Field>
              <Field label="Tối đa ký tự" hint="Mặc định 12000.">
                <Input
                  type="number"
                  min={500}
                  value={batch.max_chars}
                  onChange={(e) => setBatch({ ...batch, max_chars: Number(e.target.value) })}
                />
              </Field>
              <Field label="Chờ gom (ms)" hint="Mặc định 2000.">
                <Input
                  type="number"
                  min={0}
                  max={30000}
                  value={batch.wait_ms}
                  onChange={(e) => setBatch({ ...batch, wait_ms: Number(e.target.value) })}
                />
              </Field>
            </div>
          </div>

          <ErrorNote error={save.error} />

          <div className="flex items-center gap-3 border-t border-slate-200 pt-4">
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "Đang lưu…" : "Lưu cài đặt"}
            </Button>
            {save.isSuccess && !save.isPending ? (
              <span className="text-sm text-green-700">Đã lưu.</span>
            ) : null}
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Thống kê bị chặn
// ---------------------------------------------------------------------------

/**
 * KIND_LABELS nói luôn CÁCH XỬ LÝ, không chỉ tên lỗi: cả bảng này tồn tại để
 * trả lời một câu hỏi — có đáng mua proxy không — và câu trả lời khác nhau tuỳ
 * loại. Proxy chỉ giải quyết được chặn IP và rate-limit.
 */
const KIND_LABELS: Record<string, { label: string; tone: "danger" | "warning" | "neutral" }> = {
  bot_block: { label: "Chặn IP máy chủ — proxy giải quyết được", tone: "danger" },
  rate_limit: { label: "Quá giới hạn truy cập — giảm tần suất trước", tone: "warning" },
  login_required: { label: "Đòi đăng nhập — cần cookies, proxy không giúp", tone: "warning" },
  geo_blocked: { label: "Chặn theo khu vực", tone: "neutral" },
  unavailable: { label: "Bài đã xoá / riêng tư — không phải bị chặn", tone: "neutral" },
  timeout: { label: "Tải quá lâu", tone: "neutral" },
  other: { label: "Khác", tone: "neutral" },
};

function FetchStatsSection() {
  const [days, setDays] = React.useState(7);
  const stats = useFetchStats(days);
  const items = stats.data?.items ?? [];

  return (
    <Card>
      <CardBody>
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">Bị nền tảng chặn</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Đếm theo ngày, để quyết định có cần proxy hay không —{" "}
              <strong>đo trước, mua sau</strong>. Hệ thống đã tự giảm rủi ro bằng cách rải lệch giờ
              quét và chỉ cho mỗi nền tảng 1 lần tải cùng lúc; bảng này cho biết chừng đó đã đủ
              chưa.
            </p>
          </div>
          <Select value={days} onChange={(e) => setDays(Number(e.target.value))} className="w-36">
            <option value={7}>7 ngày qua</option>
            <option value={30}>30 ngày qua</option>
            <option value={90}>90 ngày qua</option>
          </Select>
        </div>

        <ErrorNote error={stats.error} />

        <div className="mt-4">
          <Table>
            <thead>
              <tr>
                <Th>Ngày</Th>
                <Th>Nền tảng</Th>
                <Th>Loại</Th>
                <Th>Số lần</Th>
                <Th>Gần nhất</Th>
              </tr>
            </thead>
            <tbody>
              {stats.isLoading ? (
                <EmptyRow colSpan={5}>Đang tải…</EmptyRow>
              ) : items.length ? (
                items.map((row) => {
                  const kind = KIND_LABELS[row.kind] ?? { label: row.kind, tone: "neutral" };
                  return (
                    <tr key={`${row.day}-${row.platform}-${row.kind}`}>
                      <Td className="text-xs">{row.day}</Td>
                      <Td className="text-xs">{platformLabel(row.platform)}</Td>
                      <Td>
                        <Badge tone={kind.tone}>{kind.label}</Badge>
                      </Td>
                      <Td className="text-sm font-medium">{row.count}</Td>
                      <Td>
                        <DateCell value={row.last_at} />
                      </Td>
                    </tr>
                  );
                })
              ) : (
                <EmptyRow colSpan={5}>
                  Chưa ghi nhận lần nào bị chặn — chưa có lý do gì để mua proxy.
                </EmptyRow>
              )}
            </tbody>
          </Table>
        </div>
      </CardBody>
    </Card>
  );
}
