"use client";

import * as React from "react";

import { ErrorNote } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Field, Input, Select } from "@/components/ui/field";
import { Modal } from "@/components/ui/modal";
import { EmptyRow, Table, Td, Th } from "@/components/ui/table";
import {
  useCreateProxy,
  useCreateVia,
  useDeleteProxy,
  useDeleteVia,
  useProxies,
  useScrapeHealth,
  useScrapeLoad,
  useUpdateProxy,
  useUpdateVia,
  useViaCookieSpecs,
  useVias,
} from "@/hooks/use-api";
import { formatDateTime, platformLabel } from "@/lib/utils";
import type { ProxyStatus, ScrapeProxy, Via, ViaHealth, ViaStatus } from "@/types/api";

/**
 * Quản lý via + proxy dùng để quét kênh Facebook / X / Instagram.
 *
 * Ba nền tảng này không liệt kê được bài của một trang qua yt-dlp, và kênh nguồn
 * là trang CỦA NGƯỜI KHÁC nên không có API chính thức nào dùng được. Cách còn
 * lại là tự đọc bằng một phiên đăng nhập, đi qua proxy.
 *
 * Màn này tồn tại vì cái đàn via đó là thứ SỐNG: via chết dần theo thời gian
 * dùng, và người vận hành phải nhìn thấy nó chết để thay. Quản lý qua file cấu
 * hình thì không ai nhìn thấy gì cho tới lúc mọi kênh cùng ngừng ra bài.
 */

/** Nền tảng cần via — đúng thứ tự ưu tiên đã chốt. */
const VIA_PLATFORMS = ["facebook", "x", "instagram"] as const;

const VIA_STATUS: Record<
  ViaStatus,
  { label: string; tone: "success" | "warning" | "danger" | "neutral" }
> = {
  active: { label: "Đang chạy", tone: "success" },
  cooldown: { label: "Đang nghỉ", tone: "warning" },
  dead: { label: "Đã chết", tone: "danger" },
  disabled: { label: "Đã tắt", tone: "neutral" },
};

const PROXY_STATUS: Record<
  ProxyStatus,
  { label: string; tone: "success" | "warning" | "danger" | "neutral" }
> = {
  active: { label: "Đang chạy", tone: "success" },
  degraded: { label: "Hay bị chặn", tone: "warning" },
  dead: { label: "Đã chết", tone: "danger" },
  disabled: { label: "Đã tắt", tone: "neutral" },
};

const PROXY_KIND: Record<string, string> = {
  residential: "Residential",
  mobile: "Mobile",
  datacenter: "Datacenter",
};

/** Dưới ngưỡng này thì banner đỏ: đàn via sắp không đủ để quét hết kênh. */
const HEALTHY_FLOOR = 0.3;

export function ScrapeInfraSection() {
  return (
    <>
      <ViaHealthCard />
      <ViaListCard />
      <ProxyListCard />
      <HourlyLoadCard />
    </>
  );
}

// ---------------------------------------------------------------------------
// Tổng quan
// ---------------------------------------------------------------------------

function ViaHealthCard() {
  const health = useScrapeHealth();
  const items = health.data?.items ?? [];
  // Chỉ cảnh báo những nền tảng ĐÃ có via: "chưa thêm via cho Instagram" không
  // phải sự cố, đó là chưa dùng tới.
  const failing = items.filter((h) => h.total > 0 && h.healthy < HEALTHY_FLOOR);

  return (
    <Card>
      <CardBody>
        <h2 className="text-sm font-semibold text-slate-900">Sức khoẻ đàn via</h2>
        <p className="mt-1 max-w-3xl text-sm text-slate-600">
          Facebook, X và Instagram không cho liệt kê bài của một trang nếu không đăng nhập, nên việc
          quét chúng chạy bằng via (phiên đăng nhập) + proxy. Via <strong>chết dần</strong> theo
          thời gian dùng — đây là chi phí vận hành thường xuyên, không phải cài một lần.
        </p>

        <ErrorNote error={health.error} />

        {failing.length > 0 ? (
          <div className="mt-3 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-900">
            Sắp hết via cho <b>{failing.map((h) => platformLabel(h.platform)).join(", ")}</b> — dưới{" "}
            {Math.round(HEALTHY_FLOOR * 100)}% còn chạy được. Thêm via mới trước khi các kênh của
            nền tảng đó ngừng ra bài.
          </div>
        ) : null}

        <div className="mt-4 grid gap-3 sm:grid-cols-3">
          {items.map((h) => (
            <HealthTile key={h.platform} health={h} />
          ))}
        </div>
      </CardBody>
    </Card>
  );
}

function HealthTile({ health: h }: { health: ViaHealth }) {
  const pct = h.total > 0 ? Math.round(h.healthy * 100) : 0;
  const tone =
    h.total === 0
      ? "text-slate-400"
      : h.healthy < HEALTHY_FLOOR
        ? "text-red-700"
        : "text-green-700";

  return (
    <div className="rounded-lg border border-slate-200 p-3">
      <div className="text-xs font-medium text-slate-500">{platformLabel(h.platform)}</div>
      {h.total === 0 ? (
        <div className="mt-1 text-sm text-slate-400">Chưa có via nào</div>
      ) : (
        <>
          <div className={`mt-1 text-2xl font-semibold tabular-nums ${tone}`}>
            {h.active}
            <span className="text-base font-normal text-slate-400"> / {h.total}</span>
          </div>
          <div className="mt-1 text-xs text-slate-500">
            {pct}% chạy được
            {h.cooldown > 0 ? ` · ${h.cooldown} đang nghỉ` : ""}
            {h.dead > 0 ? ` · ${h.dead} đã chết` : ""}
            {h.disabled > 0 ? ` · ${h.disabled} đã tắt` : ""}
          </div>
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Via
// ---------------------------------------------------------------------------

function ViaListCard() {
  const [platform, setPlatform] = React.useState("");
  const [adding, setAdding] = React.useState(false);
  const [editing, setEditing] = React.useState<Via | null>(null);

  const vias = useVias(platform);
  const update = useUpdateVia();
  const remove = useDeleteVia();
  const items = vias.data?.items ?? [];

  return (
    <Card>
      <CardBody>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">Via (phiên đăng nhập)</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Cookies được mã hoá trước khi lưu và <strong>không bao giờ hiện lại</strong> ở đây.
              Via bị đòi đăng nhập nhiều lần liên tiếp sẽ tự vào trạng thái nghỉ; nghỉ xong vẫn hỏng
              thì hệ thống coi là đã chết và bạn cần dán cookies mới.
            </p>
          </div>
          <div className="flex items-end gap-2">
            <Select value={platform} onChange={(e) => setPlatform(e.target.value)} className="w-40">
              <option value="">Tất cả nền tảng</option>
              {VIA_PLATFORMS.map((p) => (
                <option key={p} value={p}>
                  {platformLabel(p)}
                </option>
              ))}
            </Select>
            <Button onClick={() => setAdding(true)}>+ Thêm via</Button>
          </div>
        </div>

        <ErrorNote error={vias.error ?? update.error ?? remove.error} />

        <div className="mt-4">
          <Table>
            <thead>
              <tr>
                <Th>Via</Th>
                <Th>Trạng thái</Th>
                <Th className="text-right">Còn lại hôm nay</Th>
                <Th className="text-right">Lỗi liên tiếp</Th>
                <Th>Dùng lần cuối</Th>
                <Th className="text-right">Thao tác</Th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 ? (
                <EmptyRow colSpan={6}>
                  Chưa có via nào. Kênh Facebook / X / Instagram sẽ không quét được cho tới khi có
                  ít nhất một via.
                </EmptyRow>
              ) : (
                items.map((via) => (
                  <ViaRow
                    key={via.id}
                    via={via}
                    busy={update.isPending || remove.isPending}
                    onEdit={() => setEditing(via)}
                    onToggle={() =>
                      update.mutate({
                        id: via.id,
                        status: via.status === "disabled" ? "active" : "disabled",
                      })
                    }
                    onDelete={() => {
                      if (confirm(`Xoá via "${via.label}"? Cookies của nó sẽ mất hẳn.`)) {
                        remove.mutate(via.id);
                      }
                    }}
                  />
                ))
              )}
            </tbody>
          </Table>
        </div>
      </CardBody>

      {adding ? <ViaDialog onClose={() => setAdding(false)} /> : null}
      {editing ? <ViaDialog via={editing} onClose={() => setEditing(null)} /> : null}
    </Card>
  );
}

function ViaRow({
  via,
  busy,
  onEdit,
  onToggle,
  onDelete,
}: {
  via: Via;
  busy: boolean;
  onEdit: () => void;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const st = VIA_STATUS[via.status] ?? { label: via.status, tone: "neutral" as const };
  return (
    <tr>
      <Td>
        <div className="font-medium text-slate-900">{via.label}</div>
        <div className="text-xs text-slate-500">{platformLabel(via.platform)}</div>
      </Td>
      <Td>
        <Badge tone={st.tone}>{st.label}</Badge>
        {/* Nghỉ tới lúc nào là thứ quyết định có cần thay via ngay hay chờ được. */}
        {via.status === "cooldown" && via.cooldown_until ? (
          <div className="mt-1 text-xs text-slate-500">
            thử lại: {formatDateTime(via.cooldown_until)}
          </div>
        ) : null}
        {via.last_error ? (
          <p className="mt-1 max-w-64 text-xs text-red-700">{via.last_error}</p>
        ) : null}
      </Td>
      <Td className="text-right tabular-nums">
        <span className={via.quota_left === 0 ? "text-amber-700" : "text-slate-900"}>
          {via.quota_left}
        </span>
        <span className="text-slate-400"> / {via.daily_quota}</span>
      </Td>
      <Td className="text-right tabular-nums text-slate-600">
        {via.consecutive_login_errors || ""}
      </Td>
      <Td className="whitespace-nowrap text-xs text-slate-500">
        {via.last_used_at ? formatDateTime(via.last_used_at) : "chưa dùng"}
      </Td>
      <Td className="whitespace-nowrap text-right">
        <div className="flex justify-end gap-2">
          <Button size="sm" variant="secondary" onClick={onEdit}>
            Sửa
          </Button>
          <Button size="sm" variant="secondary" disabled={busy} onClick={onToggle}>
            {via.status === "disabled" ? "Bật" : "Tắt"}
          </Button>
          <Button size="sm" variant="danger" disabled={busy} onClick={onDelete}>
            Xoá
          </Button>
        </div>
      </Td>
    </tr>
  );
}

function ViaDialog({ via, onClose }: { via?: Via; onClose: () => void }) {
  const editing = via != null;
  const create = useCreateVia();
  const update = useUpdateVia();

  const [platform, setPlatform] = React.useState(via?.platform ?? "facebook");
  const [label, setLabel] = React.useState(via?.label ?? "");
  const [cookies, setCookies] = React.useState("");
  const [quota, setQuota] = React.useState(String(via?.daily_quota ?? 50));
  const pending = create.isPending || update.isPending;

  // Mỗi nền tảng cần một bộ cookie khác nhau, và người dán không có cách nào
  // đoán ra bộ nào. Lấy từ server vì chính server là bên từ chối khi thiếu.
  const specs = useViaCookieSpecs();
  const spec = specs.data?.items.find((s) => s.platform === platform);

  // Báo thiếu NGAY TRONG FORM, không đợi bấm Lưu: dán cookies là thao tác dài,
  // và biết mình dán thiếu sau khi đã đóng DevTools là phải mở lại từ đầu.
  const missing = React.useMemo(() => {
    const text = cookies.trim();
    if (!text || !spec) return [];
    const names = new Set(
      text
        .split(";")
        .map((part) => part.split("=")[0]?.trim())
        .filter(Boolean),
    );
    return spec.required.filter((name) => !names.has(name));
  }, [cookies, spec]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const body = {
      label: label.trim(),
      cookies: cookies.trim(),
      daily_quota: Number(quota) || undefined,
    };
    if (via) {
      await update.mutateAsync({ id: via.id, ...body });
    } else {
      await create.mutateAsync({ platform, ...body });
    }
    onClose();
  }

  return (
    <Modal
      title={editing ? `Sửa via "${via.label}"` : "Thêm via"}
      description="Cookies được mã hoá trước khi lưu và không bao giờ hiển thị lại."
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        {!editing ? (
          <Field
            label="Nền tảng"
            required
            hint="Chỉ ba nền tảng này cần via — YouTube và TikTok liệt kê được kênh mà không cần đăng nhập."
          >
            <Select value={platform} onChange={(e) => setPlatform(e.target.value)}>
              {VIA_PLATFORMS.map((p) => (
                <option key={p} value={p}>
                  {platformLabel(p)}
                </option>
              ))}
            </Select>
          </Field>
        ) : null}

        <Field
          label="Tên gợi nhớ"
          required
          hint="Chỉ để bạn nhận ra via nào là via nào. ĐỪNG ghi email hay mật khẩu vào đây — tên này hiện khắp giao diện và đi vào nhật ký."
        >
          <Input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            required
            maxLength={100}
          />
        </Field>

        <Field
          label={editing ? "Cookies mới (bỏ trống = giữ nguyên)" : "Cookies"}
          required={!editing}
          error={
            missing.length > 0
              ? `Còn thiếu ${missing.join(", ")} — thiếu cookie này thì ${platformLabel(platform)} coi như chưa đăng nhập.`
              : undefined
          }
          hint={
            editing
              ? "Dán cookies mới để cứu một via đã chết — việc đó cũng đặt lại trạng thái về đang chạy."
              : (spec?.hint ??
                "Dán chuỗi cookie của phiên đã đăng nhập, ngăn cách bằng dấu chấm phẩy.")
          }
        >
          <textarea
            className="min-h-24 w-full rounded-md border border-slate-300 px-3 py-2 font-mono text-xs"
            value={cookies}
            onChange={(e) => setCookies(e.target.value)}
            required={!editing}
            placeholder={spec?.example}
          />
        </Field>

        {/* Bắt buộc vs tuỳ chọn: chép dư cookie thì vô hại, chép thiếu đúng
            một cái trong danh sách này là via không dùng được. */}
        {spec ? (
          <p className="-mt-2 text-xs text-slate-500">
            Bắt buộc phải có:{" "}
            {spec.required.map((name) => (
              <code key={name} className="mr-1 rounded bg-slate-100 px-1 py-0.5">
                {name}
              </code>
            ))}
            . Các cookie khác chép kèm cũng được, không bắt buộc.
          </p>
        ) : null}

        <Field
          label="Hạn mức lượt quét / ngày"
          hint="1 lượt = quét xong 1 kênh. Số nhỏ buộc phải nuôi nhiều via thay vì vắt kiệt một cái — đó là thứ giữ cho cả đàn sống lâu."
        >
          <Input
            type="number"
            min={1}
            max={10000}
            value={quota}
            onChange={(e) => setQuota(e.target.value)}
          />
        </Field>

        <ErrorNote error={create.error ?? update.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu thay đổi" : "Thêm via"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Proxy
// ---------------------------------------------------------------------------

function ProxyListCard() {
  const [adding, setAdding] = React.useState(false);
  const [editing, setEditing] = React.useState<ScrapeProxy | null>(null);

  const proxies = useProxies();
  const update = useUpdateProxy();
  const remove = useDeleteProxy();
  const items = proxies.data?.items ?? [];

  return (
    <Card>
      <CardBody>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">Proxy</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Proxy chữa lỗi <b>bị chặn IP</b>, còn via chữa lỗi <b>bị đòi đăng nhập</b> — hai bệnh
              khác nhau. Proxy bị chặn nhiều lần liên tiếp sẽ ra khỏi vòng chọn và{" "}
              <strong>không tự bật lại</strong>: một IP đã bị liệt thì chờ cũng không khỏi.
            </p>
          </div>
          <Button onClick={() => setAdding(true)}>+ Thêm proxy</Button>
        </div>

        <ErrorNote error={proxies.error ?? update.error ?? remove.error} />

        <div className="mt-4">
          <Table>
            <thead>
              <tr>
                <Th>Proxy</Th>
                <Th>Loại</Th>
                <Th>Trạng thái</Th>
                <Th className="text-right">Hôm nay</Th>
                <Th className="text-right">Thao tác</Th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 ? (
                <EmptyRow colSpan={5}>
                  Chưa có proxy nào — request sẽ đi thẳng bằng IP máy chủ. Vẫn chạy được, nhưng khả
                  năng bị chặn cao hơn hẳn.
                </EmptyRow>
              ) : (
                items.map((proxy) => (
                  <ProxyRow
                    key={proxy.id}
                    proxy={proxy}
                    busy={update.isPending || remove.isPending}
                    onEdit={() => setEditing(proxy)}
                    onToggle={() =>
                      update.mutate({
                        id: proxy.id,
                        status: proxy.status === "active" ? "disabled" : "active",
                      })
                    }
                    onDelete={() => {
                      if (confirm(`Xoá proxy "${proxy.label}"?`)) remove.mutate(proxy.id);
                    }}
                  />
                ))
              )}
            </tbody>
          </Table>
        </div>
      </CardBody>

      {adding ? <ProxyDialog onClose={() => setAdding(false)} /> : null}
      {editing ? <ProxyDialog proxy={editing} onClose={() => setEditing(null)} /> : null}
    </Card>
  );
}

function ProxyRow({
  proxy,
  busy,
  onEdit,
  onToggle,
  onDelete,
}: {
  proxy: ScrapeProxy;
  busy: boolean;
  onEdit: () => void;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const st = PROXY_STATUS[proxy.status] ?? { label: proxy.status, tone: "neutral" as const };
  const errRate =
    proxy.used_today > 0 ? Math.round((proxy.errors_today / proxy.used_today) * 100) : 0;

  return (
    <tr>
      <Td>
        <div className="font-medium text-slate-900">{proxy.label}</div>
        {/* Endpoint đã bị server cắt user/pass trước khi rời khỏi DB. */}
        <div className="font-mono text-xs text-slate-500">{proxy.endpoint}</div>
        <div className="text-xs text-slate-400">
          {proxy.platform ? platformLabel(proxy.platform) : "dùng chung mọi nền tảng"}
        </div>
      </Td>
      <Td className="whitespace-nowrap text-xs">
        {PROXY_KIND[proxy.kind] ?? proxy.kind}
        {/* Nói thẳng ngay tại dòng đó, không giấu trong tài liệu: đây là nguyên
            nhân phổ biến nhất khiến proxy chết ngay sau khi mua. */}
        {proxy.kind === "datacenter" ? (
          <div className="text-amber-700">Meta/X nhận ra dải này rất nhanh</div>
        ) : null}
      </Td>
      <Td>
        <Badge tone={st.tone}>{st.label}</Badge>
        {proxy.status === "degraded" || proxy.status === "dead" ? (
          <div className="mt-1 text-xs text-slate-500">
            {proxy.consecutive_blocks} lần bị chặn liên tiếp
          </div>
        ) : null}
        {proxy.last_error ? (
          <p className="mt-1 max-w-56 text-xs text-red-700">{proxy.last_error}</p>
        ) : null}
      </Td>
      <Td className="text-right text-xs tabular-nums">
        <div className="text-slate-900">{proxy.used_today} lượt</div>
        {proxy.errors_today > 0 ? (
          <div className="text-red-700">
            {proxy.errors_today} lỗi ({errRate}%)
          </div>
        ) : null}
      </Td>
      <Td className="whitespace-nowrap text-right">
        <div className="flex justify-end gap-2">
          <Button size="sm" variant="secondary" onClick={onEdit}>
            Sửa
          </Button>
          <Button size="sm" variant="secondary" disabled={busy} onClick={onToggle}>
            {proxy.status === "active" ? "Tắt" : "Bật lại"}
          </Button>
          <Button size="sm" variant="danger" disabled={busy} onClick={onDelete}>
            Xoá
          </Button>
        </div>
      </Td>
    </tr>
  );
}

function ProxyDialog({ proxy, onClose }: { proxy?: ScrapeProxy; onClose: () => void }) {
  const editing = proxy != null;
  const create = useCreateProxy();
  const update = useUpdateProxy();

  const [label, setLabel] = React.useState(proxy?.label ?? "");
  const [platform, setPlatform] = React.useState(proxy?.platform ?? "");
  const [endpoint, setEndpoint] = React.useState("");
  const [kind, setKind] = React.useState(proxy?.kind ?? "residential");
  const pending = create.isPending || update.isPending;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const body = { label: label.trim(), endpoint: endpoint.trim(), kind };
    if (proxy) {
      await update.mutateAsync({ id: proxy.id, ...body });
    } else {
      await create.mutateAsync({ platform, ...body });
    }
    onClose();
  }

  return (
    <Modal
      title={editing ? `Sửa proxy "${proxy.label}"` : "Thêm proxy"}
      description="Endpoint được mã hoá trước khi lưu; bảng chỉ hiện phần đã cắt user/pass."
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="Tên gợi nhớ" required>
          <Input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            required
            maxLength={100}
          />
        </Field>

        <Field
          label={editing ? "Endpoint mới (bỏ trống = giữ nguyên)" : "Endpoint"}
          required={!editing}
          hint="Dán thẳng dòng nhà bán proxy giao cho bạn — không cần tự ghép thành URL."
        >
          <Input
            value={endpoint}
            onChange={(e) => setEndpoint(e.target.value)}
            required={!editing}
            placeholder="23.95.45.5:10356:u3qvdrto:9w6w0mc8c5"
          />
        </Field>

        {/* Nói ra cả ba dạng: dạng người dùng sẵn có (nhà bán proxy giao) lại
            là dạng ít ai nghĩ là hợp lệ, nên họ hay ngồi ghép tay và ghép sai. */}
        <ul className="-mt-2 space-y-0.5 text-xs text-slate-500">
          <li>
            <code className="rounded bg-slate-100 px-1 py-0.5">ip:port:user:pass</code> — dạng các
            nhà bán proxy hay giao, dán nguyên dòng
          </li>
          <li>
            <code className="rounded bg-slate-100 px-1 py-0.5">ip:port</code> — proxy không cần đăng
            nhập
          </li>
          <li>
            <code className="rounded bg-slate-100 px-1 py-0.5">socks5://user:pass@ip:port</code> —
            khai scheme khi KHÔNG phải http
          </li>
        </ul>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            label="Loại"
            hint="Datacenter rẻ nhất nhưng Meta/X nhận ra gần như ngay — không nên dùng cho ba nền tảng này."
          >
            <Select value={kind} onChange={(e) => setKind(e.target.value as ScrapeProxy["kind"])}>
              <option value="residential">Residential</option>
              <option value="mobile">Mobile</option>
              <option value="datacenter">Datacenter</option>
            </Select>
          </Field>

          {!editing ? (
            <Field
              label="Chỉ dùng cho nền tảng"
              hint="Bỏ trống = dùng chung, đúng với gateway residential xoay IP theo request."
            >
              <Select value={platform} onChange={(e) => setPlatform(e.target.value)}>
                <option value="">Dùng chung mọi nền tảng</option>
                {VIA_PLATFORMS.map((p) => (
                  <option key={p} value={p}>
                    {platformLabel(p)}
                  </option>
                ))}
              </Select>
            </Field>
          ) : null}
        </div>

        <ErrorNote error={create.error ?? update.error} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Huỷ
          </Button>
          <Button type="submit" disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu thay đổi" : "Thêm proxy"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Phân bổ lượt quét theo giờ
// ---------------------------------------------------------------------------

/**
 * Biểu đồ này trả lời đúng một câu: lịch quét có bị dồn cục không.
 *
 * Vài trăm kênh quét 1 lần/ngày mà tất cả rơi vào cùng một khung giờ thì nền
 * tảng nhìn thấy một đợt tấn công. Bảng "bị chặn" chỉ nói RẰNG bị chặn; cột giờ
 * này mới nói VÌ SAO.
 */
function HourlyLoadCard() {
  const [days, setDays] = React.useState(7);
  const load = useScrapeLoad(days);
  const rows = load.data?.items;

  // Gộp các nền tảng lại theo giờ: câu hỏi ở đây là về NHỊP của cả hệ thống,
  // không phải của từng nền tảng.
  const byHour = React.useMemo(() => {
    const out = Array.from({ length: 24 }, (_, hour) => ({ hour, total: 0, failed: 0 }));
    for (const r of rows ?? []) {
      const slot = out[r.hour];
      if (slot) {
        slot.total += r.total;
        slot.failed += r.failed;
      }
    }
    return out;
  }, [rows]);

  const peak = Math.max(1, ...byHour.map((h) => h.total));
  const total = byHour.reduce((sum, h) => sum + h.total, 0);

  return (
    <Card>
      <CardBody>
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-sm font-semibold text-slate-900">Lượt quét theo giờ</h2>
            <p className="mt-1 max-w-3xl text-sm text-slate-600">
              Kiểm tra lịch quét có bị dồn cục không. Cột cao chót vót ở một giờ nghĩa là hàng loạt
              kênh cùng chạy một lúc — nền tảng nhìn thấy điều đó rõ hơn bất kỳ thứ gì khác.
            </p>
          </div>
          <Select value={days} onChange={(e) => setDays(Number(e.target.value))} className="w-36">
            <option value={1}>Hôm nay</option>
            <option value={7}>7 ngày qua</option>
            <option value={30}>30 ngày qua</option>
          </Select>
        </div>

        <ErrorNote error={load.error} />

        {total === 0 ? (
          <p className="mt-4 text-sm text-slate-500">
            Chưa có lượt quét nào đi qua via trong khoảng này.
          </p>
        ) : (
          <div className="mt-4">
            <div className="flex h-32 items-end gap-1">
              {byHour.map((h) => (
                <div key={h.hour} className="flex flex-1 flex-col items-center gap-1">
                  <div
                    className="flex w-full flex-col justify-end"
                    style={{ height: `${Math.round((h.total / peak) * 100)}%` }}
                    title={`${h.hour}h — ${h.total} lượt, ${h.failed} lỗi`}
                  >
                    {/* Phần lỗi nằm trên cùng cột: tỉ lệ hỏng theo giờ là thứ
                        chỉ ra khung giờ nào đang bị nền tảng để ý. */}
                    {h.failed > 0 ? (
                      <div
                        className="w-full rounded-t bg-red-400"
                        style={{ height: `${Math.round((h.failed / h.total) * 100)}%` }}
                      />
                    ) : null}
                    <div className="w-full flex-1 bg-indigo-400" />
                  </div>
                </div>
              ))}
            </div>
            <div className="mt-1 flex gap-1 text-[10px] text-slate-400">
              {byHour.map((h) => (
                <div key={h.hour} className="flex-1 text-center">
                  {h.hour % 3 === 0 ? h.hour : ""}
                </div>
              ))}
            </div>
            <p className="mt-2 text-xs text-slate-500">
              {total} lượt quét · cao nhất {peak} lượt trong một giờ · phần đỏ là lượt hỏng
            </p>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
