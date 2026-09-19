import {
  Download,
  Eye,
  FileText,
  Info,
  Loader2,
  Pencil,
  Trash2,
} from "lucide-react";
import { ApiStatusTag } from "@/components/common/api-status-tag";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { formatDateTime } from "@/lib/datetime";
import type { ApiStatus, DataTableColumn, FileRecord } from "@/types";
import { formatFileSize, isPreviewable } from "./utils";

type FileColumnActions = {
  onPreview: (record: FileRecord) => void;
  onDownload: (record: FileRecord) => void;
  onDetail: (record: FileRecord) => void;
  onEdit: (record: FileRecord) => void;
  onChangeStatus: (record: FileRecord, status: ApiStatus) => void;
  onDelete: (record: FileRecord) => void;
  downloadingIds: Set<number>;
  selectedIds: Set<number>;
  onToggleSelect: (id: number, checked: boolean) => void;
  onToggleSelectAll: (checked: boolean) => void;
  allChecked: boolean;
  selectableCount: number;
};

export function createFileColumns(actions: FileColumnActions): DataTableColumn<FileRecord>[] {
  return [
    {
      title: (
        <Checkbox
          aria-label="选择当前页文件"
          checked={actions.allChecked}
          disabled={actions.selectableCount === 0}
          onChange={(event) => actions.onToggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, record) => (
        <Checkbox
          aria-label={`选择文件 ${record.originalName}`}
          checked={actions.selectedIds.has(record.id)}
          onChange={(event) =>
            actions.onToggleSelect(record.id, event.target.checked)
          }
        />
      ),
    },
    {
      title: "文件名",
      key: "originalName",
      width: 260,
      render: (_, record) => (
        <div className="flex items-start gap-2">
          <FileText className="mt-0.5 h-4 w-4 shrink-0 text-text-tertiary" aria-hidden />
          <div className="min-w-0">
            <div className="font-medium text-text-primary">
              {record.originalName}
            </div>
            <div className="text-xs text-text-tertiary">
              ID {record.id}
            </div>
          </div>
        </div>
      ),
    },
    {
      title: "扩展名",
      dataIndex: "extension",
      width: 90,
      render: (value) => (
        <span className="font-mono text-body-secondary text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "MIME 类型",
      dataIndex: "mimeType",
      width: 180,
      render: (value) => (
        <span className="block max-w-[180px] truncate font-mono text-body-secondary text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "文件大小",
      key: "fileSize",
      width: 100,
      render: (_, record) => (
        <span className="tabular-nums text-text-secondary">
          {formatFileSize(record.fileSize)}
        </span>
      ),
    },
    {
      title: "业务模块",
      dataIndex: "businessModule",
      width: 120,
      render: (value) => (
        <span className="text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 96,
      render: (_, record) => <ApiStatusTag status={record.status} />,
    },
    {
      title: "备注",
      dataIndex: "remark",
      width: 160,
      render: (value) => (
        <span className="block max-w-[160px] truncate text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "创建时间",
      dataIndex: "createTime",
      width: 170,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatDateTime(String(value ?? ""))}
        </span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 300,
      render: (_, record) => {
        const previewable = isPreviewable(record.mimeType);
        const isDownloading = actions.downloadingIds.has(record.id);
        const nextStatus = record.status === 1 ? 0 : 1;

        return (
          <div className="inline-flex flex-wrap items-center justify-center gap-1">
            <Button
              size="sm"
              variant="ghost"
              disabled={!previewable}
              title={previewable ? "预览" : "该文件类型不支持预览"}
              onClick={() => actions.onPreview(record)}
            >
              <Eye className="h-4 w-4" aria-hidden />
              预览
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={isDownloading}
              onClick={() => actions.onDownload(record)}
            >
              {isDownloading ? (
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
              ) : (
                <Download className="h-4 w-4" aria-hidden />
              )}
              下载
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => actions.onDetail(record)}
            >
              <Info className="h-4 w-4" aria-hidden />
              详情
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => actions.onEdit(record)}
            >
              <Pencil className="h-4 w-4" aria-hidden />
              编辑
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                actions.onChangeStatus(record, nextStatus)
              }
            >
              {nextStatus === 1 ? "启用" : "禁用"}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-error hover:text-error"
              onClick={() => actions.onDelete(record)}
            >
              <Trash2 className="h-4 w-4" aria-hidden />
              删除
            </Button>
          </div>
        );
      },
    },
  ];
}
