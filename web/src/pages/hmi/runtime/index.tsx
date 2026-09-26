import { useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { HmiRuntimeNodeErrorBoundary } from "@/components/hmi/runtime/node-error-boundary";
import { HmiRuntimeNode } from "@/components/hmi/runtime/runtime-node";
import { getHmiRuntimeBootstrap, type HmiRuntimeBootstrap } from "@/hmi/runtime/api";
import type { DataPointBinding } from "@/hmi/model";
import { realtimePointKey } from "@/store/realtime-store";
import { useRealtimePoints } from "@/hooks/use-realtime-point";
import { Button } from "@/components/ui/button";

type HmiRuntimePageProps = {
  pageId?: string;
};

function runtimeStatusText(status: ReturnType<typeof useRealtimePoints>["status"]) {
  switch (status) {
    case "live":
      return "实时数据已连接";
    case "reconnecting":
      return "实时连接中断，正在重连";
    case "requesting-ticket":
    case "connecting":
    case "subscribing":
      return "正在连接实时数据";
    case "error":
    case "closed":
      return "实时数据连接已断开";
    default:
      return "实时连接未启动";
  }
}

function LoadingState() {
  return (
    <div className="flex h-full min-h-64 items-center justify-center text-sm text-text-secondary" role="status">
      正在加载已发布页面…
    </div>
  );
}

function RuntimeError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex h-full min-h-64 flex-col items-center justify-center gap-space-3 text-center" role="alert">
      <p className="text-sm text-error">{message}</p>
      <Button onClick={onRetry}>重新加载</Button>
    </div>
  );
}

function useRuntimeBootstrap(pageId: string | undefined) {
  const [bootstrap, setBootstrap] = useState<HmiRuntimeBootstrap | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    if (!pageId) {
      setBootstrap(null);
      setError("页面地址缺少 pageId");
      setLoading(false);
      return () => controller.abort();
    }
    setLoading(true);
    setError("");
    void getHmiRuntimeBootstrap(pageId, controller.signal).then((result) => {
      if (controller.signal.aborted) return;
      if (result.version.document.schema !== "hmi-page/v1") {
        throw new Error("页面使用了当前运行时不支持的文档版本");
      }
      setBootstrap(result);
    }).catch((cause: unknown) => {
      if (controller.signal.aborted) return;
      setBootstrap(null);
      setError(cause instanceof Error ? cause.message : "页面加载失败");
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [pageId, reloadKey]);

  return { bootstrap, error, loading, retry: () => setReloadKey((current) => current + 1) };
}

export function HmiRuntimePage({ pageId: explicitPageId }: HmiRuntimePageProps) {
  const params = useParams<{ pageId: string }>();
  const pageId = explicitPageId ?? params.pageId;
  const { bootstrap, error, loading, retry } = useRuntimeBootstrap(pageId);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const [viewportSize, setViewportSize] = useState({ width: 0, height: 0 });
  const points = useMemo(() => {
    if (!bootstrap) return [];
    const unique = new Map<string, DataPointBinding>();
    for (const node of bootstrap.version.document.nodes) {
      for (const binding of Object.values(node.bindings)) {
        if (binding.kind !== "datapoint") continue;
        const point = { deviceId: binding.deviceId, pointKey: binding.pointKey };
        unique.set(realtimePointKey(point), { kind: "datapoint", ...point });
      }
    }
    return [...unique.values()].map(({ deviceId, pointKey }) => ({ deviceId, pointKey }));
  }, [bootstrap]);
  const realtime = useRealtimePoints({ points, enabled: points.length > 0 && !loading && !error });

  useEffect(() => {
    const element = viewportRef.current;
    if (!element) return;
    const measure = () => setViewportSize({ width: element.clientWidth, height: element.clientHeight });
    measure();
    if (typeof ResizeObserver !== "undefined") {
      const observer = new ResizeObserver(measure);
      observer.observe(element);
      return () => observer.disconnect();
    }
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [loading, error, bootstrap]);

  if (loading) return <LoadingState />;
  if (error || !bootstrap) return <RuntimeError message={error || "页面加载失败"} onRetry={retry} />;

  const { document } = bootstrap.version;
  const scale = viewportSize.width > 0 && viewportSize.height > 0
    ? Math.min(viewportSize.width / document.canvas.width, viewportSize.height / document.canvas.height)
    : 1;
  const canExecuteCommands = bootstrap.canExecuteCommands;

  return (
    <main className="flex h-[calc(100vh-64px)] min-h-[400px] flex-col overflow-hidden bg-neutral-background">
      <header className="flex min-h-12 shrink-0 items-center justify-between gap-space-4 border-b border-border bg-surface px-space-4">
        <div className="min-w-0">
          <h1 className="truncate text-sm font-semibold text-text-primary">{bootstrap.page.name}</h1>
          <p className="text-xs text-text-tertiary">已发布版本 v{bootstrap.version.versionNo}</p>
        </div>
        {points.length > 0 && (
          <div className={`shrink-0 text-xs ${realtime.status === "live" ? "text-success" : "text-warning"}`} role="status">
            {runtimeStatusText(realtime.status)}
          </div>
        )}
      </header>
      <div ref={viewportRef} className="relative min-h-0 flex-1 overflow-hidden p-3">
        <div
          className="absolute left-1/2 top-1/2 overflow-hidden border border-border bg-white shadow-sm"
          style={{
            width: document.canvas.width,
            height: document.canvas.height,
            transform: `translate(-50%, -50%) scale(${scale})`,
            transformOrigin: "center center",
          }}
        >
          {[...document.nodes].sort((left, right) => left.zIndex - right.zIndex).map((node) => (
            <div
              key={node.nodeId}
              className="absolute"
              style={{
                left: node.x,
                top: node.y,
                width: node.width,
                height: node.height,
                zIndex: node.zIndex,
                transform: `rotate(${node.rotation}deg)`,
              }}
            >
              <HmiRuntimeNodeErrorBoundary nodeId={node.nodeId}>
                <HmiRuntimeNode
                  node={node}
                  dataPoints={bootstrap.dataPoints}
                  canExecuteCommands={canExecuteCommands}
                />
              </HmiRuntimeNodeErrorBoundary>
            </div>
          ))}
        </div>
      </div>
    </main>
  );
}
