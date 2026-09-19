import type { ReactNode } from "react";
import { DetailDialog } from "@/components/common/detail-dialog";
import { StatusTag } from "@/components/common/status-tag";
import { formatDateTime } from "@/lib/datetime";
import type { LoginLogRecord } from "@/types";
import { getStatusMeta } from "./utils";

type LoginLogDetailDialogProps = {
  open: boolean;
  detail: LoginLogRecord | null;
  loading: boolean;
  onCancel: () => void;
};

export function LoginLogDetailDialog({
  open,
  detail,
  loading,
  onCancel,
}: LoginLogDetailDialogProps) {
  const statusMeta = getStatusMeta(detail?.loginStatus ?? "");

  return (
    <DetailDialog
      open={open}
      title="登录日志详情"
      description={loading ? "详情加载中" : `记录 ID：${detail?.id ?? "-"}`}
      loading={loading}
      contentClassName="max-w-[720px]"
      bodyClassName="max-h-none grid gap-4 overflow-visible px-card py-space-5 md:grid-cols-2"
      closeOnEscape={false}
      closeOnOverlayClick={false}
      onCancel={onCancel}
    >
          <DetailItem label="用户名" value={detail?.username} />
          <DetailItem
            label="登录状态"
            value={<StatusTag tone={statusMeta.tone}>{statusMeta.label}</StatusTag>}
          />
          <DetailItem label="登录 IP" value={detail?.loginIp} />
          <DetailItem label="浏览器" value={detail?.browser} />
          <DetailItem label="操作系统" value={detail?.os} />
          <DetailItem
            label="登录时间"
            value={
              <span className="whitespace-nowrap tabular-nums">
                {formatDateTime(detail?.loginTime)}
              </span>
            }
          />
          <DetailItem
            label="消息"
            value={detail?.message}
            className="md:col-span-2"
          />
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
