import { useCallback, useEffect, useRef, useState } from "react";
import { getEdgeDetail } from "@/api/edge";
import { DetailDialog } from "@/components/common/detail-dialog";
import { DetailItem } from "@/components/common/detail-item";
import { EmptyState } from "@/components/common/empty-state";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { EdgeRecord } from "@/types";

type EdgeDetailDialogProps = {
  open: boolean;
  edgeId: string | null;
  onCancel: () => void;
};

function getStatusMeta(status: EdgeRecord["status"]) {
  return status === "ONLINE"
    ? { label: "在线", tone: "success" as const }
    : { label: "离线", tone: "neutral" as const };
}

export function EdgeDetailDialog({
  open,
  edgeId,
  onCancel,
}: EdgeDetailDialogProps) {
  const [detail, setDetail] = useState<EdgeRecord | null>(null);
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
      const data = await getEdgeDetail(id);
      if (requestId !== requestIdRef.current) return;
      setDetail(data);
    } catch (loadError) {
      if (requestId !== requestIdRef.current) return;
      setNotFound(
        isApiError(loadError) &&
          (loadError.code === 404 || loadError.status === 404),
      );
      setError(getErrorMessage(loadError, "无法获取 Edge 详情，请稍后重试"));
    } finally {
      if (requestId === requestIdRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open || !edgeId) {
      requestIdRef.current += 1;
      setDetail(null);
      setError("");
      setNotFound(false);
      setLoading(false);
      return;
    }

    void loadDetail(edgeId);
  }, [edgeId, loadDetail, open]);

  const statusMeta = detail ? getStatusMeta(detail.status) : null;

  return (
    <DetailDialog
      open={open}
      title="Edge 详情"
      description={edgeId ?? undefined}
      loading={loading}
      onCancel={onCancel}
    >
      {loading ? (
        <p
          className="py-12 text-center text-sm text-text-secondary"
          aria-live="polite"
        >
          正在加载 Edge 详情…
        </p>
      ) : notFound ? (
        <EmptyState
          title="Edge 不存在"
          description="该 Edge 可能已不可用，或当前账号没有可见记录。"
        />
      ) : error ? (
        <div
          className="flex flex-col items-center gap-space-4 py-12 text-center"
          role="alert"
        >
          <div>
            <p className="text-sm font-medium text-text-primary">
              Edge 详情加载失败
            </p>
            <p className="mt-space-1 text-sm text-error">{error}</p>
          </div>
          {edgeId && (
            <Button variant="secondary" onClick={() => void loadDetail(edgeId)}>
              重试
            </Button>
          )}
        </div>
      ) : detail ? (
        <div className="grid gap-space-5 sm:grid-cols-2">
          <DetailItem label="Edge ID" value={detail.edgeId} />
          <DetailItem
            label="状态"
            value={
              <StatusTag tone={statusMeta!.tone}>
                {statusMeta!.label}
              </StatusTag>
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
        </div>
      ) : null}
    </DetailDialog>
  );
}
