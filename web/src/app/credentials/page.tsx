"use client";

import { useEffect, useState } from "react";
import { Copy, KeyRound, LoaderCircle, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { fetchAPIKey, regenerateAPIKey, type APIKeyInfo } from "@/lib/api";

export default function CredentialsPage() {
  const [info, setInfo] = useState<APIKeyInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isRegenerating, setIsRegenerating] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setIsLoading(true);
      try {
        const result = await fetchAPIKey();
        if (!cancelled) {
          setInfo(result);
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : "读取 API 凭证失败");
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleCopy = async () => {
    if (!info?.api_key) {
      return;
    }
    try {
      await navigator.clipboard.writeText(info.api_key);
      toast.success("API 凭证已复制");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "复制失败");
    }
  };

  const handleRegenerate = async () => {
    setIsRegenerating(true);
    try {
      const result = await regenerateAPIKey();
      setInfo(result);
      toast.success("API 凭证已重新生成，旧凭证已失效");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "重新生成失败");
    } finally {
      setIsRegenerating(false);
    }
  };

  return (
    <section className="space-y-6">
      <div className="space-y-1">
        <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">Credentials</div>
        <h1 className="text-2xl font-semibold tracking-tight">API 凭证</h1>
      </div>

      <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
        <CardContent className="space-y-6 p-6">
          <div className="flex items-start gap-3">
            <div className="flex size-10 items-center justify-center rounded-xl bg-stone-100">
              <KeyRound className="size-5 text-stone-600" />
            </div>
            <div className="space-y-1">
              <h2 className="text-lg font-semibold tracking-tight">OpenAI 兼容 API Key</h2>
              <p className="text-sm text-stone-500">
                这条凭证用于 `/v1/*` OpenAI 兼容接口鉴权。重新生成后，旧 key 会立即失效。
              </p>
            </div>
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium text-stone-700">当前凭证</label>
            <Input
              readOnly
              value={isLoading ? "读取中..." : info?.api_key || ""}
              className="h-11 rounded-xl border-stone-200 bg-stone-50 font-mono text-sm"
            />
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium text-stone-700">请求头示例</label>
            <div className="rounded-xl border border-stone-200 bg-stone-50 px-4 py-3 font-mono text-sm text-stone-700">
              {info?.api_key ? `Authorization: Bearer ${info.api_key}` : "Authorization: Bearer <api_key>"}
            </div>
          </div>

          <div className="flex items-center justify-end gap-3">
            <Button
              variant="secondary"
              className="h-10 rounded-xl bg-stone-100 px-5 text-stone-700 hover:bg-stone-200"
              onClick={() => void handleCopy()}
              disabled={isLoading || !info?.api_key}
            >
              <Copy className="size-4" />
              复制
            </Button>
            <Button
              className="h-10 rounded-xl bg-stone-950 px-5 text-white hover:bg-stone-800"
              onClick={() => void handleRegenerate()}
              disabled={isLoading || isRegenerating}
            >
              {isRegenerating ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
              重新生成
            </Button>
          </div>
        </CardContent>
      </Card>
    </section>
  );
}
