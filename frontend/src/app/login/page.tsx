"use client";

import { useRouter } from "next/navigation";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/field";
import { api, tokenStore } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [pending, setPending] = React.useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setPending(true);
    try {
      const data = await api.signIn(email, password);
      tokenStore.save(data.token, data.user);
      router.replace("/voices");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Đăng nhập thất bại");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader
          title="Đăng nhập"
          description="Dùng tài khoản multime.ai của bạn — voice sẽ được đăng lên chính tài khoản đó."
        />
        <CardBody>
          <form onSubmit={submit} className="space-y-4">
            <Field label="Email">
              <Input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
              />
            </Field>
            <Field label="Mật khẩu">
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoComplete="current-password"
              />
            </Field>

            {error ? <p className="text-sm text-red-700">{error}</p> : null}

            <Button type="submit" className="w-full" disabled={pending}>
              {pending ? "Đang đăng nhập…" : "Đăng nhập"}
            </Button>

            <p className="text-center text-xs text-slate-500">
              Chưa có tài khoản? Đăng ký tại{" "}
              <a
                href="https://multime.ai"
                target="_blank"
                rel="noreferrer"
                className="text-indigo-700 hover:underline"
              >
                multime.ai
              </a>{" "}
              — hệ thống này không tạo tài khoản riêng.
            </p>
          </form>
        </CardBody>
      </Card>
    </div>
  );
}
