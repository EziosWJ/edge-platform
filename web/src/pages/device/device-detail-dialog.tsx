import { useCallback, useEffect, useRef, useState } from "react";
import { getDeviceDetail } from "@/api/device";
import { DetailDialog } from "@/components/common/detail-dialog";
import { DetailItem } from "@/components/common/detail-item";
import { EmptyState } from "@/components/common/empty-state";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DeviceCommunicationStatus, DeviceRecord } from "@/types";

type DeviceDetailDialogProps = {
  open: boolean;
  deviceId: string | null;
  onCancel: () => void;
};

function getStatusMeta(status: DeviceCommunicationStatus) {
  switch (status) {
    case "INITIAL":
      return { label: "初始", tone: "info" as const };
    case "ONLINE":
      return { label: "在线", tone: "success" as const };
    case "DEGRADED":
      return { label: "降级", tone: "warning" as const };
    case "OFFLINE":
      return { label: "离线", tone: "error" as const };
  }
}

export function DeviceDetailDialog({
  open,
  deviceId,
  onCancel,
}: DeviceDetailDialogProps) {
  const [detail, setDetail] = useState<DeviceRecord | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);
  const requestIdRef = useRef(0);

  const loadDetail = useCallback(async (id: string) => {
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;
    setLoading(true);
    setDetail(null);
    setError("");
    setNotFound(false);

    try {
      const data = await getDeviceDetail(id);
      if (requestId !== requestIdRef.current) return;
      setDetail(data);
    } catch (loadError) {
      if (requestId !== requestIdRef.current) return;
      setNotFound(
        isApiError(loadError) &&
          (loadError.code === 404 || loadError.status === 404),
      );
      setError(getErrorMessage(loadError, "无法获取 Device 详情，请稍后重试"));
    } finally {
      if (requestId === requestIdRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open || !deviceId) {
      requestIdRef.current += 1;
      setDetail(null);
      setError("");
      setNotFound(false);
      setLoading(false);
      return;
    }
    void loadDetail(deviceId);
  }, [deviceId, loadDetail, open]);

  const statusMeta = detail ? getStatusMeta(detail.communicationStatus) : null;

  return (
    <DetailDialog
      open={open}
      title="Device 详情"
      description={deviceId ?? undefined}
      loading={loading}
      onCancel={onCancel}
    >
      {loading ? (
        <p
          className="py-12 text-center text-sm text-text-secondary"
          aria-live="polite"
        >
          正在加载 Device 详情…
        </p>
      ) : notFound ? (
        <EmptyState
          title="Device 不存在"
          description="该 Device 可能尚未被 Cloud 发现，或当前账号没有可见记录。"
        />
      ) : error ? (
        <div
          className="flex flex-col items-center gap-space-4 py-12 text-center"
          role="alert"
        >
          <div>
            <p className="text-sm font-medium text-text-primary">
              Device 详情加载失败
            </p>
            <p className="mt-space-1 text-sm text-error">{error}</p>
          </div>
          {deviceId && (
            <Button variant="secondary" onClick={() => void loadDetail(deviceId)}>
              重试
            </Button>
          )}
        </div>
      ) : detail ? (
        <div className="grid gap-space-5 sm:grid-cols-2">
          <DetailItem label="Cloud Device ID" value={detail.deviceId} />
          <DetailItem label="所属 Edge ID" value={detail.edgeId} />
          <DetailItem label="来源 Device ID" value={detail.sourceDeviceId} />
          <DetailItem
            label="通信状态"
            value={
              <StatusTag tone={statusMeta!.tone}>{statusMeta!.label}</StatusTag>
            }
          />
          <DetailItem
            label="首次登记时间（UTC）"
            value={formatDateTime(detail.registeredAt)}
          />
          <DetailItem
            label="最近状态时间（UTC）"
            value={formatDateTime(detail.lastSeenAt)}
          />
          <DetailItem
            label="最近尝试时间（来源 UTC）"
            value={formatDateTime(detail.lastAttemptAt)}
          />
          <DetailItem
            label="最近成功时间（来源 UTC）"
            value={formatDateTime(detail.lastSuccessAt)}
          />
          <DetailItem
            className="sm:col-span-2"
            label="当前通信诊断"
            value={detail.communicationError}
          />
        </div>
      ) : null}
    </DetailDialog>
  );
}
