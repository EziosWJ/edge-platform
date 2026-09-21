import { Eye, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { getDevicePage } from "@/api/device";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { useListPage } from "@/hooks/use-list-page";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type {
  DataTableColumn,
  DeviceCommunicationStatus,
  DevicePageQuery,
  DeviceRecord,
} from "@/types";
import { DeviceDetailDialog } from "./device-detail-dialog";

type FilterState = {
  edgeId: string;
  deviceId: string;
  sourceDeviceId: string;
  status: "all" | DeviceCommunicationStatus;
};

const STATUS_OPTIONS: Array<{ value: DeviceCommunicationStatus; label: string }> = [
  { value: "INITIAL", label: "初始" },
  { value: "ONLINE", label: "在线" },
  { value: "DEGRADED", label: "降级" },
  { value: "OFFLINE", label: "离线" },
];

function buildQuery(filters: FilterState, page: number, pageSize: number) {
  return {
    page,
    pageSize,
    edgeId: filters.edgeId.trim() || undefined,
    deviceId: filters.deviceId.trim() || undefined,
    sourceDeviceId: filters.sourceDeviceId.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
  };
}

function getStatusMeta(status: DeviceCommunicationStatus) {
  return STATUS_OPTIONS.find((option) => option.value === status) ?? STATUS_OPTIONS[0];
}

const EMPTY_FILTERS: FilterState = {
  edgeId: "",
  deviceId: "",
  sourceDeviceId: "",
  status: "all",
};

function initialFilters(): FilterState {
  const params = new URLSearchParams(window.location.search);
  return { ...EMPTY_FILTERS, edgeId: params.get("edgeId") ?? "" };
}

export function DevicePage() {
  const defaultFilters = useMemo(initialFilters, []);
  const {
    data: devices,
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
  } = useListPage<FilterState, DeviceRecord, DevicePageQuery>({
    fetch: getDevicePage,
    defaultFilters,
    toQuery: (currentFilters, currentPage, currentPageSize) =>
      buildQuery(currentFilters, currentPage, currentPageSize),
    onError: (loadError) => {
      // The shared list hook owns the visible error state; this callback keeps
      // the page-specific fallback copy concise and user-facing.
      void loadError;
    },
  });
  const [detailDeviceId, setDetailDeviceId] = useState<string | null>(null);

  const openDetail = useCallback((record: DeviceRecord) => {
    setDetailDeviceId(record.deviceId);
  }, []);

  const columns = useMemo<DataTableColumn<DeviceRecord>[]>(
    () => [
      {
        title: "Cloud Device ID",
        dataIndex: "deviceId",
        width: 290,
        nowrap: true,
        render: (value) => (
          <span className="font-medium text-text-primary">{String(value ?? "-")}</span>
        ),
      },
      {
        title: "来源身份",
        key: "sourceIdentity",
        width: 280,
        render: (_, record) => (
          <div className="min-w-56">
            <div className="text-sm text-text-primary">{record.sourceDeviceId}</div>
            <div className="mt-1 text-xs text-text-tertiary">Edge · {record.edgeId}</div>
          </div>
        ),
      },
      {
        title: "通信状态",
        dataIndex: "communicationStatus",
        width: 120,
        nowrap: true,
        render: (value) => {
          const meta = getStatusMeta(value as DeviceCommunicationStatus);
          const tone =
            value === "ONLINE"
              ? "success"
              : value === "DEGRADED"
                ? "warning"
                : value === "OFFLINE"
                  ? "error"
                  : "info";
          return <StatusTag tone={tone}>{meta.label}</StatusTag>;
        },
      },
      {
        title: "首次登记时间（UTC）",
        dataIndex: "registeredAt",
        width: 220,
        nowrap: true,
        render: (value) => (
          <span className="whitespace-nowrap tabular-nums">{formatDateTime(String(value ?? ""))}</span>
        ),
      },
      {
        title: "最近状态时间（UTC）",
        dataIndex: "lastSeenAt",
        width: 220,
        nowrap: true,
        render: (value) => (
          <span className="whitespace-nowrap tabular-nums">{formatDateTime(String(value ?? ""))}</span>
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
            aria-label={`查看 ${record.deviceId} 详情`}
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
        title="Device 管理"
        description="查看 Cloud 已发现的设备、来源身份和最近通信状态。"
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
          <Input
            value={filters.deviceId}
            onChange={(event) => setFilter("deviceId", event.target.value)}
            placeholder="精确匹配 Cloud Device ID"
            aria-label="Cloud Device ID"
          />
          <Input
            value={filters.sourceDeviceId}
            onChange={(event) => setFilter("sourceDeviceId", event.target.value)}
            placeholder="精确匹配来源 Device ID"
            aria-label="来源 Device ID"
          />
          <Select
            value={filters.status}
            onChange={(event) =>
              setFilter("status", event.target.value as FilterState["status"])
            }
            aria-label="筛选通信状态"
          >
            <option value="all">全部状态</option>
            {STATUS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </SearchFilterBar>
      </form>

      <DataTableCard>
        <TableToolbar
          title="Device 列表"
          description={`共 ${total} 个 Device。`}
          actions={
            <Button variant="secondary" size="sm" disabled={loading} onClick={() => reload()}>
              <RefreshCw className="h-4 w-4" aria-hidden />
              刷新
            </Button>
          }
        />
        <DataTable<DeviceRecord>
          columns={columns}
          dataSource={devices}
          rowKey="deviceId"
          loading={loading}
          error={
            error ? (
              <EmptyState
                title="Device 列表加载失败"
                description={getErrorMessage(error, "无法获取 Device 列表，请稍后重试")}
                actionText="重试"
                onAction={() => reload()}
              />
            ) : undefined
          }
          minWidth={1280}
          onRowClick={openDetail}
          empty={
            <EmptyState
              title="暂无 Device"
              description="Cloud 尚未发现任何 Device，或当前筛选条件没有匹配结果。"
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

      <DeviceDetailDialog
        open={detailDeviceId !== null}
        deviceId={detailDeviceId}
        onCancel={() => setDetailDeviceId(null)}
      />
    </>
  );
}
