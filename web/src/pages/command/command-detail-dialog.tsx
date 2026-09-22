import { useEffect, useState } from "react";
import { getCommandDetail } from "@/api/command";
import { DetailDialog } from "@/components/common/detail-dialog";
import { DetailItem } from "@/components/common/detail-item";
import { EmptyState } from "@/components/common/empty-state";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { CommandRecord, CommandStatus } from "@/types";

type CommandDetailDialogProps = {
  open: boolean;
  commandId: string | null;
  onCancel: () => void;
};

const TERMINAL_STATUSES: CommandStatus[] = [
  "REJECTED",
  "EXPIRED",
  "SUCCEEDED",
  "FAILED",
];

const POLL_INTERVAL_MS = 2000;

function shouldStopPolling(error: unknown) {
	if (!isApiError(error)) return false;
	return error.status === 401 || error.status === 403 || error.status === 404 ||
		error.type === "unauthorized" || error.type === "forbidden" || error.type === "notFound";
}

function statusMeta(status: CommandStatus) {
  switch (status) {
    case "SUCCEEDED":
      return { label: "已成功", tone: "success" as const };
    case "REJECTED":
    case "FAILED":
      return { label: status === "REJECTED" ? "已拒绝" : "执行失败", tone: "error" as const };
    case "EXPIRED":
      return { label: "已过期", tone: "warning" as const };
    case "ACCEPTED":
      return { label: "边缘已接收", tone: "info" as const };
    default:
      return { label: "等待执行", tone: "neutral" as const };
  }
}

function jsonText(value: unknown) {
  if (value === undefined) return "-";
  if (typeof value === "string") return JSON.stringify(value, null, 2);
  try {
    return JSON.stringify(value, null, 2) ?? "null";
  } catch {
    return String(value);
  }
}

function JsonBlock({ value }: { value: unknown }) {
  return (
    <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-control border border-border bg-neutral-background p-space-3 font-mono text-xs leading-5 text-text-primary">
      {jsonText(value)}
    </pre>
  );
}

export function CommandDetailDialog({
  open,
  commandId,
  onCancel,
}: CommandDetailDialogProps) {
  const [detail, setDetail] = useState<CommandRecord | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    if (!open || !commandId) {
      setDetail(null);
      setError("");
      setNotFound(false);
      setLoading(false);
      return;
    }

    let disposed = false;
    let timer: number | undefined;
    let controller: AbortController | undefined;

    const schedulePoll = () => {
      if (!disposed) {
        timer = window.setTimeout(() => void load(true), POLL_INTERVAL_MS);
      }
    };

    const load = async (background = false) => {
      controller?.abort();
      const requestController = new AbortController();
      controller = requestController;
      if (!background) {
        setLoading(true);
        setError("");
        setNotFound(false);
      }
      try {
        const value = await getCommandDetail(commandId, requestController.signal);
        if (disposed) return;
        setDetail(value);
        setError("");
        setNotFound(false);
        if (!TERMINAL_STATUSES.includes(value.status)) {
          schedulePoll();
        }
      } catch (loadError) {
        if (disposed || requestController.signal.aborted) return;
        setNotFound(
          isApiError(loadError) &&
            (loadError.code === 404 || loadError.status === 404),
        );
        setError(getErrorMessage(loadError, "无法获取 Command 详情，请稍后重试"));
        if (!shouldStopPolling(loadError)) schedulePoll();
      } finally {
        if (!disposed && !background) setLoading(false);
      }
    };

    void load();
    return () => {
      disposed = true;
      if (timer !== undefined) window.clearTimeout(timer);
      controller?.abort();
    };
  }, [commandId, open, reloadToken]);

  const meta = detail ? statusMeta(detail.status) : null;

  return (
    <DetailDialog
      open={open}
      title="Command 详情"
      description={commandId ?? undefined}
      loading={loading}
      onCancel={onCancel}
      bodyClassName="space-y-space-6"
    >
      {loading ? (
        <p className="py-12 text-center text-sm text-text-secondary" aria-live="polite">
          正在读取 Cloud 执行事实…
        </p>
      ) : notFound ? (
        <EmptyState title="Command 不存在" description="请确认 Command ID，或检查当前账号的查询权限。" />
      ) : error ? (
        <div className="flex flex-col items-center gap-space-4 py-12 text-center" role="alert">
          <div>
            <p className="text-sm font-medium text-text-primary">Command 详情加载失败</p>
            <p className="mt-space-1 text-sm text-error">{error}</p>
          </div>
          <Button variant="secondary" onClick={() => setReloadToken((value) => value + 1)}>
            重试
          </Button>
        </div>
      ) : detail ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-space-3 border-b border-border pb-space-4">
            <div>
              <p className="text-xs text-text-tertiary">Command name</p>
              <h2 className="mt-1 text-lg font-semibold text-text-primary">{detail.name}</h2>
            </div>
            <StatusTag tone={meta!.tone}>{meta!.label}</StatusTag>
          </div>
          <div className="grid gap-space-5 sm:grid-cols-2">
            <DetailItem label="Cloud Device ID" value={detail.deviceId} />
            <DetailItem label="请求用户 ID" value={String(detail.requestedBy)} />
            <DetailItem label="冻结 Edge ID" value={detail.edgeId} />
            <DetailItem label="冻结来源 Device ID" value={detail.sourceDeviceId} />
            <DetailItem label="签发时间（Cloud UTC）" value={formatDateTime(detail.issuedAt)} />
            <DetailItem label="到期时间（Cloud UTC）" value={formatDateTime(detail.expiresAt)} />
            <DetailItem label="Edge 首次接收（Edge UTC）" value={formatDateTime(detail.edgeReceivedAt)} />
            <DetailItem label="开始执行（Edge UTC）" value={formatDateTime(detail.startedAt)} />
            <DetailItem label="完成执行（Edge UTC）" value={formatDateTime(detail.completedAt)} />
            <DetailItem label="结果收到（Cloud UTC）" value={formatDateTime(detail.resultReceivedAt)} />
            <DetailItem label="投递到期（Cloud UTC）" value={formatDateTime(detail.deliveryExpiredAt)} />
          </div>
          <section>
            <h3 className="text-sm font-semibold text-text-primary">Command 参数</h3>
            <div className="mt-space-2"><JsonBlock value={detail.args} /></div>
          </section>
          <section>
            <h3 className="text-sm font-semibold text-text-primary">执行结果</h3>
            <div className="mt-space-2"><JsonBlock value={detail.result} /></div>
          </section>
          {(detail.errorType || detail.errorMessage) && (
            <section className="border-l-2 border-warning px-space-3">
              <h3 className="text-sm font-semibold text-text-primary">执行错误</h3>
              <p className="mt-space-1 text-sm text-text-secondary">
                {[detail.errorType, detail.errorMessage].filter(Boolean).join("：")}
              </p>
            </section>
          )}
        </>
      ) : null}
    </DetailDialog>
  );
}
