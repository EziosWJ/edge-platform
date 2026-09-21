import { Eye, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { getEdgePage } from "@/api/edge";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
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
import { useListPage } from "@/hooks/use-list-page";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type {
  DataTableColumn,
  EdgePageQuery,
  EdgeRecord,
  EdgeStatus,
} from "@/types";
import { EdgeDetailDialog } from "./edge-detail-dialog";

type FilterState = {
  status: "all" | EdgeStatus;
  edgeId: string;
};

const DEFAULT_FILTERS: FilterState = {
  status: "all",
  edgeId: "",
};

function buildQuery(filters: FilterState, page: number, pageSize: number) {
  return {
    page,
    pageSize,
    status: filters.status === "all" ? undefined : filters.status,
    edgeId: filters.edgeId.trim() || undefined,
  };
}

function getStatusMeta(status: EdgeStatus) {
  return status === "ONLINE"
    ? { label: "在线", tone: "success" as const }
    : { label: "离线", tone: "neutral" as const };
}

export function EdgePage() {
  const {
    data: edges,
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
    reload,
  } = useListPage<FilterState, EdgeRecord, EdgePageQuery>({
    fetch: getEdgePage,
    defaultFilters: DEFAULT_FILTERS,
    toQuery: (currentFilters, currentPage, currentPageSize) =>
      buildQuery(currentFilters, currentPage, currentPageSize),
    onError: (loadError) =>
      toast.error({
        title: "Edge 列表加载失败",
        description: getErrorMessage(
          loadError,
          "无法获取 Edge 列表，请稍后重试",
        ),
      }),
  });
  const [detailEdgeId, setDetailEdgeId] = useState<string | null>(null);

  const openDetail = useCallback((record: EdgeRecord) => {
    setDetailEdgeId(record.edgeId);
  }, []);

  const columns = useMemo<DataTableColumn<EdgeRecord>[]>(
    () => [
      {
        title: "Edge ID",
        dataIndex: "edgeId",
        width: 240,
        nowrap: true,
        render: (value) => (
          <span className="font-medium text-text-primary">
            {String(value ?? "-")}
          </span>
        ),
      },
      {
        title: "状态",
        dataIndex: "status",
        width: 120,
        nowrap: true,
        render: (value) => {
          const meta = getStatusMeta(value as EdgeStatus);
          return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
        },
      },
      {
        title: "首次登记时间（UTC）",
        dataIndex: "registeredAt",
        width: 240,
        nowrap: true,
        render: (value) => (
          <span className="whitespace-nowrap tabular-nums">
            {formatDateTime(String(value ?? ""))}
          </span>
        ),
      },
      {
        title: "最近状态时间（UTC）",
        dataIndex: "lastSeenAt",
        width: 240,
        nowrap: true,
        render: (value) => (
          <span className="whitespace-nowrap tabular-nums">
            {formatDateTime(String(value ?? ""))}
          </span>
        ),
      },
      {
        title: "操作",
        key: "actions",
        width: 100,
        align: "center",
        nowrap: true,
        render: (_, record) => (
          <Button
            size="sm"
            variant="ghost"
            aria-label={`查看 ${record.edgeId} 详情`}
            onClick={() => openDetail(record)}
          >
            <Eye className="h-4 w-4" aria-hidden />
            详情
          </Button>
        ),
      },
    ],
    [openDetail],
  );

  return (
    <>
      <PageHeader
        title="Edge 管理"
        description="查看 Cloud 已发现的 Edge 及其最近状态。"
      />

      <form
        onSubmit={(event) => {
          event.preventDefault();
          submitFilters();
        }}
      >
        <SearchFilterBar
          actions={
            <>
              <Button type="button" variant="secondary" onClick={resetFilters}>
                <RotateCcw className="h-4 w-4" aria-hidden />
                重置
              </Button>
              <Button type="submit" variant="primary">
                <Search className="h-4 w-4" aria-hidden />
                查询
              </Button>
            </>
          }
        >
          <Input
            value={filters.edgeId}
            onChange={(event) => setFilter("edgeId", event.target.value)}
            placeholder="精确匹配 Edge ID"
            aria-label="Edge ID"
          />
          <Select
            value={filters.status}
            onChange={(event) =>
              setFilter("status", event.target.value as FilterState["status"])
            }
            aria-label="筛选状态"
          >
            <option value="all">全部状态</option>
            <option value="ONLINE">在线</option>
            <option value="OFFLINE">离线</option>
          </Select>
        </SearchFilterBar>
      </form>

      <DataTableCard>
        <TableToolbar
          title="Edge 列表"
          description={`共 ${total} 个 Edge。`}
          actions={
            <Button
              variant="secondary"
              size="sm"
              disabled={loading}
              onClick={() => reload()}
            >
              <RefreshCw className="h-4 w-4" aria-hidden />
              刷新
            </Button>
          }
        />
        <DataTable<EdgeRecord>
          columns={columns}
          dataSource={edges}
          rowKey="edgeId"
          loading={loading}
          error={
            error ? (
              <EmptyState
                title="Edge 列表加载失败"
                description={error}
                actionText="重试"
                onAction={() => reload()}
              />
            ) : undefined
          }
          minWidth={1040}
          onRowClick={openDetail}
          empty={
            <EmptyState
              title="暂无 Edge"
              description="Cloud 尚未发现任何 Edge，或当前筛选条件没有匹配结果。"
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

      <EdgeDetailDialog
        open={detailEdgeId !== null}
        edgeId={detailEdgeId}
        onCancel={() => setDetailEdgeId(null)}
      />
    </>
  );
}
