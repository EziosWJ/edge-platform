import { zodResolver } from "@hookform/resolvers/zod";
import { Plus, RefreshCw, RotateCcw, Search, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useForm } from "react-hook-form";
import {
  createMenu,
  batchDeleteMenus,
  deleteMenu,
  getMenuDetail,
  getMenuPage,
  getSystemMenuTree,
  updateMenu,
  updateMenuStatus,
} from "@/api/rbac";
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
  MENU_TYPE_OPTIONS,
  MENU_TYPE_VALUES,
  MENU_VISIBLE_OPTIONS,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import type {
  ApiStatus,
  SystemMenuRecord,
  SystemMenuTreeRecord,
  SystemMenuType,
} from "@/types";
import { createMenuColumns } from "./columns";
import { MenuFormDialog } from "./menu-form-dialog";
import {
  buildMenuPayload,
  buildQuery,
  collectDescendantIds,
  DEFAULT_MENU_FILTERS,
  flattenMenuTree,
  menuFormSchema,
  menuTreeToSelectNodes,
  toFormValues,
  type MenuFilterState,
  type MenuFormMode,
  type MenuFormValues,
  type MenuRow,
} from "./schema";

type ConfirmAction =
  | { type: "delete"; menu: SystemMenuRecord }
  | { type: "batchDelete"; menus: MenuRow[] }
  | { type: "status"; menu: SystemMenuRecord; status: ApiStatus };

export function SystemMenusPage() {
  const [filters, setFilters] = useState<MenuFilterState>(DEFAULT_MENU_FILTERS);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [appliedFilters, setAppliedFilters] =
    useState<MenuFilterState>(DEFAULT_MENU_FILTERS);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [queryVersion, setQueryVersion] = useState(0);
  const [menus, setMenus] = useState<SystemMenuRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<MenuFormMode>("create");
  const [editingMenu, setEditingMenu] = useState<SystemMenuRecord | null>(null);
  const [parentTree, setParentTree] = useState<SystemMenuTreeRecord[]>([]);
  const [treeLoading, setTreeLoading] = useState(false);
  const [formSubmitting, setFormSubmitting] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const editRequestId = useRef(0);

  const form = useForm<MenuFormValues>({
    resolver: zodResolver(menuFormSchema),
    defaultValues: toFormValues(),
  });
  const menuTypeDict = useDictOptions<SystemMenuType>(DICT_CODES.MENU_TYPE, {
    fallback: MENU_TYPE_OPTIONS,
    allowedValues: MENU_TYPE_VALUES,
    showErrorToast: true,
    errorTitle: "菜单类型字典加载失败",
  });
  const statusDict = useDictOptions<ApiStatus>(DICT_CODES.COMMON_STATUS, {
    fallback: COMMON_STATUS_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "菜单状态字典加载失败",
  });
  const visibleDict = useDictOptions<ApiStatus>(DICT_CODES.MENU_VISIBLE, {
    fallback: MENU_VISIBLE_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "菜单可见性字典加载失败",
  });

  const loadMenus = useCallback(async () => {
    setLoading(true);
    setError("");

    try {
      const data = await getMenuPage(buildQuery(appliedFilters, page, pageSize));
      setMenus(data.records);
      setTotal(data.total);
    } catch (loadError) {
      setMenus([]);
      setTotal(0);
      setError(getErrorMessage(loadError, "菜单列表加载失败"));
    } finally {
      setLoading(false);
    }
  }, [appliedFilters, page, pageSize]);

  const loadParentTree = useCallback(async () => {
    setTreeLoading(true);
    try {
      const data = await getSystemMenuTree();
      setParentTree(data);
    } catch (loadError) {
      setParentTree([]);
      toast.error({
        title: "菜单树加载失败",
        description: getErrorMessage(loadError, "无法获取父级菜单"),
      });
    } finally {
      setTreeLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadMenus();
  }, [loadMenus, queryVersion]);

  const submitFilters = (event?: FormEvent) => {
    event?.preventDefault();
    setPage(1);
    setAppliedFilters(filters);
    setQueryVersion((version) => version + 1);
  };

  const resetFilters = () => {
    setFilters(DEFAULT_MENU_FILTERS);
    setAppliedFilters(DEFAULT_MENU_FILTERS);
    setPage(1);
  };

  const openCreateForm = async (parentId = 0) => {
    setFormMode("create");
    setEditingMenu(null);
    form.reset(toFormValues(undefined, parentId));
    setFormOpen(true);
    await loadParentTree();
  };

  const openEditForm = async (menu: SystemMenuRecord) => {
    const requestId = ++editRequestId.current;
    setFormMode("edit");
    setEditingMenu(menu);
    form.reset(toFormValues(menu));
    setFormOpen(true);
    await loadParentTree();

    try {
      const detail = await getMenuDetail(menu.id);
      if (editRequestId.current !== requestId) return;
      const nextMenu = { ...menu, ...detail };
      setEditingMenu(nextMenu);
      form.reset(toFormValues(nextMenu));
    } catch (detailError) {
      if (editRequestId.current !== requestId) return;
      toast.error({
        title: "菜单详情加载失败",
        description: getErrorMessage(detailError, "无法获取菜单详情"),
      });
    }
  };

  const submitMenuForm = async (values: MenuFormValues) => {
    setFormSubmitting(true);

    try {
      if (formMode === "edit" && editingMenu) {
        await updateMenu(editingMenu.id, buildMenuPayload(values));
        toast.success("菜单已更新");
      } else {
        await createMenu(buildMenuPayload(values));
        toast.success("菜单已创建");
      }

      setFormOpen(false);
      await loadMenus();
    } catch (submitError) {
      if (isApiError(submitError) && submitError.fieldErrors) {
        Object.entries(submitError.fieldErrors).forEach(([field, message]) => {
          form.setError(field as keyof MenuFormValues, { message });
        });
      }

      toast.error({
        title: formMode === "edit" ? "更新失败" : "创建失败",
        description: getErrorMessage(submitError, "请检查表单后重试"),
      });
    } finally {
      setFormSubmitting(false);
    }
  };

  const runConfirmAction = async () => {
    if (!confirmAction) return;

    if (confirmAction.type === "delete" && confirmAction.menu.isBuiltin === 1) {
      toast.warning("内置菜单不允许删除");
      setConfirmAction(null);
      return;
    }

    setConfirmLoading(true);
    try {
      if (confirmAction.type === "delete") {
        await deleteMenu(confirmAction.menu.id);
        toast.success("菜单已删除");
      }

      if (confirmAction.type === "batchDelete") {
        await batchDeleteMenus({ ids: confirmAction.menus.map((item) => item.id) });
        toast.success("菜单已批量删除");
      }

      if (confirmAction.type === "status") {
        await updateMenuStatus(confirmAction.menu.id, {
          status: confirmAction.status,
        });
        toast.success(
          confirmAction.status === 1 ? "菜单已启用" : "菜单已禁用",
        );
      }

      setConfirmAction(null);
      setSelectedIds(new Set());
      await loadMenus();
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
        title: "删除菜单",
        description: `确认删除菜单「${confirmAction.menu.menuName}」吗？此操作不可恢复。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDelete") {
      return {
        title: "批量删除菜单",
        description: `确认删除已选择的 ${confirmAction.menus.length} 个普通菜单吗？内置菜单不会被选中。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    const enabled = confirmAction.status === 1;
    return {
      title: enabled ? "启用菜单" : "禁用菜单",
      description: `确认${enabled ? "启用" : "禁用"}菜单「${confirmAction.menu.menuName}」吗？`,
      confirmText: enabled ? "启用" : "禁用",
      danger: !enabled,
    };
  }, [confirmAction]);

  const rows = useMemo(() => flattenMenuTree(menus), [menus]);
  useEffect(() => {
    setSelectedIds((current) => {
      const recordIds = new Set(rows.map((item) => item.id));
      return new Set([...current].filter((id) => recordIds.has(id)));
    });
  }, [rows]);
  const selectableIds = useMemo(
    () => rows.filter((item) => item.isBuiltin !== 1).map((item) => item.id),
    [rows],
  );
  const allSelectableChecked =
    selectableIds.length > 0 && selectableIds.every((id) => selectedIds.has(id));
  const selectedMenus = useMemo(
    () => rows.filter((item) => selectedIds.has(item.id) && item.isBuiltin !== 1),
    [rows, selectedIds],
  );

  const toggleSelect = (id: number, checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const toggleSelectAll = (checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      selectableIds.forEach((id) => {
        if (checked) next.add(id);
        else next.delete(id);
      });
      return next;
    });
  };

  const parentNodes = useMemo(() => {
    const disabledIds = collectDescendantIds(editingMenu);
    if (editingMenu) disabledIds.add(editingMenu.id);

    return [
      {
        id: 0,
        label: "根目录",
        children: menuTreeToSelectNodes(parentTree, disabledIds),
      },
    ];
  }, [editingMenu, parentTree]);

  const columns = createMenuColumns({
    onEdit: (menu) => void openEditForm(menu),
    onCreateChild: (parentId) => void openCreateForm(parentId),
    onToggleStatus: (menu, status) =>
      setConfirmAction({ type: "status", menu, status }),
    onDelete: (menu) => setConfirmAction({ type: "delete", menu }),
    selectedIds,
    onToggleSelect: toggleSelect,
    onToggleSelectAll: toggleSelectAll,
    allSelectableChecked,
    selectableCount: selectableIds.length,
  });

  return (
    <>
      <PageHeader
        title="菜单管理"
        description="维护系统菜单结构、路由、外链、权限标识和展示状态。"
        actions={
          <Button variant="primary" onClick={() => void openCreateForm()}>
            <Plus className="h-4 w-4" aria-hidden />
            新建菜单
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
        <form className="contents" onSubmit={(event) => submitFilters(event)}>
          <div className="relative">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary"
              aria-hidden
            />
            <Input
              value={filters.menuName}
              onChange={(event) =>
                setFilters((current) => ({
                  ...current,
                  menuName: event.target.value,
                }))
              }
              placeholder="菜单名称"
              className="pl-9"
            />
          </div>
          <Select
            value={filters.menuType}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                menuType: event.target.value as MenuFilterState["menuType"],
              }))
            }
            aria-label="筛选菜单类型"
          >
            <option value="all">全部类型</option>
            {menuTypeDict.options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
          <Select
            value={String(filters.status)}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                status:
                  event.target.value === "all"
                    ? "all"
                    : (Number(event.target.value) as ApiStatus),
              }))
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
          <Select
            value={String(filters.visible)}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                visible:
                  event.target.value === "all"
                    ? "all"
                    : (Number(event.target.value) as ApiStatus),
              }))
            }
            aria-label="筛选可见性"
          >
            <option value="all">全部可见性</option>
            {visibleDict.options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </form>
      </SearchFilterBar>

      <DataTableCard
        toolbar={
          <TableToolbar
            title="菜单列表"
            description={`共 ${total} 条数据，当前显示 ${rows.length} 条。`}
            actions={
              <>
                <StatusTag tone={loading ? "warning" : error ? "error" : "info"}>
                  {loading ? "加载中" : error ? "加载失败" : "已同步"}
                </StatusTag>
                <Button size="sm" variant="secondary" onClick={loadMenus}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={selectedMenus.length === 0}
                  onClick={() => setConfirmAction({ type: "batchDelete", menus: selectedMenus })}
                >
                  <Trash2 className="h-4 w-4" aria-hidden />
                  批量删除
                </Button>
              </>
            }
          />
        }
        pagination={
          <Pagination
            page={page}
            pageSize={pageSize}
            total={total}
            disabled={loading}
            onPageChange={setPage}
            onPageSizeChange={(nextPageSize) => {
              setPageSize(nextPageSize);
              setPage(1);
            }}
          />
        }
      >
        <DataTable<MenuRow>
          columns={columns}
          dataSource={rows}
          rowKey="id"
          loading={loading}
          error={error}
          minWidth={1660}
          empty={
            <EmptyState
              title="暂无菜单数据"
              description="调整筛选条件后重新查询。"
              actionText="重置筛选"
              onAction={resetFilters}
            />
          }
        />
      </DataTableCard>

      <MenuFormDialog
        open={formOpen}
        mode={formMode}
        form={form}
        loading={formSubmitting}
        treeLoading={treeLoading}
        editingMenu={editingMenu}
        parentNodes={parentNodes}
        menuTypeOptions={menuTypeDict.options}
        visibleOptions={visibleDict.options}
        statusOptions={statusDict.options}
        onCancel={() => setFormOpen(false)}
        onSubmit={submitMenuForm}
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
