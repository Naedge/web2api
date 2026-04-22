"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { LoaderCircle, LockKeyhole } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { fetchAuthStatus, login, setup } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [initialized, setInitialized] = useState<boolean | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const loadStatus = async () => {
      try {
        const result = await fetchAuthStatus();
        if (cancelled) {
          return;
        }
        if (result.authenticated) {
          router.replace("/accounts");
          return;
        }
        setInitialized(result.initialized);
      } catch (error) {
        const message = error instanceof Error ? error.message : "读取登录状态失败";
        toast.error(message);
        setInitialized(true);
      }
    };

    void loadStatus();
    return () => {
      cancelled = true;
    };
  }, [router]);

  const handleSubmit = async () => {
    const normalizedUsername = username.trim();
    const normalizedPassword = password.trim();
    if (!normalizedUsername || !normalizedPassword) {
      toast.error("请输入账号和密码");
      return;
    }
    if (initialized === false && normalizedPassword !== confirmPassword.trim()) {
      toast.error("两次密码输入不一致");
      return;
    }

    setIsSubmitting(true);
    try {
      if (initialized === false) {
        await setup(normalizedUsername, normalizedPassword);
      } else {
        await login(normalizedUsername, normalizedPassword);
      }
      router.replace("/accounts");
    } catch (error) {
      const message = error instanceof Error ? error.message : "登录失败";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="grid min-h-[calc(100vh-1rem)] w-full place-items-center px-4 py-6">
      <Card className="w-full max-w-[505px] rounded-[30px] border-white/80 bg-white/95 shadow-[0_28px_90px_rgba(28,25,23,0.10)]">
        <CardContent className="space-y-7 p-6 sm:p-8">
          <div className="space-y-4 text-center">
            <div className="mx-auto inline-flex size-14 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
              <LockKeyhole className="size-5" />
            </div>
            <div className="space-y-2">
              <h1 className="text-3xl font-semibold tracking-tight text-stone-950">
                {initialized === false ? "初始化管理员账号" : "欢迎回来"}
              </h1>
              <p className="text-sm leading-6 text-stone-500">
                {initialized === false
                  ? "首次进入先创建管理员账号和密码。"
                  : "输入管理员账号密码后继续使用账号管理和图片生成功能。"}
              </p>
            </div>
          </div>

          <div className="space-y-3">
            <label htmlFor="username" className="block text-sm font-medium text-stone-700">
              账号
            </label>
            <Input
              id="username"
              type="text"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  void handleSubmit();
                }
              }}
              placeholder="请输入账号"
              className="h-13 rounded-2xl border-stone-200 bg-white px-4"
            />
          </div>

          <div className="space-y-3">
            <label htmlFor="password" className="block text-sm font-medium text-stone-700">
              密码
            </label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  void handleSubmit();
                }
              }}
              placeholder="请输入密码"
              className="h-13 rounded-2xl border-stone-200 bg-white px-4"
            />
          </div>

          {initialized === false ? (
            <div className="space-y-3">
              <label htmlFor="confirm-password" className="block text-sm font-medium text-stone-700">
                确认密码
              </label>
              <Input
                id="confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    void handleSubmit();
                  }
                }}
                placeholder="请再次输入密码"
                className="h-13 rounded-2xl border-stone-200 bg-white px-4"
              />
            </div>
          ) : null}

          <Button
            className="h-13 w-full rounded-2xl bg-stone-950 text-white hover:bg-stone-800"
            onClick={() => void handleSubmit()}
            disabled={isSubmitting || initialized === null}
          >
            {isSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : null}
            {initialized === false ? "创建并登录" : "登录"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
