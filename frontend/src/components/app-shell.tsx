"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import * as React from "react";

import { usePermissions } from "@/components/permission";
import { Button } from "@/components/ui/button";
import { tokenStore } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { User } from "@/types/api";

const NAV = [
  {
    group: "Tạo voice",
    items: [{ href: "/on-demand", label: "Tạo voice" }],
  },
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
    ],
  },
];

/** AppShell là layout dashboard + chốt đăng nhập cho mọi trang bên trong. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [user, setUser] = React.useState<User | null>(null);
  const [checked, setChecked] = React.useState(false);
  const { role, perms } = usePermissions();

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
    <div className="flex min-h-screen">
      <aside className="w-64 shrink-0 border-r border-slate-200 bg-white">
        <div className="border-b border-slate-200 px-5 py-4">
          <p className="text-lg font-semibold text-slate-900">Voice Automation</p>
          <p className="text-xs text-slate-500">Danh sách → Bài Post → Voice</p>
        </div>

        <nav className="px-3 py-4">
          {NAV.map((section) => (
            <div key={section.group} className="mb-5">
              <p className="px-2 pb-1.5 text-xs font-semibold uppercase tracking-wide text-slate-400">
                {section.group}
              </p>
              {section.items.map((item) =>
                "adminOnly" in item && item.adminOnly && !perms.can_manage_users ? null : (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={cn(
                      "block rounded-md px-2 py-1.5 text-sm",
                      pathname === item.href
                        ? "bg-indigo-50 font-medium text-indigo-800"
                        : "text-slate-700 hover:bg-slate-100",
                    )}
                  >
                    {item.label}
                  </Link>
                ),
              )}
            </div>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-end gap-3 border-b border-slate-200 bg-white px-6">
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

        <main className="min-w-0 flex-1 p-6">{children}</main>
      </div>
    </div>
  );
}
