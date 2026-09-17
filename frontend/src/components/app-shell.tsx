"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import * as React from "react";

import { usePermissions } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { useHealth } from "@/hooks/use-api";
import { tokenStore } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { User } from "@/types/api";

const NAV = [
  {
    group: "Danh sách kênh",
    items: [
      { href: "/lists/breaking", label: "Danh sách kênh Breaking" },
      { href: "/lists/scheduled", label: "Danh sách kênh Định kỳ" },
    ],
  },
  {
    group: "Quy trình",
    items: [
      { href: "/source-posts", label: "Bài Post" },
      { href: "/voices", label: "Voice" },
    ],
  },
  {
    group: "Danh mục",
    items: [
      { href: "/prompts", label: "Prompt mẫu" },
      { href: "/ai-engines", label: "AI Engine" },
    ],
  },
  {
    group: "Vận hành",
    items: [
      { href: "/audit-log", label: "Nhật ký thao tác" },
      { href: "/users", label: "Tài khoản", adminOnly: true },
      // Chuỗi dự phòng LLM + batch: cấu hình ảnh hưởng hạn mức và chi phí của
      // cả hệ thống, nên chỉ admin.
      { href: "/settings", label: "Cài đặt", adminOnly: true },
    ],
  },
  {
    group: "Trợ giúp",
    items: [{ href: "/huong-dan", label: "Hướng dẫn sử dụng" }],
  },
];

/** SIDEBAR_KEY nhớ trạng thái thu gọn của sidebar theo từng trình duyệt. */
const SIDEBAR_KEY = "voice-tool.sidebar_collapsed";

/** AppShell là layout dashboard + chốt đăng nhập cho mọi trang bên trong. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [user, setUser] = React.useState<User | null>(null);
  const [checked, setChecked] = React.useState(false);
  // Nhớ trạng thái thu gọn giữa các lần mở: người làm trên màn hẹp thường muốn
  // sidebar đóng luôn, bắt họ bấm lại mỗi lần tải trang là phiền.
  const [collapsed, setCollapsed] = React.useState(false);
  const { role, perms } = usePermissions();

  React.useEffect(() => {
    try {
      setCollapsed(window.localStorage.getItem(SIDEBAR_KEY) === "1");
    } catch {
      // Trình duyệt chặn localStorage (chế độ riêng tư) — mặc định mở.
    }
  }, []);

  function toggleSidebar() {
    setCollapsed((value) => {
      try {
        window.localStorage.setItem(SIDEBAR_KEY, value ? "0" : "1");
      } catch {
        // Không lưu được thì thôi, lần sau mở lại như mặc định.
      }
      return !value;
    });
  }

  React.useEffect(() => {
    if (!tokenStore.access()) {
      router.replace("/login");
      return;
    }
    setUser(tokenStore.user());
    setChecked(true);
  }, [router]);

  if (!checked) {
    return <div className="p-10 text-sm text-slate-500">Đang kiểm tra đăng nhập…</div>;
  }

  return (
    // h-screen + overflow-hidden ở khung ngoài: sidebar và header đứng yên, chỉ
    // <main> cuộn. Để cả trang cuộn thì bảng Voice dài vài chục dòng là menu
    // trôi mất, muốn đổi trang phải cuộn ngược lên đầu.
    <div className="flex h-screen overflow-hidden">
      <aside
        className={cn(
          "flex shrink-0 flex-col border-r border-slate-200 bg-white transition-[width] duration-200",
          collapsed ? "w-14" : "w-64",
        )}
      >
        <div
          className={cn(
            "flex items-center gap-2 border-b border-slate-200 py-4",
            collapsed ? "justify-center px-2" : "px-5",
          )}
        >
          {collapsed ? null : (
            <div className="min-w-0 flex-1">
              <p className="truncate text-lg font-semibold text-slate-900">Voice Automation</p>
              <p className="truncate text-xs text-slate-500">Danh sách → Bài Post → Voice</p>
            </div>
          )}
          <button
            type="button"
            onClick={toggleSidebar}
            title={collapsed ? "Mở rộng menu" : "Thu gọn menu"}
            aria-label={collapsed ? "Mở rộng menu" : "Thu gọn menu"}
            className="shrink-0 rounded-md border border-slate-300 px-2 py-1 text-sm text-slate-600 hover:border-slate-400 hover:text-slate-900"
          >
            {collapsed ? "»" : "«"}
          </button>
        </div>

        {/* Menu dài hơn màn hình thì tự cuộn TRONG sidebar, không kéo theo cả trang. */}
        <nav className={cn("flex-1 overflow-y-auto py-4", collapsed ? "px-1" : "px-3")}>
          {NAV.map((section) => (
            <div key={section.group} className="mb-5">
              {collapsed ? (
                // Thu gọn thì chỉ còn vạch ngăn giữa các nhóm: chữ nhóm không
                // lọt vào 56px mà cắt cụt thì đọc thành chữ vô nghĩa.
                <div className="mx-2 mb-2 border-t border-slate-200" />
              ) : (
                <p className="px-2 pb-1.5 text-xs font-semibold uppercase tracking-wide text-slate-400">
                  {section.group}
                </p>
              )}
              {section.items.map((item) =>
                "adminOnly" in item && item.adminOnly && !perms.can_manage_users ? null : (
                  <Link
                    key={item.href}
                    href={item.href}
                    title={item.label}
                    className={cn(
                      "block rounded-md px-2 py-1.5 text-sm",
                      collapsed && "truncate text-center",
                      pathname === item.href
                        ? "bg-indigo-50 font-medium text-indigo-800"
                        : "text-slate-700 hover:bg-slate-100",
                    )}
                  >
                    {/* Thu gọn: 2 chữ cái đầu là đủ để nhận ra mục đang ở, còn
                        tên đầy đủ nằm ở tooltip. */}
                    {collapsed ? item.label.slice(0, 2) : item.label}
                  </Link>
                ),
              )}
            </div>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex h-14 shrink-0 items-center justify-end gap-3 border-b border-slate-200 bg-white px-6">
          <span className="text-sm text-slate-600">{user?.email}</span>
          <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
            {role}
          </span>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              tokenStore.clear();
              router.replace("/login");
            }}
          >
            Đăng xuất
          </Button>
        </header>

        <main className="min-w-0 flex-1 overflow-y-auto p-6">
          <HealthBanner />
          {children}
        </main>
      </div>
    </div>
  );
}

/**
 * HealthBanner — ba thứ hỏng mà không có gì hiện ra trên màn hình.
 *
 * Nặng nhất là token multime của người tạo kênh hết hạn: khi đó MỌI auto-publish
 * của các kênh họ tạo đều fail, Voice nằm lại ở trạng thái lỗi, mà bảng kênh vẫn
 * hiện "Đang bật" như bình thường. Không ai đi tìm sự cố mình không biết là có,
 * nên nó phải tự hiện ra ở mọi trang.
 *
 * Cố ý KHÔNG có nút tắt: tắt được thì thứ đầu tiên người ta làm là tắt nó đi.
 * Muốn hết banner thì phải xử lý hết voice lỗi — đó chính là việc cần làm.
 */
function HealthBanner() {
  const health = useHealth();
  const data = health.data;

  // Đọc hỏng thì im lặng: không biến một lỗi phụ thành cảnh báo đỏ giả.
  if (!data || data.ok) return null;

  const parts: string[] = [];
  if (data.users_need_relogin > 0) {
    parts.push(
      `${data.users_need_relogin} người tạo kênh đã hết hạn đăng nhập multime — ` +
        "kênh của họ không tự đăng được, chính họ phải đăng nhập lại",
    );
  }
  if (data.channels_with_error > 0) {
    parts.push(`${data.channels_with_error} kênh đang bật nhưng vòng quét gần nhất lỗi`);
  }
  if (data.failed_voices > 0) {
    parts.push(`${data.failed_voices} voice đang ở trạng thái lỗi`);
  }

  return (
    <div className="mb-4 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
      <ul className="list-inside list-disc space-y-0.5">
        {parts.map((text) => (
          <li key={text}>{text}</li>
        ))}
      </ul>
      {/* Mỗi lối tắt chỉ hiện khi có đúng thứ nó dẫn tới. Hiện cả hai bất kể
          cảnh báo nào đang bật thì "Xem kênh" xuất hiện cạnh một dòng nói về
          voice — người đọc bấm vào rồi không thấy gì, và lần sau không tin
          banner nữa. */}
      <div className="mt-2 flex gap-3 text-xs">
        {data.failed_voices > 0 ? (
          <Link href="/voices?publish_status=failed" className="font-medium underline">
            Xem voice lỗi
          </Link>
        ) : null}
        {data.channels_with_error > 0 || data.users_need_relogin > 0 ? (
          <Link href="/lists/breaking" className="font-medium underline">
            Xem kênh
          </Link>
        ) : null}
      </div>
    </div>
  );
}
