import { zodResolver } from "@hookform/resolvers/zod";
import { Plus, RefreshCw, RotateCcw, Search, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import {
  createDept,
  batchDeleteDepts,
  deleteDept,
  getDeptDetail,
  getDeptOptions,
  getDeptPage,
  updateDept,
  updateDeptStatus,
} from "@/api/dept";
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
import { isApiError } from "@/lib/api-error";
import type { ApiStatus, DeptOption, DeptRecord } from "@/types";
import { createDeptColumns } from "./columns";
import { DeptFormDialog } from "./dept-form-dialog";
import {
  buildDeptPayload,
  buildQuery,
  DEFAULT_FILTERS,
  deptFormSchema,
  toFormValues,
  type DeptFormMode,
  type DeptFormValues,
  type FilterState,
} from "./schema";
import { getErrorMessage } from "@/lib/api-error";

type ConfirmAction =
  | { type: "delete"; dept: DeptRecord }
  | { type: "batchDelete"; depts: DeptRecord[] }
  | { type: "status"; dept: DeptRecord; status: ApiStatus };

export function SystemDeptsPage() {
  const {
    data: depts,
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
    reload: loadDepts,
  } = useListPage<FilterState, DeptRecord>({
    fetch: getDeptPage,
    defaultFilters: DEFAULT_FILTERS,
    toQuery: (f, p, ps) => buildQuery(f, p, ps),
    onError: (err) =>
      toast.error({
        title: "加载失败",
        description: getErrorMessage(err, "部门列表加载失败"),
      }),
  });

  const [deptOptions, setDeptOptions] = useState<DeptOption[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [optionsLoading, setOptionsLoading] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<DeptFormMode>("create");
  const [editingDept, setEditingDept] = useState<DeptRecord | null>(null);
  const [formSubmitting, setFormSubmitting] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const form = useForm<DeptFormValues>({
    resolver: zodResolver(deptFormSchema),
    defaultValues: toFormValues(),
  });
  const statusDict = useDictOptions<ApiStatus>(DICT_CODES.COMMON_STATUS, {
    fallback: COMMON_STATUS_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "部门状态字典加载失败",
  });

  const loadDeptOptions = useCallback(async () => {
    setOptionsLoading(true);

    try {
      const data = await getDeptOptions();
      setDeptOptions(data);
    } catch (loadError) {
      setDeptOptions([]);
      toast.error({
        title: "部门树加载失败",
        description: getErrorMessage(loadError, "无法获取部门选择树"),
      });
    } finally {
      setOptionsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadDeptOptions();
  }, [loadDeptOptions]);

  useEffect(() => {
    setSelectedIds((current) => {
      const recordIds = new Set(depts.map((item) => item.id));
      return new Set([...current].filter((id) => recordIds.has(id)));
    });
  }, [depts]);

  const selectableIds = useMemo(
    () => depts.filter((item) => item.isBuiltin !== 1).map((item) => item.id),
    [depts],
  );
  const allSelectableChecked =
    selectableIds.length > 0 && selectableIds.every((id) => selectedIds.has(id));
  const selectedDepts = useMemo(
    () => depts.filter((item) => selectedIds.has(item.id) && item.isBuiltin !== 1),
    [depts, selectedIds],
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
    setEditingDept(null);
    form.reset(toFormValues());
    setFormOpen(true);
    void loadDeptOptions();
  };

  const openEditForm = async (dept: DeptRecord) => {
    setFormMode("edit");
    setEditingDept(dept);
    form.reset(toFormValues(dept));
    setFormOpen(true);
    void loadDeptOptions();

    try {
      const detail = await getDeptDetail(dept.id);
      setEditingDept(detail);
      form.reset(toFormValues(detail));
    } catch (detailError) {
      toast.error({
        title: "部门详情加载失败",
        description: getErrorMessage(detailError, "无法获取部门详情"),
      });
    }
  };

  const submitDeptForm = async (values: DeptFormValues) => {
    setFormSubmitting(true);

    try {
      if (formMode === "edit" && editingDept) {
        await updateDept(editingDept.id, buildDeptPayload(values));
        toast.success("部门已更新");
      } else {
        await createDept(buildDeptPayload(values));
        toast.success("部门已创建");
      }

      setFormOpen(false);
      await Promise.all([loadDepts(), loadDeptOptions()]);
    } catch (submitError) {
      if (isApiError(submitError) && submitError.fieldErrors) {
        Object.entries(submitError.fieldErrors).forEach(([field, message]) => {
          form.setError(field as keyof DeptFormValues, { message });
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

    setConfirmLoading(true);
    try {
      if (confirmAction.type === "delete") {
        if (confirmAction.dept.isBuiltin === 1) {
          toast.warning("内置部门不允许删除");
          return;
        }

        await deleteDept(confirmAction.dept.id);
        toast.success("部门已删除");
      }

      if (confirmAction.type === "batchDelete") {
        await batchDeleteDepts({ ids: confirmAction.depts.map((item) => item.id) });
        toast.success("部门已批量删除");
      }

      if (confirmAction.type === "status") {
        await updateDeptStatus(confirmAction.dept.id, {
          status: confirmAction.status,
        });
        toast.success(
          confirmAction.status === 1 ? "部门已启用" : "部门已禁用",
        );
      }

      setConfirmAction(null);
      setSelectedIds(new Set());
      await Promise.all([loadDepts(), loadDeptOptions()]);
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
        title: "删除部门",
        description: `确认删除部门「${confirmAction.dept.deptName}」吗？此操作不可恢复。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDelete") {
      return {
        title: "批量删除部门",
        description: `确认删除已选择的 ${confirmAction.depts.length} 个普通部门吗？内置部门不会被选中。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    const enabled = confirmAction.status === 1;
    return {
      title: enabled ? "启用部门" : "禁用部门",
      description: `确认${enabled ? "启用" : "禁用"}部门「${confirmAction.dept.deptName}」吗？`,
      confirmText: enabled ? "启用" : "禁用",
      danger: !enabled,
    };
  }, [confirmAction]);

  const columns = createDeptColumns({
    onEdit: openEditForm,
    onChangeStatus: (dept, status) =>
      setConfirmAction({ type: "status", dept, status }),
    onDelete: (dept) => setConfirmAction({ type: "delete", dept }),
    selectedIds,
    onToggleSelect: toggleSelect,
    onToggleSelectAll: toggleSelectAll,
    allSelectableChecked,
    selectableCount: selectableIds.length,
  });

  return (
    <>
      <PageHeader
        title="部门管理"
        description="维护组织部门、负责人、状态和排序。"
        actions={
          <Button variant="primary" onClick={openCreateForm}>
            <Plus className="h-4 w-4" aria-hidden />
            新建部门
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
            value={filters.deptName}
            onChange={(event) => setFilter("deptName", event.target.value)}
            placeholder="部门名称"
          />
          <Input
            value={filters.deptCode}
            onChange={(event) => setFilter("deptCode", event.target.value)}
            placeholder="部门编码"
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
            title="部门列表"
            description={`共 ${total} 条数据，当前显示 ${depts.length} 条。`}
            actions={
              <>
                <StatusTag tone={loading ? "warning" : error ? "error" : "info"}>
                  {loading ? "加载中" : error ? "加载失败" : "已同步"}
                </StatusTag>
                <Button size="sm" variant="secondary" onClick={loadDepts}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={selectedDepts.length === 0}
                  onClick={() => setConfirmAction({ type: "batchDelete", depts: selectedDepts })}
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
        <DataTable<DeptRecord>
          columns={columns}
          dataSource={depts}
          rowKey="id"
          loading={loading}
          error={error}
          minWidth={1120}
          empty={
            <EmptyState
              title="暂无部门数据"
              description="调整筛选条件后重新查询。"
              actionText="重置筛选"
              onAction={resetFilters}
            />
          }
        />
      </DataTableCard>

      <DeptFormDialog
        open={formOpen}
        mode={formMode}
        form={form}
        loading={formSubmitting}
        optionsLoading={optionsLoading}
        editingDept={editingDept}
        deptOptions={deptOptions}
        statusOptions={statusDict.options}
        onCancel={() => setFormOpen(false)}
        onSubmit={submitDeptForm}
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
