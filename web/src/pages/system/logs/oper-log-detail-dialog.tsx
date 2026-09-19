import type { ReactNode } from "react";
import { DetailDialog } from "@/components/common/detail-dialog";
import { StatusTag } from "@/components/common/status-tag";
import { formatDateTime } from "@/lib/datetime";
import type { OperLogDetail } from "@/types";
import {
  getDetailSummary,
  getOperationTypeLabel,
  getStatusMeta,
} from "./utils";

type OperLogDetailDialogProps = {
  open: boolean;
  detail: OperLogDetail | null;
  loading: boolean;
  onCancel: () => void;
};

export function OperLogDetailDialog({
  open,
  detail,
  loading,
  onCancel,
}: OperLogDetailDialogProps) {
  const statusMeta = getStatusMeta(detail?.operationStatus ?? "");
  const requestSummary = getDetailSummary(detail, "requestParams");
  const responseSummary = getDetailSummary(detail, "responseResult");

  return (
    <DetailDialog
      open={open}
      title="操作日志详情"
      description={loading ? "详情加载中" : `记录 ID：${detail?.id ?? "-"}`}
      loading={loading}
      contentClassName="max-w-[860px]"
      bodyClassName="max-h-[calc(100vh-150px)] px-card py-space-5"
      closeOnEscape={false}
      closeOnOverlayClick={false}
      onCancel={onCancel}
    >
          <div className="grid gap-4 md:grid-cols-3">
            <DetailItem label="模块" value={detail?.moduleName} />
            <DetailItem
              label="操作类型"
              value={getOperationTypeLabel(detail?.operationType ?? "")}
            />
            <DetailItem
              label="操作状态"
              value={<StatusTag tone={statusMeta.tone}>{statusMeta.label}</StatusTag>}
            />
            <DetailItem label="请求方法" value={detail?.requestMethod} />
            <DetailItem
              label="请求地址"
              value={detail?.requestUrl}
              className="md:col-span-2"
            />
            <DetailItem label="操作人" value={detail?.operatorName} />
            <DetailItem label="操作 IP" value={detail?.operatorIp} />
            <DetailItem label="耗时" value={`${detail?.costTime ?? 0} ms`} />
            <DetailItem
              label="操作时间"
              value={
                <span className="whitespace-nowrap tabular-nums">
                  {formatDateTime(detail?.operationTime)}
                </span>
              }
            />
            <DetailItem
              label="异常信息"
              value={detail?.errorMessage}
              className="md:col-span-2"
            />
          </div>

          <div className="mt-5 grid gap-4 md:grid-cols-2">
            <SummaryBlock title="请求参数摘要" value={requestSummary} />
            <SummaryBlock title="响应结果摘要" value={responseSummary} />
          </div>
    </DetailDialog>
  );
}

function DetailItem({
  label,
  value,
  className,
}: {
  label: string;
  value?: ReactNode;
  className?: string;
}) {
  return (
    <div className={className}>
      <div className="text-body-secondary text-text-tertiary">{label}</div>
      <div className="mt-1 break-words text-sm text-text-primary">
        {value || "-"}
      </div>
    </div>
  );
}

function SummaryBlock({ title, value }: { title: string; value: string }) {
  return (
    <div>
      <div className="text-body-secondary text-text-tertiary">{title}</div>
      <pre className="mt-space-2 max-h-64 overflow-auto rounded-control border border-border bg-neutral-background p-space-3 text-body-secondary text-text-secondary">
        {value}
      </pre>
    </div>
  );
}
