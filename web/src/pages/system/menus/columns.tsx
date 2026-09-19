import { Pencil, Plus, Trash2 } from "lucide-react";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { getMenuIcon } from "@/lib/menu-icons";
import type { ApiStatus, DataTableColumn, SystemMenuType } from "@/types";
import { typeLabelMap, type MenuRow } from "./schema";

type MenuColumnsHandlers = {
  onEdit: (menu: MenuRow) => void;
  onCreateChild: (parentId: number) => void;
  onToggleStatus: (menu: MenuRow, status: ApiStatus) => void;
  onDelete: (menu: MenuRow) => void;
  selectedIds: Set<number>;
  onToggleSelect: (id: number, checked: boolean) => void;
  onToggleSelectAll: (checked: boolean) => void;
  allSelectableChecked: boolean;
  selectableCount: number;
};

const statusMeta: Record<ApiStatus, { label: string; tone: "success" | "neutral" }> = {
  1: { label: "启用", tone: "success" },
  0: { label: "禁用", tone: "neutral" },
};

const visibleMeta: Record<ApiStatus, { label: string; tone: "info" | "neutral" }> = {
  1: { label: "显示", tone: "info" },
  0: { label: "隐藏", tone: "neutral" },
};

export function createMenuColumns({
  onEdit,
  onCreateChild,
  onToggleStatus,
  onDelete,
  selectedIds,
  onToggleSelect,
  onToggleSelectAll,
  allSelectableChecked,
  selectableCount,
}: MenuColumnsHandlers): DataTableColumn<MenuRow>[] {
  return [
    {
      title: (
        <Checkbox
          aria-label="选择当前页普通菜单"
          checked={allSelectableChecked}
          disabled={selectableCount === 0}
          onChange={(event) => onToggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, menu) => {
        const disabled = menu.isBuiltin === 1;
        return (
          <Checkbox
            aria-label={`选择菜单 ${menu.menuName}`}
            checked={selectedIds.has(menu.id)}
            disabled={disabled}
            title={disabled ? "内置菜单不参与批量删除" : undefined}
            onChange={(event) => onToggleSelect(menu.id, event.target.checked)}
          />
        );
      },
    },
    {
      title: "菜单名称",
      key: "menu",
      width: 260,
      render: (_, menu) => {
        const Icon = getMenuIcon(menu.icon);

        return (
          <div
            className="flex items-center gap-2"
            style={{ paddingLeft: menu.level * 20 }}
          >
            <span className="inline-flex h-control-sm w-8 shrink-0 items-center justify-center rounded-control border border-border bg-neutral-background text-text-secondary">
              <Icon className="h-4 w-4" aria-hidden />
            </span>
            <div className="min-w-0">
              <div className="truncate font-medium text-text-primary">
                {menu.menuName}
              </div>
              <div className="text-xs text-text-tertiary">
                ID {menu.id} · 父级 {menu.parentId}
              </div>
            </div>
          </div>
        );
      },
    },
    {
      title: "类型",
      dataIndex: "menuType",
      width: 96,
      render: (value) => typeLabelMap[value as SystemMenuType],
    },
    {
      title: "路由 / 外链",
      key: "path",
      width: 220,
      render: (_, menu) => (
        <span className="block max-w-[220px] truncate text-text-secondary">
          {menu.menuType === "LINK" ? menu.externalUrl || "-" : menu.path || "-"}
        </span>
      ),
    },
    {
      title: "组件路径",
      dataIndex: "component",
      width: 200,
      render: (value) => (
        <span className="block max-w-[200px] truncate text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "权限标识",
      dataIndex: "permissionCode",
      width: 190,
      render: (value) => (
        <span className="block max-w-[190px] truncate text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "可见性",
      dataIndex: "visible",
      width: 96,
      render: (value) => {
        const meta = visibleMeta[(value === 0 ? 0 : 1) as ApiStatus];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 96,
      render: (value) => {
        const meta = statusMeta[(value === 0 ? 0 : 1) as ApiStatus];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "属性",
      dataIndex: "isBuiltin",
      width: 96,
      render: (value) =>
        value === 1 ? (
          <StatusTag tone="info">内置</StatusTag>
        ) : (
          <StatusTag tone="neutral">普通</StatusTag>
        ),
    },
    {
      title: "排序",
      dataIndex: "sortOrder",
      align: "center",
      width: 80,
      render: (value) => (
        <span className="tabular-nums">{String(value ?? 0)}</span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 330,
      render: (_, menu) => (
        <div className="inline-flex flex-wrap items-center justify-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => onEdit(menu)}>
            <Pencil className="h-4 w-4" aria-hidden />
            编辑
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onCreateChild(menu.id)}
          >
            <Plus className="h-4 w-4" aria-hidden />
            子菜单
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onToggleStatus(menu, menu.status === 1 ? 0 : 1)}
          >
            {menu.status === 1 ? "禁用" : "启用"}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="text-error hover:text-error"
            disabled={menu.isBuiltin === 1}
            title={menu.isBuiltin === 1 ? "内置菜单不允许删除" : undefined}
            onClick={() => onDelete(menu)}
          >
            <Trash2 className="h-4 w-4" aria-hidden />
            删除
          </Button>
        </div>
      ),
    },
  ];
}
