import { useCallback, useEffect, useState } from "react";
import { Plus, RotateCcw, Search } from "lucide-react";
import { getPermissionTotal, getPermissions } from "@/api/rbac";
import { PermissionGuard } from "@/components/auth/permission-guard";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type {
  DataTableColumn,
  PermissionRecord,
  PermissionStatus,
  PermissionType,
} from "@/types";

const statusMeta = {
  enabled: { label: "启用", tone: "success" },
  disabled: { label: "停用", tone: "neutral" },
} as const;

const typeLabelMap: Record<PermissionType, string> = {
  page: "页面",
  button: "按钮",
  api: "接口",
  data: "数据",
};

export function SystemPermissionsPage() {
  const [keyword, setKeyword] = useState("");
  const [type, setType] = useState<PermissionType | "all">("all");
  const [status, setStatus] = useState<PermissionStatus | "all">("all");
  const [permissions, setPermissions] = useState<PermissionRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const total = getPermissionTotal();

  const loadPermissions = useCallback(async () => {
    setLoading(true);
    const data = await getPermissions({ keyword, type, status });
    setPermissions(data);
    setLoading(false);
  }, [keyword, status, type]);

  useEffect(() => {
    void loadPermissions();
  }, [loadPermissions]);

  const reset = () => {
    setKeyword("");
    setType("all");
    setStatus("all");
  };

  const columns: DataTableColumn<PermissionRecord>[] = [
    {
      title: "权限名称",
      key: "name",
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.name}</div>
          <div className="text-xs text-text-tertiary">{record.code}</div>
        </div>
      ),
    },
    {
      title: "权限编码",
      dataIndex: "code",
      width: 200,
    },
    {
      title: "所属菜单",
      dataIndex: "menuName",
      width: 140,
    },
    {
      title: "权限类型",
      dataIndex: "type",
      width: 100,
      render: (value) => typeLabelMap[value as PermissionType],
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 100,
      render: (value) => {
        const meta = statusMeta[value as PermissionStatus];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "排序",
      dataIndex: "sort",
      align: "center",
      width: 90,
    },
    {
      title: "描述",
      dataIndex: "description",
      width: 220,
      render: (value) => (
        <span className="block max-w-[220px] truncate text-text-secondary">
          {String(value ?? "-")}
        </span>
      ),
    },
    {
      title: "更新时间",
      dataIndex: "updatedAt",
      width: 160,
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 220,
      render: () => (
        <div className="inline-flex flex-wrap items-center gap-2">
          <Button size="sm" variant="ghost">
            编辑
          </Button>
          <Button size="sm" variant="ghost">
            删除
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="权限点管理"
        description="维护系统权限点，当前使用前端 mock 数据，不做真实权限控制。"
        actions={
          <PermissionGuard permissionCode="system:permission:add">
            <Button variant="primary">
              <Plus className="h-4 w-4" aria-hidden />
              新增权限点
            </Button>
          </PermissionGuard>
        }
      />

      <SearchFilterBar
        actions={
          <Button variant="secondary" onClick={reset}>
            <RotateCcw className="h-4 w-4" aria-hidden />
            重置
          </Button>
        }
      >
        <div className="relative">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary"
            aria-hidden
          />
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="搜索权限名称、编码、菜单或描述"
            className="pl-9"
          />
        </div>
        <Select
          value={type}
          onChange={(event) =>
            setType(event.target.value as PermissionType | "all")
          }
          aria-label="筛选权限类型"
        >
          <option value="all">全部类型</option>
          <option value="page">页面</option>
          <option value="button">按钮</option>
          <option value="api">接口</option>
          <option value="data">数据</option>
        </Select>
        <Select
          value={status}
          onChange={(event) =>
            setStatus(event.target.value as PermissionStatus | "all")
          }
          aria-label="筛选状态"
        >
          <option value="all">全部状态</option>
          <option value="enabled">启用</option>
          <option value="disabled">停用</option>
        </Select>
      </SearchFilterBar>

      <DataTableCard>
        <TableToolbar
          title="权限点列表"
          description={`共 ${total} 条 mock 数据，当前显示 ${permissions.length} 条。`}
          actions={
            <StatusTag tone={loading ? "warning" : "info"}>
              {loading ? "加载中" : "加载完成"}
            </StatusTag>
          }
        />
        <DataTable<PermissionRecord>
          columns={columns}
          dataSource={permissions}
          rowKey="id"
          loading={loading}
          minWidth={1560}
          empty={
            <EmptyState
              title="暂无权限点"
              description="调整筛选条件后重新查询。"
              actionText="重置筛选"
              onAction={reset}
            />
          }
        />
        <div className="flex items-center justify-between border-t border-border px-5 py-3 text-sm text-text-tertiary">
          <span>第 1 页 / 共 1 页</span>
          <div className="flex items-center gap-2">
            <Button size="sm" disabled>
              上一页
            </Button>
            <Button size="sm" disabled>
              下一页
            </Button>
          </div>
        </div>
      </DataTableCard>
    </>
  );
}
