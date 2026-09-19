import { zodResolver } from "@hookform/resolvers/zod";
import { Plus, RefreshCw, RotateCcw, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import {
  assignRoleMenus,
  batchDeleteRoles,
  createRole,
  deleteRole,
  getRoleDetail,
  getRolePage,
  getSystemMenuTree,
  updateRole,
  updateRoleStatus,
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
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { useListPage } from "@/hooks/use-list-page";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import type {
  ApiStatus,
  RoleDetailRecord,
  RoleListRecord,
  SystemMenuTreeRecord,
} from "@/types";
import { createRoleColumns } from "./columns";
import { RoleFormDialog } from "./role-form-dialog";
import { RoleMenuDialog } from "./role-menu-dialog";
import {
  buildQuery,
  buildRolePayload,
  DEFAULT_ROLE_FILTERS,
  menuTreeToCheckNodes,
  normalizeCheckedIds,
  roleFormSchema,
  toFormValues,
  type RoleFilterState,
  type RoleFormMode,
  type RoleFormValues,
} from "./schema";

type ConfirmAction =
  | { type: "delete"; role: RoleListRecord }
  | { type: "batchDelete"; roles: RoleListRecord[] }
  | { type: "status"; role: RoleListRecord; status: ApiStatus };

export function SystemRolesPage() {
  const {
    data: roles,
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
    reload: loadRoles,
  } = useListPage<RoleFilterState, RoleListRecord>({
    fetch: getRolePage,
    defaultFilters: DEFAULT_ROLE_FILTERS,
    toQuery: (f, p, ps) => buildQuery(f, p, ps),
    onError: (err) =>
      toast.error({
        title: "加载失败",
        description: getErrorMessage(err, "角色列表加载失败"),
      }),
  });

  const [formOpen, setFormOpen] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [formMode, setFormMode] = useState<RoleFormMode>("create");
  const [editingRole, setEditingRole] = useState<RoleListRecord | null>(null);
  const [formSubmitting, setFormSubmitting] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [assignmentRole, setAssignmentRole] = useState<RoleDetailRecord | null>(
    null,
  );
  const [menuTree, setMenuTree] = useState<SystemMenuTreeRecord[]>([]);
  const [selectedMenuIds, setSelectedMenuIds] = useState<number[]>([]);
  const [assignmentLoading, setAssignmentLoading] = useState(false);
  const [assignmentSubmitting, setAssignmentSubmitting] = useState(false);

  const editRequestId = useRef(0);

  const form = useForm<RoleFormValues>({
    resolver: zodResolver(roleFormSchema),
    defaultValues: toFormValues(),
  });
  const statusDict = useDictOptions<ApiStatus>(DICT_CODES.COMMON_STATUS, {
    fallback: COMMON_STATUS_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "角色状态字典加载失败",
  });

  useEffect(() => {
    setSelectedIds((current) => {
      const recordIds = new Set(roles.map((item) => item.id));
      return new Set([...current].filter((id) => recordIds.has(id)));
    });
  }, [roles]);

  const selectableIds = useMemo(
    () => roles.filter((item) => item.isBuiltin !== 1).map((item) => item.id),
    [roles],
  );
  const allSelectableChecked =
    selectableIds.length > 0 && selectableIds.every((id) => selectedIds.has(id));
  const selectedRoles = useMemo(
    () => roles.filter((item) => selectedIds.has(item.id) && item.isBuiltin !== 1),
    [roles, selectedIds],
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

  const openCreateForm = () => {
    setFormMode("create");
    setEditingRole(null);
    form.reset(toFormValues());
    setFormOpen(true);
  };

  const openEditForm = async (role: RoleListRecord) => {
    const requestId = ++editRequestId.current;
    setFormMode("edit");
    setEditingRole(role);
    form.reset(toFormValues(role));
    setFormOpen(true);

    try {
      const detail = await getRoleDetail(role.id);
      if (editRequestId.current !== requestId) return;
      setEditingRole(detail);
      form.reset(toFormValues(detail));
    } catch (detailError) {
      if (editRequestId.current !== requestId) return;
      toast.error({
        title: "角色详情加载失败",
        description: getErrorMessage(detailError, "无法获取角色详情"),
      });
    }
  };

  const submitRoleForm = async (values: RoleFormValues) => {
    setFormSubmitting(true);

    try {
      if (formMode === "edit" && editingRole) {
        await updateRole(editingRole.id, buildRolePayload(values));
        toast.success("角色已更新");
      } else {
        await createRole(buildRolePayload(values));
        toast.success("角色已创建");
      }

      setFormOpen(false);
      await loadRoles();
    } catch (submitError) {
      if (isApiError(submitError) && submitError.fieldErrors) {
        Object.entries(submitError.fieldErrors).forEach(([field, message]) => {
          form.setError(field as keyof RoleFormValues, { message });
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

  const openAssignment = async (role: RoleListRecord) => {
    setAssignmentRole({ ...role, menuIds: [] });
    setMenuTree([]);
    setSelectedMenuIds([]);
    setAssignmentLoading(true);

    try {
      const [detail, menus] = await Promise.all([
        getRoleDetail(role.id),
        getSystemMenuTree(),
      ]);
      setAssignmentRole(detail);
      setMenuTree(menus);
      setSelectedMenuIds(detail.menuIds ?? []);
    } catch (loadError) {
      toast.error({
        title: "菜单权限加载失败",
        description: getErrorMessage(loadError, "无法获取角色菜单信息"),
      });
    } finally {
      setAssignmentLoading(false);
    }
  };

  const closeAssignment = () => {
    setAssignmentRole(null);
    setMenuTree([]);
    setSelectedMenuIds([]);
  };

  const submitAssignment = async () => {
    if (!assignmentRole) return;

    setAssignmentSubmitting(true);

    try {
      await assignRoleMenus(assignmentRole.id, { menuIds: selectedMenuIds });
      toast.success("菜单权限已保存");
      closeAssignment();
    } catch (submitError) {
      toast.error({
        title: "保存失败",
        description: getErrorMessage(submitError, "请稍后重试"),
      });
    } finally {
      setAssignmentSubmitting(false);
    }
  };

  const runConfirmAction = async () => {
    if (!confirmAction) return;

    if (confirmAction.type === "delete" && confirmAction.role.isBuiltin === 1) {
      toast.warning("内置角色不允许删除");
      setConfirmAction(null);
      return;
    }

    setConfirmLoading(true);
    try {
      if (confirmAction.type === "delete") {
        await deleteRole(confirmAction.role.id);
        toast.success("角色已删除");
      }

      if (confirmAction.type === "batchDelete") {
        await batchDeleteRoles({ ids: confirmAction.roles.map((item) => item.id) });
        toast.success("角色已批量删除");
      }

      if (confirmAction.type === "status") {
        await updateRoleStatus(confirmAction.role.id, {
          status: confirmAction.status,
        });
        toast.success(
          confirmAction.status === 1 ? "角色已启用" : "角色已禁用",
        );
      }

      setConfirmAction(null);
      setSelectedIds(new Set());
      await loadRoles();
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
        title: "删除角色",
        description: `确认删除角色「${confirmAction.role.roleName}」吗？此操作不可恢复。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDelete") {
      return {
        title: "批量删除角色",
        description: `确认删除已选择的 ${confirmAction.roles.length} 个普通角色吗？内置角色不会被选中。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    const enabled = confirmAction.status === 1;
    return {
      title: enabled ? "启用角色" : "禁用角色",
      description: `确认${enabled ? "启用" : "禁用"}角色「${confirmAction.role.roleName}」吗？`,
      confirmText: enabled ? "启用" : "禁用",
      danger: !enabled,
    };
  }, [confirmAction]);

  const menuNodes = useMemo(() => menuTreeToCheckNodes(menuTree), [menuTree]);

  const columns = createRoleColumns({
    onEdit: (role) => void openEditForm(role),
    onAssignMenus: (role) => void openAssignment(role),
    onToggleStatus: (role, status) =>
      setConfirmAction({ type: "status", role, status }),
    onDelete: (role) => setConfirmAction({ type: "delete", role }),
    selectedIds,
    onToggleSelect: toggleSelect,
    onToggleSelectAll: toggleSelectAll,
    allSelectableChecked,
    selectableCount: selectableIds.length,
  });

  return (
    <>
      <PageHeader
        title="角色管理"
        description="维护系统角色、状态和菜单权限分配。"
        actions={
          <Button variant="primary" onClick={openCreateForm}>
            <Plus className="h-4 w-4" aria-hidden />
            新建角色
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
            value={filters.roleName}
            onChange={(event) => setFilter("roleName", event.target.value)}
            placeholder="角色名称"
          />
          <Input
            value={filters.roleCode}
            onChange={(event) => setFilter("roleCode", event.target.value)}
            placeholder="角色编码"
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

      <DataTableCard
        toolbar={
          <TableToolbar
            title="角色列表"
            description={`共 ${total} 条数据，当前显示 ${roles.length} 条。`}
            actions={
              <>
                <StatusTag tone={loading ? "warning" : error ? "error" : "info"}>
                  {loading ? "加载中" : error ? "加载失败" : "已同步"}
                </StatusTag>
                <Button size="sm" variant="secondary" onClick={loadRoles}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={selectedRoles.length === 0}
                  onClick={() => setConfirmAction({ type: "batchDelete", roles: selectedRoles })}
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
            onPageSizeChange={setPageSize}
          />
        }
      >
        <DataTable<RoleListRecord>
          columns={columns}
          dataSource={roles}
          rowKey="id"
          loading={loading}
          error={error}
          minWidth={1320}
          empty={
            <EmptyState
              title="暂无角色数据"
              description="调整筛选条件后重新查询。"
              actionText="重置筛选"
              onAction={resetFilters}
            />
          }
        />
      </DataTableCard>

      <RoleFormDialog
        open={formOpen}
        mode={formMode}
        form={form}
        loading={formSubmitting}
        editingRole={editingRole}
        statusOptions={statusDict.options}
        onCancel={() => setFormOpen(false)}
        onSubmit={submitRoleForm}
      />

      <RoleMenuDialog
        role={assignmentRole}
        nodes={menuNodes}
        checkedIds={selectedMenuIds}
        loading={assignmentLoading}
        submitting={assignmentSubmitting}
        onCheckedChange={(ids) => setSelectedMenuIds(normalizeCheckedIds(ids))}
        onCancel={closeAssignment}
        onSubmit={submitAssignment}
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
