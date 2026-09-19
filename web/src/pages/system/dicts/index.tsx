import { zodResolver } from "@hookform/resolvers/zod";
import {
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useForm } from "react-hook-form";
import {
  createDictData,
  createDictType,
  batchDeleteDictData,
  batchDeleteDictTypes,
  deleteDictData,
  deleteDictType,
  getDictDataPage,
  getDictTypeDetail,
  getDictTypePage,
  updateDictData,
  updateDictType,
  updateDictTypeStatus,
} from "@/api/system";
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
import { cn } from "@/lib/utils";
import type {
  ApiStatus,
  SystemDictDataRecord,
  SystemDictTypeRecord,
} from "@/types";
import { createDictDataColumns, createDictTypeColumns } from "./columns";
import { DictDataFormDialog } from "./dict-data-form-dialog";
import { DictTypeFormDialog } from "./dict-type-form-dialog";
import {
  buildDataPayload,
  buildDataQuery,
  buildTypePayload,
  buildTypeQuery,
  DEFAULT_ITEM_FILTERS,
  DEFAULT_TYPE_FILTERS,
  dictDataFormSchema,
  dictTypeFormSchema,
  toDataFormValues,
  toTypeFormValues,
  type DictDataFormValues,
  type DictTypeFormValues,
  type FormMode,
  type ItemFilterState,
  type TypeFilterState,
} from "./schema";
import { getErrorMessage } from "@/lib/api-error";

type ConfirmAction =
  | { type: "deleteType"; dictType: SystemDictTypeRecord }
  | { type: "batchDeleteTypes"; dictTypes: SystemDictTypeRecord[] }
  | { type: "status"; dictType: SystemDictTypeRecord; status: ApiStatus }
  | { type: "deleteData"; dictData: SystemDictDataRecord }
  | { type: "batchDeleteData"; dictData: SystemDictDataRecord[] };

export function SystemDictsPage() {
  // 类型列表 - useListPage
  const {
    data: dictTypes,
    total: typeTotal,
    loading: typeLoading,
    error: typeError,
    page: typePage,
    pageSize: typePageSize,
    setPage: setTypePage,
    setPageSize: setTypePageSize,
    filters: typeFilters,
    setFilter: setTypeFilter,
    submitFilters: submitTypeFilters,
    resetFilters: resetTypeFilters,
    reload: loadDictTypes,
  } = useListPage<TypeFilterState, SystemDictTypeRecord>({
    fetch: getDictTypePage,
    defaultFilters: DEFAULT_TYPE_FILTERS,
    toQuery: (f, p, ps) => buildTypeQuery(f, p, ps),
    onError: (err) =>
      toast.error({
        title: "加载失败",
        description: getErrorMessage(err, "字典类型加载失败"),
      }),
  });

  // 字典项列表 - 保留手动管理（依赖 selectedTypeId）
  const [itemFilters, setItemFilters] =
    useState<ItemFilterState>(DEFAULT_ITEM_FILTERS);
  const [appliedItemFilters, setAppliedItemFilters] =
    useState<ItemFilterState>(DEFAULT_ITEM_FILTERS);
  const [itemPage, setItemPage] = useState(1);
  const [itemPageSize, setItemPageSize] = useState(10);
  const [itemQueryVersion, setItemQueryVersion] = useState(0);
  const [dictItems, setDictItems] = useState<SystemDictDataRecord[]>([]);
  const [itemTotal, setItemTotal] = useState(0);
  const [activeType, setActiveType] = useState<SystemDictTypeRecord | null>(
    null,
  );
  const [itemLoading, setItemLoading] = useState(false);
  const [selectedTypeIds, setSelectedTypeIds] = useState<Set<number>>(new Set());
  const [selectedDataIds, setSelectedDataIds] = useState<Set<number>>(new Set());
  const [itemError, setItemError] = useState("");
  const [typeFormOpen, setTypeFormOpen] = useState(false);
  const [typeFormMode, setTypeFormMode] = useState<FormMode>("create");
  const [editingType, setEditingType] = useState<SystemDictTypeRecord | null>(
    null,
  );
  const [typeSubmitting, setTypeSubmitting] = useState(false);
  const [dataFormOpen, setDataFormOpen] = useState(false);
  const [dataFormMode, setDataFormMode] = useState<FormMode>("create");
  const [editingData, setEditingData] = useState<SystemDictDataRecord | null>(
    null,
  );
  const [dataSubmitting, setDataSubmitting] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const typeForm = useForm<DictTypeFormValues>({
    resolver: zodResolver(dictTypeFormSchema),
    defaultValues: toTypeFormValues(),
  });
  const dataForm = useForm<DictDataFormValues>({
    resolver: zodResolver(dictDataFormSchema),
    defaultValues: toDataFormValues(),
  });
  const statusDict = useDictOptions<ApiStatus>(DICT_CODES.COMMON_STATUS, {
    fallback: COMMON_STATUS_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "字典状态字典加载失败",
  });

  const selectedType = activeType;
  const selectedTypeId = activeType?.id ?? null;

  // 同步 activeType 与类型列表数据
  useEffect(() => {
    setActiveType((current) => {
      if (!current) return current;
      const next = dictTypes.find((item) => item.id === current.id);
      return next ?? current;
    });
  }, [dictTypes]);

  useEffect(() => {
    setSelectedTypeIds((current) => {
      const recordIds = new Set(dictTypes.map((item) => item.id));
      return new Set([...current].filter((id) => recordIds.has(id)));
    });
  }, [dictTypes]);

  useEffect(() => {
    setSelectedDataIds((current) => {
      const recordIds = new Set(dictItems.map((item) => item.id));
      return new Set([...current].filter((id) => recordIds.has(id)));
    });
  }, [dictItems]);

  const selectableTypeIds = useMemo(
    () => dictTypes.filter((item) => item.isBuiltin !== 1).map((item) => item.id),
    [dictTypes],
  );
  const allSelectableTypesChecked =
    selectableTypeIds.length > 0 &&
    selectableTypeIds.every((id) => selectedTypeIds.has(id));
  const selectedTypes = useMemo(
    () =>
      dictTypes.filter(
        (item) => selectedTypeIds.has(item.id) && item.isBuiltin !== 1,
      ),
    [dictTypes, selectedTypeIds],
  );
  const allDataChecked =
    dictItems.length > 0 && dictItems.every((item) => selectedDataIds.has(item.id));
  const selectedData = useMemo(
    () => dictItems.filter((item) => selectedDataIds.has(item.id)),
    [dictItems, selectedDataIds],
  );

  const toggleTypeSelect = (id: number, checked: boolean) => {
    setSelectedTypeIds((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const toggleTypeSelectAll = (checked: boolean) => {
    setSelectedTypeIds((current) => {
      const next = new Set(current);
      selectableTypeIds.forEach((id) => {
        if (checked) next.add(id);
        else next.delete(id);
      });
      return next;
    });
  };

  const toggleDataSelect = (id: number, checked: boolean) => {
    setSelectedDataIds((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const toggleDataSelectAll = (checked: boolean) => {
    setSelectedDataIds(checked ? new Set(dictItems.map((item) => item.id)) : new Set());
  };

  const loadDictItems = useCallback(async () => {
    if (!selectedTypeId) {
      setDictItems([]);
      setItemTotal(0);
      return;
    }

    setItemLoading(true);
    setItemError("");

    try {
      const data = await getDictDataPage(
        buildDataQuery(appliedItemFilters, selectedTypeId, itemPage, itemPageSize),
      );
      setDictItems(data.records);
      setItemTotal(data.total);
    } catch (loadError) {
      setDictItems([]);
      setItemTotal(0);
      setItemError(getErrorMessage(loadError, "字典项加载失败"));
    } finally {
      setItemLoading(false);
    }
  }, [appliedItemFilters, itemPage, itemPageSize, selectedTypeId]);

  useEffect(() => {
    void loadDictItems();
  }, [itemQueryVersion, loadDictItems]);

  const submitItemFilters = (event?: FormEvent) => {
    event?.preventDefault();
    setItemPage(1);
    setAppliedItemFilters(itemFilters);
    setItemQueryVersion((version) => version + 1);
  };

  const resetItemFilters = () => {
    setItemFilters(DEFAULT_ITEM_FILTERS);
    setAppliedItemFilters(DEFAULT_ITEM_FILTERS);
    setItemPage(1);
  };

  const selectType = (dictType: SystemDictTypeRecord) => {
    setActiveType(dictType);
    setSelectedDataIds(new Set());
    setItemPage(1);
    setItemFilters(DEFAULT_ITEM_FILTERS);
    setAppliedItemFilters(DEFAULT_ITEM_FILTERS);
  };

  const closeTypePanel = () => {
    setActiveType(null);
    setSelectedDataIds(new Set());
    setDictItems([]);
    setItemTotal(0);
    setItemError("");
    setItemFilters(DEFAULT_ITEM_FILTERS);
    setAppliedItemFilters(DEFAULT_ITEM_FILTERS);
    setItemPage(1);
  };

  const openCreateTypeForm = () => {
    setTypeFormMode("create");
    setEditingType(null);
    typeForm.reset(toTypeFormValues());
    setTypeFormOpen(true);
  };

  const openEditTypeForm = async (dictType: SystemDictTypeRecord) => {
    setTypeFormMode("edit");
    setEditingType(dictType);
    typeForm.reset(toTypeFormValues(dictType));
    setTypeFormOpen(true);

    try {
      const detail = await getDictTypeDetail(dictType.id);
      setEditingType(detail);
      typeForm.reset(toTypeFormValues(detail));
    } catch (detailError) {
      toast.error({
        title: "字典类型详情加载失败",
        description: getErrorMessage(detailError, "无法获取字典类型详情"),
      });
    }
  };

  const submitTypeForm = async (values: DictTypeFormValues) => {
    setTypeSubmitting(true);

    try {
      if (typeFormMode === "edit" && editingType) {
        await updateDictType(editingType.id, buildTypePayload(values));
        toast.success("字典类型已更新");
      } else {
        await createDictType(buildTypePayload(values));
        toast.success("字典类型已创建");
      }

      setTypeFormOpen(false);
      await loadDictTypes();
    } catch (submitError) {
      if (isApiError(submitError) && submitError.fieldErrors) {
        Object.entries(submitError.fieldErrors).forEach(([field, message]) => {
          typeForm.setError(field as keyof DictTypeFormValues, { message });
        });
      }

      toast.error({
        title: typeFormMode === "edit" ? "更新失败" : "创建失败",
        description: getErrorMessage(submitError, "请检查表单后重试"),
      });
    } finally {
      setTypeSubmitting(false);
    }
  };

  const openCreateDataForm = () => {
    if (!selectedType) {
      toast.warning("请先选择字典类型");
      return;
    }

    setDataFormMode("create");
    setEditingData(null);
    dataForm.reset(toDataFormValues());
    setDataFormOpen(true);
  };

  const openEditDataForm = (dictData: SystemDictDataRecord) => {
    setDataFormMode("edit");
    setEditingData(dictData);
    dataForm.reset(toDataFormValues(dictData));
    setDataFormOpen(true);
  };

  const submitDataForm = async (values: DictDataFormValues) => {
    const dictTypeId = editingData?.dictTypeId ?? selectedTypeId;
    if (!dictTypeId) return;

    setDataSubmitting(true);

    try {
      if (dataFormMode === "edit" && editingData) {
        await updateDictData(editingData.id, buildDataPayload(values, dictTypeId));
        toast.success("字典项已更新");
      } else {
        await createDictData(buildDataPayload(values, dictTypeId));
        toast.success("字典项已创建");
      }

      setDataFormOpen(false);
      await loadDictItems();
    } catch (submitError) {
      if (isApiError(submitError) && submitError.fieldErrors) {
        Object.entries(submitError.fieldErrors).forEach(([field, message]) => {
          dataForm.setError(field as keyof DictDataFormValues, { message });
        });
      }

      toast.error({
        title: dataFormMode === "edit" ? "更新失败" : "创建失败",
        description: getErrorMessage(submitError, "请检查表单后重试"),
      });
    } finally {
      setDataSubmitting(false);
    }
  };

  const runConfirmAction = async () => {
    if (!confirmAction) return;

    setConfirmLoading(true);

    try {
      if (confirmAction.type === "deleteType") {
        if (confirmAction.dictType.isBuiltin === 1) {
          toast.warning("内置字典不允许删除");
          return;
        }

        await deleteDictType(confirmAction.dictType.id);
        toast.success("字典类型已删除");
        if (selectedTypeId === confirmAction.dictType.id) {
          closeTypePanel();
        }
        await loadDictTypes();
      }

      if (confirmAction.type === "batchDeleteTypes") {
        await batchDeleteDictTypes({ ids: confirmAction.dictTypes.map((item) => item.id) });
        toast.success("字典类型已批量删除");
        if (selectedTypeId && confirmAction.dictTypes.some((item) => item.id === selectedTypeId)) {
          closeTypePanel();
        }
        setSelectedTypeIds(new Set());
        await loadDictTypes();
      }

      if (confirmAction.type === "status") {
        await updateDictTypeStatus(confirmAction.dictType.id, {
          status: confirmAction.status,
        });
        toast.success(
          confirmAction.status === 1 ? "字典类型已启用" : "字典类型已禁用",
        );
        await loadDictTypes();
      }

      if (confirmAction.type === "deleteData") {
        await deleteDictData(confirmAction.dictData.id);
        toast.success("字典项已删除");
        await loadDictItems();
      }

      if (confirmAction.type === "batchDeleteData") {
        await batchDeleteDictData({ ids: confirmAction.dictData.map((item) => item.id) });
        toast.success("字典项已批量删除");
        setSelectedDataIds(new Set());
        await loadDictItems();
      }

      setConfirmAction(null);
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

    if (confirmAction.type === "deleteType") {
      return {
        title: "删除字典类型",
        description: `确认删除字典类型「${confirmAction.dictType.dictName}」吗？下有字典项时后端会拒绝删除。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "deleteData") {
      return {
        title: "删除字典项",
        description: `确认删除字典项「${confirmAction.dictData.dictLabel}」吗？此操作不可恢复。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDeleteTypes") {
      return {
        title: "批量删除字典类型",
        description: `确认删除已选择的 ${confirmAction.dictTypes.length} 个普通字典类型吗？内置字典不会被选中。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDeleteData") {
      return {
        title: "批量删除字典项",
        description: `确认删除已选择的 ${confirmAction.dictData.length} 个字典项吗？此操作不可恢复。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    const enabled = confirmAction.status === 1;
    return {
      title: enabled ? "启用字典类型" : "禁用字典类型",
      description: `确认${enabled ? "启用" : "禁用"}字典类型「${confirmAction.dictType.dictName}」吗？`,
      confirmText: enabled ? "启用" : "禁用",
      danger: !enabled,
    };
  }, [confirmAction]);

  const typeColumns = createDictTypeColumns({
    onSelect: selectType,
    onEdit: openEditTypeForm,
    onChangeStatus: (dictType, status) =>
      setConfirmAction({ type: "status", dictType, status }),
    onDelete: (dictType) => setConfirmAction({ type: "deleteType", dictType }),
    selectedIds: selectedTypeIds,
    onToggleSelect: toggleTypeSelect,
    onToggleSelectAll: toggleTypeSelectAll,
    allSelectableChecked: allSelectableTypesChecked,
    selectableCount: selectableTypeIds.length,
  });

  const itemColumns = createDictDataColumns({
    onEdit: openEditDataForm,
    onDelete: (dictData) => setConfirmAction({ type: "deleteData", dictData }),
    selectedIds: selectedDataIds,
    onToggleSelect: toggleDataSelect,
    onToggleSelectAll: toggleDataSelectAll,
    allChecked: allDataChecked,
    selectableCount: dictItems.length,
  });

  return (
    <>
      <PageHeader
        title="字典管理"
        description="维护系统字典类型和字典项，数据来自后端接口。"
        actions={
          <Button variant="primary" onClick={openCreateTypeForm}>
            <Plus className="h-4 w-4" aria-hidden />
            新建字典
          </Button>
        }
      />

      <SearchFilterBar
        actions={
          <>
            <Button variant="secondary" onClick={resetTypeFilters}>
              <RotateCcw className="h-4 w-4" aria-hidden />
              重置
            </Button>
            <Button variant="primary" onClick={() => submitTypeFilters()}>
              <Search className="h-4 w-4" aria-hidden />
              查询
            </Button>
          </>
        }
      >
        <form className="contents" onSubmit={(event) => { event.preventDefault(); submitTypeFilters(); }}>
          <Input
            value={typeFilters.dictName}
            onChange={(event) => setTypeFilter("dictName", event.target.value)}
            placeholder="字典名称"
          />
          <Input
            value={typeFilters.dictCode}
            onChange={(event) => setTypeFilter("dictCode", event.target.value)}
            placeholder="字典编码"
          />
          <Select
            value={String(typeFilters.status)}
            onChange={(event) =>
              setTypeFilter(
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

      <div
        className={cn(
          "grid gap-6",
          selectedType
            ? "xl:grid-cols-[minmax(0,0.88fr)_minmax(420px,1.12fr)]"
            : "grid-cols-1",
        )}
      >
        <DataTableCard>
          <TableToolbar
            title="字典类型"
            description={`共 ${typeTotal} 条数据，当前显示 ${dictTypes.length} 条。`}
            actions={
              <>
                <StatusTag
                  tone={typeLoading ? "warning" : typeError ? "error" : "info"}
                >
                  {typeLoading ? "加载中" : typeError ? "加载失败" : "已同步"}
                </StatusTag>
                <Button size="sm" variant="secondary" onClick={loadDictTypes}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={selectedTypes.length === 0}
                  onClick={() => setConfirmAction({ type: "batchDeleteTypes", dictTypes: selectedTypes })}
                >
                  <Trash2 className="h-4 w-4" aria-hidden />
                  批量删除
                </Button>
              </>
            }
          />
          <DataTable<SystemDictTypeRecord>
            columns={typeColumns}
            dataSource={dictTypes}
            rowKey="id"
            loading={typeLoading}
            error={typeError}
            minWidth={1000}
            onRowClick={selectType}
            rowClassName={(record) =>
              selectedType?.id === record.id ? "bg-primary/5" : undefined
            }
            empty={
              <EmptyState
                title="暂无字典类型"
                description="调整筛选条件后重新查询。"
                actionText="重置筛选"
                onAction={resetTypeFilters}
              />
            }
          />
          <Pagination
            page={typePage}
            pageSize={typePageSize}
            total={typeTotal}
            disabled={typeLoading}
            onPageChange={setTypePage}
            onPageSizeChange={setTypePageSize}
          />
        </DataTableCard>

        {selectedType && (
          <DataTableCard>
            <TableToolbar
              title="字典项"
              description={`当前类型：${selectedType.dictName} / ${selectedType.dictCode}`}
              actions={
                <div className="flex items-center gap-2">
                  <StatusTag
                    tone={itemLoading ? "warning" : itemError ? "error" : "info"}
                  >
                    {itemLoading ? "加载中" : itemError ? "加载失败" : "已同步"}
                  </StatusTag>
                  <Button size="sm" variant="secondary" onClick={closeTypePanel}>
                    <X className="h-4 w-4" aria-hidden />
                    收起
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={!selectedType}
                    onClick={openCreateDataForm}
                  >
                    <Plus className="h-4 w-4" aria-hidden />
                    新增项
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={selectedData.length === 0}
                    onClick={() => setConfirmAction({ type: "batchDeleteData", dictData: selectedData })}
                  >
                    <Trash2 className="h-4 w-4" aria-hidden />
                    批量删除
                  </Button>
                </div>
              }
            />
            <div className="border-b border-border p-4">
              <form className="flex flex-wrap gap-3" onSubmit={submitItemFilters}>
                <div className="relative min-w-[180px] flex-1">
                  <Search
                    className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary"
                    aria-hidden
                  />
                  <Input
                    value={itemFilters.dictLabel}
                    onChange={(event) =>
                      setItemFilters((current) => ({
                        ...current,
                        dictLabel: event.target.value,
                      }))
                    }
                    placeholder="字典项名称"
                    className="pl-9"
                    disabled={!selectedType}
                  />
                </div>
                <Input
                  value={itemFilters.dictValue}
                  onChange={(event) =>
                    setItemFilters((current) => ({
                      ...current,
                      dictValue: event.target.value,
                    }))
                  }
                  placeholder="字典项值"
                  className="min-w-[160px] flex-1"
                  disabled={!selectedType}
                />
                <Button variant="secondary" onClick={resetItemFilters}>
                  重置
                </Button>
                <Button variant="primary" type="submit" disabled={!selectedType}>
                  查询
                </Button>
              </form>
            </div>
            <DataTable<SystemDictDataRecord>
              columns={itemColumns}
              dataSource={dictItems}
              rowKey="id"
              loading={itemLoading}
              error={itemError}
              minWidth={830}
              empty={
                <EmptyState
                  title="暂无字典项"
                  description="当前字典类型下没有匹配的字典项。"
                  actionText="重置筛选"
                  onAction={resetItemFilters}
                />
              }
            />
            <Pagination
              page={itemPage}
              pageSize={itemPageSize}
              total={itemTotal}
              disabled={itemLoading || !selectedType}
              onPageChange={setItemPage}
              onPageSizeChange={(nextPageSize) => {
                setItemPageSize(nextPageSize);
                setItemPage(1);
              }}
            />
          </DataTableCard>
        )}
      </div>

      <DictTypeFormDialog
        open={typeFormOpen}
        mode={typeFormMode}
        form={typeForm}
        loading={typeSubmitting}
        editingType={editingType}
        statusOptions={statusDict.options}
        onCancel={() => setTypeFormOpen(false)}
        onSubmit={submitTypeForm}
      />

      <DictDataFormDialog
        open={dataFormOpen}
        mode={dataFormMode}
        form={dataForm}
        loading={dataSubmitting}
        dictType={selectedType}
        onCancel={() => setDataFormOpen(false)}
        onSubmit={submitDataForm}
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
