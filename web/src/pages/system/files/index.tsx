import {
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  batchDeleteFiles,
  deleteFile,
  downloadFile,
  getFilePage,
  updateFileStatus,
} from "@/api/file";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { DataTableCard } from "@/components/common/data-table-card";
import { DataTable } from "@/components/common/data-table";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  API_STATUS_VALUES,
  COMMON_STATUS_OPTIONS,
  DICT_CODES,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { useListPage } from "@/hooks/use-list-page";
import type { ApiStatus, FileRecord } from "@/types";
import { createFileColumns } from "./columns";
import { FileDetailDialog } from "./file-detail-dialog";
import { FileEditDialog } from "./file-edit-dialog";
import { FilePreviewDialog } from "./file-preview-dialog";
import { FileUploadDialog } from "./file-upload-dialog";
import { getErrorMessage } from "@/lib/api-error";
import { downloadBlob } from "./utils";

type FilterState = {
  originalName: string;
  businessModule: string;
  mimeType: string;
  status: "all" | ApiStatus;
};

type ConfirmAction =
  | { type: "delete"; file: FileRecord }
  | { type: "batchDelete"; files: FileRecord[] }
  | { type: "status"; file: FileRecord; status: ApiStatus };

const DEFAULT_FILTERS: FilterState = {
  originalName: "",
  businessModule: "",
  mimeType: "",
  status: "all",
};

function buildQuery(filters: FilterState, page: number, pageSize: number) {
  return {
    page,
    pageSize,
    originalName: filters.originalName.trim() || undefined,
    businessModule: filters.businessModule.trim() || undefined,
    mimeType: filters.mimeType.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
  };
}

export function SystemFilesPage() {
  const {
    data: files,
    total,
    loading,
    error,
    page,
    pageSize,
    setPage,
    setPageSize,
    filters,
    setFilter,
    submitFilters,
    resetFilters,
    reload: loadFiles,
  } = useListPage<FilterState, FileRecord>({
    fetch: getFilePage,
    defaultFilters: DEFAULT_FILTERS,
    toQuery: (f, p, ps) => buildQuery(f, p, ps),
    onError: (err) =>
      toast.error({
        title: "加载失败",
        description: getErrorMessage(err, "文件列表加载失败"),
      }),
  });

  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [downloadingIds, setDownloadingIds] = useState<Set<number>>(new Set());
  const [uploadOpen, setUploadOpen] = useState(false);
  const [detailRecord, setDetailRecord] = useState<FileRecord | null>(null);
  const [editRecord, setEditRecord] = useState<FileRecord | null>(null);
  const [previewRecord, setPreviewRecord] = useState<FileRecord | null>(null);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const businessModuleDict = useDictOptions(DICT_CODES.FILE_BUSINESS_MODULE, {
    showErrorToast: true,
    errorTitle: "文件业务模块字典加载失败",
  });
  const statusDict = useDictOptions<ApiStatus>(DICT_CODES.COMMON_STATUS, {
    fallback: COMMON_STATUS_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "文件状态字典加载失败",
  });

  // 同步 selectedIds 与当前页数据
  useEffect(() => {
    setSelectedIds((current) => {
      const nextRecordIds = new Set(files.map((item) => item.id));
      return new Set([...current].filter((id) => nextRecordIds.has(id)));
    });
  }, [files]);

  const toggleSelect = useCallback((id: number, checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(
    (checked: boolean) => {
      setSelectedIds((current) => {
        const next = new Set(current);
        files.forEach((item) => {
          if (checked) {
            next.add(item.id);
          } else {
            next.delete(item.id);
          }
        });
        return next;
      });
    },
    [files],
  );

  const allChecked = useMemo(
    () => files.length > 0 && files.every((item) => selectedIds.has(item.id)),
    [files, selectedIds],
  );

  const selectedFiles = useMemo(
    () => files.filter((item) => selectedIds.has(item.id)),
    [files, selectedIds],
  );

  const handleDownload = useCallback(async (record: FileRecord) => {
    setDownloadingIds((current) => {
      const next = new Set(current);
      next.add(record.id);
      return next;
    });

    try {
      const blob = await downloadFile(record.id);
      downloadBlob(blob, record.originalName);
    } catch (err) {
      toast.error({
        title: "下载失败",
        description: getErrorMessage(err, "文件下载失败"),
      });
    } finally {
      setDownloadingIds((current) => {
        const next = new Set(current);
        next.delete(record.id);
        return next;
      });
    }
  }, []);

  const runConfirmAction = async () => {
    if (!confirmAction) return;

    setConfirmLoading(true);

    try {
      if (confirmAction.type === "delete") {
        await deleteFile(confirmAction.file.id);
        toast.success("文件已删除");
      }

      if (confirmAction.type === "batchDelete") {
        await batchDeleteFiles({
          ids: confirmAction.files.map((item) => item.id),
        });
        toast.success("文件已批量删除");
      }

      if (confirmAction.type === "status") {
        await updateFileStatus(confirmAction.file.id, {
          status: confirmAction.status,
        });
        toast.success(
          confirmAction.status === 1 ? "文件已启用" : "文件已禁用",
        );
      }

      setConfirmAction(null);
      setSelectedIds(new Set());
      await loadFiles();
    } catch (actionError) {
      toast.error({
        title: "操作失败",
        description: getErrorMessage(actionError, "请稍后重试"),
      });
    } finally {
      setConfirmLoading(false);
    }
  };

  const confirmMeta = useMemo(() => {
    if (!confirmAction) return null;

    if (confirmAction.type === "delete") {
      return {
        title: "删除文件",
        description: `确认删除文件「${confirmAction.file.originalName}」吗？此操作不可恢复。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDelete") {
      return {
        title: "批量删除文件",
        description: `确认删除已选择的 ${confirmAction.files.length} 个文件吗？此操作不可恢复。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    const enabled = confirmAction.status === 1;
    return {
      title: enabled ? "启用文件" : "禁用文件",
      description: `确认${enabled ? "启用" : "禁用"}文件「${confirmAction.file.originalName}」吗？`,
      confirmText: enabled ? "启用" : "禁用",
      danger: !enabled,
    };
  }, [confirmAction]);

  const columns = useMemo(
    () =>
      createFileColumns({
        onPreview: setPreviewRecord,
        onDownload: (record) => void handleDownload(record),
        onDetail: setDetailRecord,
        onEdit: setEditRecord,
        onChangeStatus: (file, status) =>
          setConfirmAction({ type: "status", file, status }),
        onDelete: (file) => setConfirmAction({ type: "delete", file }),
        downloadingIds,
        selectedIds,
        onToggleSelect: toggleSelect,
        onToggleSelectAll: toggleSelectAll,
        allChecked,
        selectableCount: files.length,
      }),
    [
      downloadingIds,
      selectedIds,
      toggleSelect,
      toggleSelectAll,
      allChecked,
      files.length,
      handleDownload,
    ],
  );

  return (
    <>
      <PageHeader
        title="文件管理"
        description="维护文件元数据，支持上传、预览、下载、编辑和启停管理。"
        actions={
          <Button variant="primary" onClick={() => setUploadOpen(true)}>
            <Plus className="h-4 w-4" aria-hidden />
            上传文件
          </Button>
        }
      />

      <SearchFilterBar
        actions={
          <>
            <Button variant="secondary" onClick={resetFilters}>
              <RotateCcw className="h-4 w-4" aria-hidden />
              重置
            </Button>
            <Button variant="primary" onClick={() => submitFilters()}>
              <Search className="h-4 w-4" aria-hidden />
              查询
            </Button>
          </>
        }
      >
        <form className="contents" onSubmit={(event) => { event.preventDefault(); submitFilters(); }}>
          <Input
            value={filters.originalName}
            onChange={(event) => setFilter("originalName", event.target.value)}
            placeholder="文件名"
          />
          {businessModuleDict.options.length > 0 ? (
            <Select
              value={filters.businessModule}
              onChange={(event) => setFilter("businessModule", event.target.value)}
              aria-label="筛选业务模块"
            >
              <option value="">全部业务模块</option>
              {businessModuleDict.options.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </Select>
          ) : (
            <Input
              value={filters.businessModule}
              onChange={(event) => setFilter("businessModule", event.target.value)}
              placeholder="业务模块"
            />
          )}
          <Input
            value={filters.mimeType}
            onChange={(event) => setFilter("mimeType", event.target.value)}
            placeholder="MIME 类型"
          />
          <Select
            value={String(filters.status)}
            onChange={(event) =>
              setFilter(
                "status",
                event.target.value === "all"
                  ? "all"
                  : (Number(event.target.value) as ApiStatus),
              )
            }
            aria-label="筛选状态"
          >
            <option value="all">全部状态</option>
            {statusDict.options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </form>
      </SearchFilterBar>

      <DataTableCard>
        <TableToolbar
          title="文件列表"
          description={`共 ${total} 条数据，当前显示 ${files.length} 条。`}
          actions={
            <>
              <StatusTag tone={loading ? "warning" : error ? "error" : "info"}>
                {loading ? "加载中" : error ? "加载失败" : "已同步"}
              </StatusTag>
              <Button size="sm" variant="secondary" onClick={loadFiles}>
                <RefreshCw className="h-4 w-4" aria-hidden />
                刷新
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={selectedFiles.length === 0}
                onClick={() =>
                  setConfirmAction({
                    type: "batchDelete",
                    files: selectedFiles,
                  })
                }
              >
                <Trash2 className="h-4 w-4" aria-hidden />
                批量删除
              </Button>
            </>
          }
        />
        <DataTable<FileRecord>
          columns={columns}
          dataSource={files}
          rowKey="id"
          loading={loading}
          error={error}
          minWidth={1600}
          empty={
            <EmptyState
              title="暂无文件数据"
              description="调整筛选条件后重新查询。"
              actionText="重置筛选"
              onAction={resetFilters}
            />
          }
        />
        <Pagination
          page={page}
          pageSize={pageSize}
          total={total}
          disabled={loading}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      </DataTableCard>

      <FileUploadDialog
        open={uploadOpen}
        businessModuleOptions={businessModuleDict.options}
        onCancel={() => setUploadOpen(false)}
        onUploaded={() => {
          void loadFiles();
        }}
      />

      <FileDetailDialog
        open={!!detailRecord}
        record={detailRecord}
        onCancel={() => setDetailRecord(null)}
      />

      <FileEditDialog
        open={!!editRecord}
        record={editRecord}
        businessModuleOptions={businessModuleDict.options}
        onCancel={() => setEditRecord(null)}
        onSaved={() => {
          setEditRecord(null);
          void loadFiles();
        }}
      />

      <FilePreviewDialog
        open={!!previewRecord}
        record={previewRecord}
        onCancel={() => setPreviewRecord(null)}
        onDownload={(record) => void handleDownload(record)}
      />

      {confirmMeta && (
        <ConfirmDialog
          open={!!confirmAction}
          title={confirmMeta.title}
          description={confirmMeta.description}
          confirmText={confirmMeta.confirmText}
          danger={confirmMeta.danger}
          loading={confirmLoading}
          onConfirm={runConfirmAction}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </>
  );
}
