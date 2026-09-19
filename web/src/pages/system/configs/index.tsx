import { zodResolver } from "@hookform/resolvers/zod";
import {
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useForm, type UseFormReturn } from "react-hook-form";
import { z } from "zod";
import {
  batchDeleteSystemConfigs,
  createSystemConfig,
  deleteSystemConfig,
  getSystemConfigDetail,
  getSystemConfigPage,
  updateSystemConfig,
  updateSystemConfigStatus,
} from "@/api/system";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { DataTableCard } from "@/components/common/data-table-card";
import { DataTable } from "@/components/common/data-table";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  API_STATUS_VALUES,
  COMMON_STATUS_OPTIONS,
  CONFIG_TYPE_OPTIONS,
  CONFIG_TYPE_VALUES,
  CONFIG_VALUE_TYPE_OPTIONS,
  CONFIG_VALUE_TYPE_VALUES,
  DICT_CODES,
  type DictSelectOption,
} from "@/constants/dicts";
import { useDictOptions } from "@/hooks/use-dict-options";
import { useListPage } from "@/hooks/use-list-page";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import { cn } from "@/lib/utils";
import type {
  ApiStatus,
  DataTableColumn,
  SystemConfigRecord,
  SystemConfigType,
  SystemConfigValueType,
} from "@/types";

type FilterState = {
  configName: string;
  configKey: string;
  configType: "all" | SystemConfigType;
  status: "all" | ApiStatus;
};

type FormMode = "create" | "edit";

type ConfirmAction =
  | { type: "delete"; config: SystemConfigRecord }
  | { type: "batchDelete"; configs: SystemConfigRecord[] }
  | { type: "status"; config: SystemConfigRecord; status: ApiStatus };

const DEFAULT_FILTERS: FilterState = {
  configName: "",
  configKey: "",
  configType: "all",
  status: "all",
};

const LOG_CLEAR_ENABLED_KEY = "system.log-clear-enabled";

const statusMeta: Record<ApiStatus, { label: string; tone: "success" | "neutral" }> = {
  1: { label: "启用", tone: "success" },
  0: { label: "禁用", tone: "neutral" },
};

const emptyToUndefined = (value: unknown) =>
  value === "" || value === null ? undefined : value;

const configFormSchema = z
  .object({
    configName: z
      .string()
      .trim()
      .min(1, "配置名称不能为空")
      .max(100, "配置名称不能超过 100 个字符"),
    configKey: z
      .string()
      .trim()
      .min(1, "配置键不能为空")
      .max(100, "配置键不能超过 100 个字符"),
    configValue: z.string().max(500, "配置值不能超过 500 个字符"),
    configType: z.enum(["SYSTEM", "CUSTOM"]),
    valueType: z.enum(["TEXT", "NUMBER", "BOOLEAN"]),
    status: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
    remark: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
    ),
  })
  .superRefine((values, context) => {
    const value = values.configValue.trim();

    if (values.valueType === "NUMBER" && value && !Number.isFinite(Number(value))) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["configValue"],
        message: "数字类型的配置值必须是合法数字",
      });
    }

    if (
      values.valueType === "BOOLEAN" &&
      value !== "true" &&
      value !== "false"
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["configValue"],
        message: "布尔类型的配置值必须为 true 或 false",
      });
    }
  });

type ConfigFormValues = z.infer<typeof configFormSchema>;

function buildQuery(filters: FilterState, page: number, pageSize: number) {
  return {
    page,
    pageSize,
    configName: filters.configName.trim() || undefined,
    configKey: filters.configKey.trim() || undefined,
    configType: filters.configType === "all" ? undefined : filters.configType,
    status: filters.status === "all" ? undefined : filters.status,
  };
}

function getConfigStatus(config?: Pick<SystemConfigRecord, "status">): ApiStatus {
  return config?.status === 0 || config?.status === "disabled" ? 0 : 1;
}

function isBuiltinConfig(config: SystemConfigRecord) {
  return config.isBuiltin === 1;
}

function isEditableBuiltinConfig(config: SystemConfigRecord) {
  return (
    isBuiltinConfig(config) &&
    config.configKey === LOG_CLEAR_ENABLED_KEY &&
    config.valueType === "BOOLEAN"
  );
}

function getConfigTypeLabel(
  value: SystemConfigType | undefined,
  options: Array<DictSelectOption<SystemConfigType>>,
) {
  return options.find((item) => item.value === value)?.label ?? value ?? "-";
}

function getValueTypeLabel(
  value: SystemConfigValueType | undefined,
  options: Array<DictSelectOption<SystemConfigValueType>>,
) {
  return options.find((item) => item.value === value)?.label ?? value ?? "-";
}

function toFormValues(config?: SystemConfigRecord): ConfigFormValues {
  const valueType = config?.valueType ?? "TEXT";
  const configValue = config?.configValue ?? "";

  return {
    configName: config?.configName ?? "",
    configKey: config?.configKey ?? "",
    configValue:
      valueType === "BOOLEAN" && configValue !== "true" && configValue !== "false"
        ? "false"
        : configValue,
    configType: config?.configType ?? "SYSTEM",
    valueType,
    status: getConfigStatus(config),
    remark: config?.remark ?? "",
  };
}

function buildPayload(values: ConfigFormValues) {
  const normalizedValue =
    values.valueType === "TEXT" ? values.configValue : values.configValue.trim();

  return {
    configName: values.configName.trim(),
    configKey: values.configKey.trim(),
    configValue:
      values.valueType === "BOOLEAN" ? normalizedValue.toLowerCase() : normalizedValue,
    configType: values.configType,
    valueType: values.valueType,
    status: values.status,
    remark: values.remark?.trim(),
  };
}

export function SystemConfigsPage() {
  const {
    data: configs,
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
    reload: loadConfigs,
  } = useListPage<FilterState, SystemConfigRecord>({
    fetch: getSystemConfigPage,
    defaultFilters: DEFAULT_FILTERS,
    toQuery: (f, p, ps) => buildQuery(f, p, ps),
    onError: (err) =>
      toast.error({
        title: "加载失败",
        description: getErrorMessage(err, "配置列表加载失败"),
      }),
  });

  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<FormMode>("create");
  const [editingConfig, setEditingConfig] = useState<SystemConfigRecord | null>(
    null,
  );
  const [submitting, setSubmitting] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const form = useForm<ConfigFormValues>({
    resolver: zodResolver(configFormSchema),
    defaultValues: toFormValues(),
  });
  const configTypeDict = useDictOptions<SystemConfigType>(DICT_CODES.CONFIG_TYPE, {
    fallback: CONFIG_TYPE_OPTIONS,
    allowedValues: CONFIG_TYPE_VALUES,
    showErrorToast: true,
    errorTitle: "配置类型字典加载失败",
  });
  const valueTypeDict = useDictOptions<SystemConfigValueType>(
    DICT_CODES.CONFIG_VALUE_TYPE,
    {
      fallback: CONFIG_VALUE_TYPE_OPTIONS,
      allowedValues: CONFIG_VALUE_TYPE_VALUES,
      showErrorToast: true,
      errorTitle: "配置值类型字典加载失败",
    },
  );
  const statusDict = useDictOptions<ApiStatus>(DICT_CODES.COMMON_STATUS, {
    fallback: COMMON_STATUS_OPTIONS,
    allowedValues: API_STATUS_VALUES,
    valueType: "number",
    showErrorToast: true,
    errorTitle: "配置状态字典加载失败",
  });

  // 同步 selectedIds 与当前页数据
  useEffect(() => {
    setSelectedIds((current) => {
      const nextRecordIds = new Set(configs.map((item) => item.id));
      return new Set([...current].filter((id) => nextRecordIds.has(id)));
    });
  }, [configs]);

  const selectableIds = useMemo(
    () => configs.filter((item) => !isBuiltinConfig(item)).map((item) => item.id),
    [configs],
  );
  const allSelectableChecked =
    selectableIds.length > 0 && selectableIds.every((id) => selectedIds.has(id));
  const selectedConfigs = useMemo(
    () =>
      configs.filter((item) => selectedIds.has(item.id) && !isBuiltinConfig(item)),
    [configs, selectedIds],
  );

  const toggleSelectAll = (checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current);

      selectableIds.forEach((id) => {
        if (checked) {
          next.add(id);
        } else {
          next.delete(id);
        }
      });

      return next;
    });
  };

  const toggleSelect = (id: number, checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  };

  const openCreateForm = () => {
    setFormMode("create");
    setEditingConfig(null);
    form.reset(toFormValues());
    setFormOpen(true);
  };

  const openEditForm = async (config: SystemConfigRecord) => {
    if (isBuiltinConfig(config) && !isEditableBuiltinConfig(config)) {
      toast.warning("内置配置不允许编辑");
      return;
    }

    setFormMode("edit");
    setEditingConfig(config);
    form.reset(toFormValues(config));
    setFormOpen(true);

    try {
      const detail = await getSystemConfigDetail(config.id);
      setEditingConfig(detail);
      form.reset(toFormValues(detail));
    } catch (detailError) {
      toast.error({
        title: "配置详情加载失败",
        description: getErrorMessage(detailError, "无法获取配置详情"),
      });
    }
  };

  const submitForm = async (values: ConfigFormValues) => {
    setSubmitting(true);

    try {
      if (formMode === "edit" && editingConfig) {
        await updateSystemConfig(editingConfig.id, buildPayload(values));
        toast.success("配置已更新");
      } else {
        await createSystemConfig(buildPayload(values));
        toast.success("配置已创建");
      }

      setFormOpen(false);
      await loadConfigs();
    } catch (submitError) {
      if (isApiError(submitError) && submitError.fieldErrors) {
        Object.entries(submitError.fieldErrors).forEach(([field, message]) => {
          form.setError(field as keyof ConfigFormValues, { message });
        });
      }

      toast.error({
        title: formMode === "edit" ? "更新失败" : "创建失败",
        description: getErrorMessage(submitError, "请检查表单后重试"),
      });
    } finally {
      setSubmitting(false);
    }
  };

  const runConfirmAction = async () => {
    if (!confirmAction) return;

    setConfirmLoading(true);

    try {
      if (confirmAction.type === "delete") {
        if (isBuiltinConfig(confirmAction.config)) {
          toast.warning("内置配置不允许删除");
          return;
        }

        await deleteSystemConfig(confirmAction.config.id);
        toast.success("配置已删除");
      }

      if (confirmAction.type === "batchDelete") {
        await batchDeleteSystemConfigs({
          ids: confirmAction.configs.map((item) => item.id),
        });
        toast.success("配置已批量删除");
      }

      if (confirmAction.type === "status") {
        await updateSystemConfigStatus(confirmAction.config.id, {
          status: confirmAction.status,
        });
        toast.success(confirmAction.status === 1 ? "配置已启用" : "配置已禁用");
      }

      setConfirmAction(null);
      setSelectedIds(new Set());
      await loadConfigs();
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
        title: "删除配置",
        description: `确认删除配置「${confirmAction.config.configName ?? "-"}」吗？此操作不可恢复。`,
        confirmText: "删除",
        danger: true,
      };
    }

    if (confirmAction.type === "batchDelete") {
      return {
        title: "批量删除配置",
        description: `确认删除已选择的 ${confirmAction.configs.length} 个普通配置吗？内置配置不会被选中。`,
        confirmText: "批量删除",
        danger: true,
      };
    }

    const enabled = confirmAction.status === 1;
    return {
      title: enabled ? "启用配置" : "禁用配置",
      description: `确认${enabled ? "启用" : "禁用"}配置「${confirmAction.config.configName ?? "-"}」吗？`,
      confirmText: enabled ? "启用" : "禁用",
      danger: !enabled,
    };
  }, [confirmAction]);

  const columns: DataTableColumn<SystemConfigRecord>[] = [
    {
      title: (
        <Checkbox
          aria-label="选择当前页普通配置"
          checked={allSelectableChecked}
          disabled={selectableIds.length === 0}
          onChange={(event) => toggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, record) => {
        const disabled = isBuiltinConfig(record);
        return (
          <Checkbox
            aria-label={`选择配置 ${record.configName ?? record.id}`}
            checked={selectedIds.has(record.id)}
            disabled={disabled}
            title={disabled ? "内置配置不参与批量删除" : undefined}
            onChange={(event) => toggleSelect(record.id, event.target.checked)}
          />
        );
      },
    },
    {
      title: "配置名称",
      key: "configName",
      width: 240,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">
            {record.configName ?? "-"}
          </div>
          <div className="text-xs text-text-tertiary">
            ID {record.id} · {record.configKey ?? "-"}
          </div>
        </div>
      ),
    },
    {
      title: "配置键",
      dataIndex: "configKey",
      width: 240,
      render: (value) => (
        <span className="block max-w-[240px] truncate font-mono text-body-secondary text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "配置值",
      dataIndex: "configValue",
      width: 220,
      render: (value) => (
        <span className="block max-w-[220px] truncate text-text-secondary">
          {String(value ?? "-")}
        </span>
      ),
    },
    {
      title: "配置类型",
      dataIndex: "configType",
      width: 120,
      render: (value) =>
        getConfigTypeLabel(
          value as SystemConfigType | undefined,
          configTypeDict.options,
        ),
    },
    {
      title: "值类型",
      dataIndex: "valueType",
      width: 100,
      render: (value) =>
        getValueTypeLabel(
          value as SystemConfigValueType | undefined,
          valueTypeDict.options,
        ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 96,
      render: (_, record) => {
        const status = getConfigStatus(record);
        const meta = statusMeta[status];
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
      title: "更新时间",
      key: "updateTime",
      width: 180,
      render: (_, record) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatDateTime(record.updateTime ?? record.createTime)}
        </span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 260,
      render: (_, record) => {
        const builtin = isBuiltinConfig(record);
        const editableBuiltin = isEditableBuiltinConfig(record);
        const nextStatus = getConfigStatus(record) === 1 ? 0 : 1;

        return (
          <div className="inline-flex flex-wrap items-center justify-center gap-1">
            <Button
              size="sm"
              variant="ghost"
              disabled={builtin && !editableBuiltin}
              title={
                builtin
                  ? editableBuiltin
                    ? "内置日志清空开关仅允许修改布尔值"
                    : "内置配置不允许编辑"
                  : undefined
              }
              onClick={() => void openEditForm(record)}
            >
              <Pencil className="h-4 w-4" aria-hidden />
              编辑
            </Button>
            <Button
              size="sm"
              variant="ghost"
              title={builtin ? "内置配置允许修改启停状态" : undefined}
              onClick={() =>
                setConfirmAction({
                  type: "status",
                  config: record,
                  status: nextStatus,
                })
              }
            >
              {nextStatus === 1 ? "启用" : "禁用"}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-error hover:text-error"
              disabled={builtin}
              title={builtin ? "内置配置不允许删除" : undefined}
              onClick={() => setConfirmAction({ type: "delete", config: record })}
            >
              <Trash2 className="h-4 w-4" aria-hidden />
              删除
            </Button>
          </div>
        );
      },
    },
  ];

  return (
    <>
      <PageHeader
        title="配置管理"
        description="维护系统级和自定义配置项，支持分页查询、配置值类型校验和启停管理。"
        actions={
          <Button variant="primary" onClick={openCreateForm}>
            <Plus className="h-4 w-4" aria-hidden />
            新增配置
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
            value={filters.configName}
            onChange={(event) => setFilter("configName", event.target.value)}
            placeholder="配置名称"
          />
          <Input
            value={filters.configKey}
            onChange={(event) => setFilter("configKey", event.target.value)}
            placeholder="配置键"
          />
          <Select
            value={filters.configType}
            onChange={(event) =>
              setFilter("configType", event.target.value as FilterState["configType"])
            }
            aria-label="筛选配置类型"
          >
            <option value="all">全部类型</option>
            {configTypeDict.options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
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
            title="配置项列表"
            description={`共 ${total} 条数据，当前显示 ${configs.length} 条。`}
            actions={
              <>
                <StatusTag tone={loading ? "warning" : error ? "error" : "info"}>
                  {loading ? "加载中" : error ? "加载失败" : "已同步"}
                </StatusTag>
                <Button size="sm" variant="secondary" onClick={loadConfigs}>
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={selectedConfigs.length === 0}
                  onClick={() =>
                    setConfirmAction({
                      type: "batchDelete",
                      configs: selectedConfigs,
                    })
                  }
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
        <DataTable<SystemConfigRecord>
          columns={columns}
          dataSource={configs}
          rowKey="id"
          loading={loading}
          error={error}
          minWidth={1440}
          empty={
            <EmptyState
              title="暂无配置项"
              description="调整筛选条件后重新查询。"
              actionText="重置筛选"
              onAction={resetFilters}
            />
          }
        />
      </DataTableCard>

      <ConfigFormDialog
        open={formOpen}
        mode={formMode}
        form={form}
        loading={submitting}
        editingConfig={editingConfig}
        configTypeOptions={configTypeDict.options}
        valueTypeOptions={valueTypeDict.options}
        statusOptions={statusDict.options}
        onCancel={() => setFormOpen(false)}
        onSubmit={submitForm}
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

type ConfigFormDialogProps = {
  open: boolean;
  mode: FormMode;
  form: UseFormReturn<ConfigFormValues>;
  loading: boolean;
  editingConfig: SystemConfigRecord | null;
  configTypeOptions: Array<DictSelectOption<SystemConfigType>>;
  valueTypeOptions: Array<DictSelectOption<SystemConfigValueType>>;
  statusOptions: Array<DictSelectOption<ApiStatus>>;
  onCancel: () => void;
  onSubmit: (values: ConfigFormValues) => void;
};

function ConfigFormDialog({
  open,
  mode,
  form,
  loading,
  editingConfig,
  configTypeOptions,
  valueTypeOptions,
  statusOptions,
  onCancel,
  onSubmit,
}: ConfigFormDialogProps) {
  const {
    formState: { errors },
    handleSubmit,
    register,
    setValue,
    watch,
  } = form;
  const valueType = watch("valueType");
  const readonlyKey = mode === "edit";
  const builtinValueOnly = editingConfig
    ? isEditableBuiltinConfig(editingConfig)
    : false;

  useEffect(() => {
    if (!open || valueType !== "BOOLEAN") return;

    const currentValue = form.getValues("configValue").trim().toLowerCase();
    if (currentValue !== "true" && currentValue !== "false") {
      setValue("configValue", "false", { shouldValidate: true });
    }
  }, [form, open, setValue, valueType]);

  return (
    <FormDialog
      open={open}
      title={mode === "edit" ? "编辑配置" : "新增配置"}
      description="编辑时配置键不可修改，但会随请求体提交给后端校验。"
      loading={loading}
      submitDisabled={editingConfig?.isBuiltin === 1 && !builtinValueOnly}
      loadingText="保存中..."
      contentClassName="max-w-[720px]"
      bodyClassName="max-h-[calc(100vh-150px)] px-card py-space-5"
      closeOnEscape={false}
      closeOnOverlayClick={false}
      trapFocus={false}
      onCancel={onCancel}
      onSubmit={() => void handleSubmit(onSubmit)()}
    >
      <div className="grid gap-4 md:grid-cols-2">
            <Field
              label="配置名称"
              htmlFor="configName"
              required
              error={errors.configName?.message}
            >
              <Input
                id="configName"
                disabled={loading || builtinValueOnly}
                placeholder="例如：系统名称"
                {...register("configName")}
              />
            </Field>

            <Field
              label="配置键"
              htmlFor="configKey"
              required
              error={errors.configKey?.message}
              help={readonlyKey ? "配置键修改会影响调用方，编辑时固定。" : undefined}
            >
              <Input
                id="configKey"
                disabled={loading}
                readOnly={readonlyKey}
                className={cn(readonlyKey && "bg-neutral-background text-text-tertiary")}
                placeholder="例如：system.name"
                {...register("configKey")}
              />
            </Field>

            <Field
              label="配置类型"
              htmlFor="configType"
              error={errors.configType?.message}
            >
              <Select
                id="configType"
                disabled={loading || builtinValueOnly}
                {...register("configType")}
              >
                {configTypeOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>

            <Field
              label="值类型"
              htmlFor="valueType"
              error={errors.valueType?.message}
            >
              <Select
                id="valueType"
                disabled={loading || builtinValueOnly}
                {...register("valueType")}
              >
                {valueTypeOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>

            <Field label="状态" htmlFor="status" error={errors.status?.message}>
              <Select
                id="status"
                disabled={loading || builtinValueOnly}
                {...register("status")}
              >
                {statusOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>

            <Field
              label="配置值"
              htmlFor="configValue"
              error={errors.configValue?.message}
            >
              {valueType === "BOOLEAN" ? (
                <Select
                  id="configValue"
                  disabled={loading}
                  {...register("configValue")}
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </Select>
              ) : (
                <Input
                  id="configValue"
                  type={valueType === "NUMBER" ? "number" : "text"}
                  inputMode={valueType === "NUMBER" ? "decimal" : undefined}
                  disabled={loading}
                  placeholder={valueType === "NUMBER" ? "例如：10" : "配置值"}
                  {...register("configValue")}
                />
              )}
            </Field>
      </div>

      <div className="mt-4">
            <Field label="备注" htmlFor="remark" error={errors.remark?.message}>
              <Textarea
                id="remark"
                disabled={loading || builtinValueOnly}
                placeholder="补充配置用途或维护说明"
                {...register("remark")}
              />
            </Field>
      </div>

          {mode === "edit" && editingConfig?.isBuiltin === 1 && (
            <div className="mt-space-4 rounded-admin border border-border bg-neutral-background px-space-4 py-space-3 text-sm text-text-secondary">
              {builtinValueOnly
                ? "内置日志清空开关仅允许修改布尔值。"
                : "内置配置由系统维护，不允许编辑或删除。"}
            </div>
          )}

    </FormDialog>
  );
}
