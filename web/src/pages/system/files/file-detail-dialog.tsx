import { useCallback, useEffect, useState } from "react";
import { getFileAccessUrl, getFileDetail } from "@/api/file";
import { ApiStatusTag } from "@/components/common/api-status-tag";
import { DetailDialog } from "@/components/common/detail-dialog";
import { DetailItem } from "@/components/common/detail-item";
import type { FileRecord } from "@/types";
import { formatDateTime } from "@/lib/datetime";
import { getErrorMessage } from "@/lib/api-error";
import { formatFileSize } from "./utils";

type FileDetailDialogProps = {
  open: boolean;
  record: FileRecord | null;
  onCancel: () => void;
};

export function FileDetailDialog({
  open,
  record,
  onCancel,
}: FileDetailDialogProps) {
  const [detail, setDetail] = useState<FileRecord | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const loadDetail = useCallback(async (id: number) => {
    setLoading(true);
    setError("");
    try {
      const data = await getFileDetail(id);
      setDetail(data);
    } catch (err) {
      setError(getErrorMessage(err, "文件详情加载失败"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open || !record) {
      setDetail(null);
      setError("");
      return;
    }

    setDetail(record);
    void loadDetail(record.id);
  }, [open, record, loadDetail]);

  const data = detail ?? record;
  const accessUrl = data?.accessUrl ? getFileAccessUrl(data.accessUrl) : "";

  return (
    <DetailDialog
      open={open}
      title="文件详情"
      description={data?.originalName}
      loading={loading}
      onCancel={onCancel}
    >
      {error && !data ? (
        <p className="text-sm text-error">{error}</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          <DetailItem label="ID" value={data?.id} />
          <DetailItem label="原始文件名" value={data?.originalName} />
          <DetailItem label="存储文件名" value={data?.storageName} />
          <DetailItem label="扩展名" value={data?.extension} />
          <DetailItem label="MIME 类型" value={data?.mimeType} />
          <DetailItem label="文件大小" value={formatFileSize(data?.fileSize)} />
          <DetailItem label="业务模块" value={data?.businessModule} />
          <DetailItem label="备注" value={data?.remark} />
          <DetailItem
            label="状态"
            value={data ? <ApiStatusTag status={data.status} /> : undefined}
          />
          <DetailItem label="创建时间" value={formatDateTime(data?.createTime)} />
          <DetailItem
            label="访问地址"
            className="sm:col-span-2"
            value={
              accessUrl ? (
                <a
                  href={accessUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="break-all text-primary underline"
                >
                  {accessUrl}
                </a>
              ) : undefined
            }
          />
        </div>
      )}
    </DetailDialog>
  );
}
