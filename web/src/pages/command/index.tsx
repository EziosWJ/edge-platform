import { ClipboardList, Eye, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { getCommandPage } from "@/api/command";
import { PermissionGuard } from "@/components/auth/permission-guard";
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
import { hasPermission } from "@/lib/permission";
import type { CommandPageQuery, CommandRecord, CommandStatus, DataTableColumn } from "@/types";
import { CommandDetailDialog } from "./command-detail-dialog";

type FilterState = {
  commandId: string;
  deviceId: string;
  name: string;
  status: "all" | CommandStatus;
  requestedBy: string;
};

const EMPTY_FILTERS: FilterState = {
  commandId: "",
  deviceId: "",
  name: "",
  status: "all",
  requestedBy: "",
};

const STATUS_OPTIONS: Array<{ value: CommandStatus; label: string }> = [
  { value: "PENDING", label: "等待执行" },
  { value: "ACCEPTED", label: "边缘已接收" },
  { value: "REJECTED", label: "已拒绝" },
  { value: "EXPIRED", label: "已过期" },
  { value: "SUCCEEDED", label: "已成功" },
  { value: "FAILED", label: "执行失败" },
];

function buildQuery(filters: FilterState, page: number, pageSize: number): CommandPageQuery {
  const requestedBy = filters.requestedBy.trim();
  return {
    page,
    pageSize,
    commandId: filters.commandId.trim() || undefined,
    deviceId: filters.deviceId.trim() || undefined,
    name: filters.name.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
    requestedBy: requestedBy ? Number(requestedBy) : undefined,
  };
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

function initialFilters(): FilterState {
  const params = new URLSearchParams(window.location.search);
  return {
    ...EMPTY_FILTERS,
    commandId: params.get("commandId") ?? "",
    deviceId: params.get("deviceId") ?? "",
  };
}

export function CommandPage() {
  const defaultFilters = useMemo(initialFilters, []);
  const [detailCommandId, setDetailCommandId] = useState<string | null>(null);
  const canViewDetail = hasPermission("command:detail");
  const {
    data: commands,
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
  } = useListPage<FilterState, CommandRecord, CommandPageQuery>({
    fetch: getCommandPage,
    defaultFilters,
    toQuery: buildQuery,
    onError: (loadError) => {
      toast.error({ title: "Command 列表加载失败", description: getErrorMessage(loadError, "请稍后重试") });
    },
  });

  const openDetail = useCallback((record: CommandRecord) => {
    setDetailCommandId(record.commandId);
  }, []);

  const columns = useMemo<DataTableColumn<CommandRecord>[]>(
    () => [
      {
        title: "Command ID",
        dataIndex: "commandId",
        width: 300,
        nowrap: true,
        render: (value) => <span className="font-mono text-xs text-text-primary">{String(value ?? "-")}</span>,
      },
      {
        title: "目标 Device",
        key: "device",
        width: 270,
        render: (_, record) => (
          <div className="min-w-52">
            <div className="font-medium text-text-primary">{record.deviceId}</div>
            <div className="mt-1 text-xs text-text-tertiary">{record.name} · {record.edgeId}</div>
          </div>
        ),
      },
      {
        title: "状态",
        dataIndex: "status",
        width: 120,
        nowrap: true,
        render: (value) => {
          const meta = statusMeta(value as CommandStatus);
          return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
        },
      },
      {
        title: "Cloud 时间",
        key: "cloudTimes",
        width: 230,
        render: (_, record) => (
          <div className="whitespace-nowrap text-xs tabular-nums text-text-secondary">
            <div>签发 {formatDateTime(record.issuedAt)}</div>
            <div className="mt-1 text-text-tertiary">到期 {formatDateTime(record.expiresAt)}</div>
          </div>
        ),
      },
      {
        title: "结果时间",
        key: "resultTimes",
        width: 230,
        render: (_, record) => (
          <div className="whitespace-nowrap text-xs tabular-nums text-text-secondary">
            <div>Edge 接收 {formatDateTime(record.edgeReceivedAt)}</div>
            <div className="mt-1 text-text-tertiary">Cloud 收到 {formatDateTime(record.resultReceivedAt)}</div>
          </div>
        ),
      },
      {
        title: "操作",
        key: "actions",
        width: 100,
        align: "center",
        nowrap: true,
        render: (_, record) => (
          <PermissionGuard permissionCode="command:detail">
            <Button size="sm" variant="ghost" onClick={() => openDetail(record)}>
              <Eye className="h-4 w-4" aria-hidden />详情
            </Button>
          </PermissionGuard>
        ),
      },
    ],
    [openDetail],
  );

  return (
    <PermissionGuard
      permissionCode="command:list"
      fallback={<EmptyState title="没有 Command 查询权限" description="请联系管理员开通 Command 查询权限。" />}
    >
      <PageHeader
        title="Command 管理"
        description="以 Cloud 视角查看控制意图、执行状态和结果时间线。"
        actions={
          <div className="flex items-center gap-space-2">
            <ClipboardList className="h-5 w-5 text-primary" aria-hidden />
            <span className="text-sm text-text-tertiary">审计台</span>
          </div>
        }
      />
      <form onSubmit={(event) => { event.preventDefault(); submitFilters(); }}>
        <SearchFilterBar
          actions={
            <>
              <Button type="button" variant="secondary" onClick={resetFilters}>
                <RotateCcw className="h-4 w-4" aria-hidden />重置
              </Button>
              <Button type="submit" variant="primary">
                <Search className="h-4 w-4" aria-hidden />查询
              </Button>
            </>
          }
        >
          <Input value={filters.commandId} onChange={(event) => setFilter("commandId", event.target.value)} placeholder="Command ID" aria-label="Command ID" />
          <Input value={filters.deviceId} onChange={(event) => setFilter("deviceId", event.target.value)} placeholder="Cloud Device ID" aria-label="Cloud Device ID" />
          <Input value={filters.name} onChange={(event) => setFilter("name", event.target.value)} placeholder="Command name" aria-label="Command name" />
          <Input value={filters.requestedBy} onChange={(event) => setFilter("requestedBy", event.target.value)} placeholder="请求用户 ID" inputMode="numeric" aria-label="请求用户 ID" />
          <Select value={filters.status} onChange={(event) => setFilter("status", event.target.value as FilterState["status"])} aria-label="筛选 Command 状态">
            <option value="all">全部状态</option>
            {STATUS_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </Select>
        </SearchFilterBar>
      </form>
      <DataTableCard>
        <TableToolbar
          title="Command 列表"
          description={`共 ${total} 条控制记录。状态来自 Edge Result，不由 delivery 是否存在推断。`}
          actions={<Button variant="secondary" size="sm" disabled={loading} onClick={() => reload()}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>}
        />
        <DataTable<CommandRecord>
          columns={columns}
          dataSource={commands}
          rowKey="commandId"
          loading={loading}
          onRowClick={canViewDetail ? openDetail : undefined}
          error={error ? <EmptyState title="Command 列表加载失败" description={error} actionText="重试" onAction={() => reload()} /> : undefined}
          minWidth={1320}
          empty={<EmptyState title="暂无 Command" description="当前筛选条件没有匹配的 Cloud 控制记录。" actionText="重置筛选" onAction={resetFilters} />}
        />
        <Pagination page={page} pageSize={pageSize} total={total} disabled={loading} onPageChange={setPage} onPageSizeChange={setPageSize} />
      </DataTableCard>
      <CommandDetailDialog open={detailCommandId !== null} commandId={detailCommandId} onCancel={() => setDetailCommandId(null)} />
    </PermissionGuard>
  );
}
