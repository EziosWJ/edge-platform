import type { UseFormReturn } from "react-hook-form";
import {
  deptOptionsToTreeSelectNodes,
} from "@/api/dept";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { TreeSelect } from "@/components/common/tree-select";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { DictSelectOption } from "@/constants/dicts";
import type { ApiStatus, DeptOption, DeptRecord } from "@/types";
import type { DeptFormMode, DeptFormValues } from "./schema";

type DeptFormDialogProps = {
  open: boolean;
  mode: DeptFormMode;
  form: UseFormReturn<DeptFormValues>;
  loading: boolean;
  optionsLoading: boolean;
  editingDept: DeptRecord | null;
  deptOptions: DeptOption[];
  statusOptions: DictSelectOption<ApiStatus>[];
  onCancel: () => void;
  onSubmit: (values: DeptFormValues) => void | Promise<void>;
};

function collectDeptBranchIds(options: DeptOption[], targetId?: number | null) {
  if (!targetId) return [];

  const findBranch = (items: DeptOption[]): DeptOption | null => {
    for (const item of items) {
      if (item.id === targetId) return item;
      const matched = item.children?.length ? findBranch(item.children) : null;
      if (matched) return matched;
    }

    return null;
  };

  const collectIds = (item: DeptOption): number[] => [
    item.id,
    ...(item.children ?? []).flatMap(collectIds),
  ];

  const branch = findBranch(options);
  return branch ? collectIds(branch) : [targetId];
}

export function DeptFormDialog({
  open,
  mode,
  form,
  loading,
  optionsLoading,
  editingDept,
  deptOptions,
  statusOptions,
  onCancel,
  onSubmit,
}: DeptFormDialogProps) {
  const {
    formState: { errors },
    handleSubmit,
    register,
    setValue,
    watch,
  } = form;

  const disabledParentIds = collectDeptBranchIds(deptOptions, editingDept?.id);
  const parentTreeNodes = deptOptionsToTreeSelectNodes(
    deptOptions,
    disabledParentIds,
  );
  const parentId = watch("parentId");
  const isBuiltin = editingDept?.isBuiltin === 1;

  return (
    <FormDialog
      open={open}
      title={mode === "create" ? "新建部门" : "编辑部门"}
      description={
        isBuiltin
          ? "内置部门编码不建议修改，已在表单中锁定。"
          : "维护部门基础信息和启用状态。"
      }
      loading={loading}
      contentClassName="max-w-[760px]"
      bodyClassName="max-h-[calc(100vh-184px)] px-card py-space-4"
      headerClassName="items-center gap-0"
      closeOnEscape={false}
      closeOnOverlayClick={false}
      trapFocus={false}
      onCancel={onCancel}
      onSubmit={() => void handleSubmit(onSubmit)()}
    >
      <div className="grid gap-4 md:grid-cols-2">
            <Field
              label="上级部门"
              error={errors.parentId?.message}
              help={optionsLoading ? "部门树加载中..." : "不选择时创建为顶级部门。"}
            >
              <TreeSelect
                value={parentId ?? null}
                nodes={parentTreeNodes}
                placeholder={optionsLoading ? "部门树加载中" : "请选择上级部门"}
                disabled={loading || optionsLoading}
                onChange={(value) => {
                  setValue(
                    "parentId",
                    value == null ? undefined : Number(value),
                    {
                      shouldDirty: true,
                      shouldValidate: true,
                    },
                  );
                }}
              />
            </Field>
            <Field label="部门名称" error={errors.deptName?.message} required>
              <Input
                {...register("deptName")}
                placeholder="请输入部门名称"
                disabled={loading}
              />
            </Field>
            <Field label="部门编码" error={errors.deptCode?.message} required>
              <Input
                {...register("deptCode")}
                placeholder="请输入部门编码"
                disabled={loading || isBuiltin}
              />
            </Field>
            <Field label="负责人" error={errors.leader?.message}>
              <Input
                {...register("leader")}
                placeholder="请输入负责人"
                disabled={loading}
              />
            </Field>
            <Field label="联系电话" error={errors.phone?.message}>
              <Input
                {...register("phone")}
                placeholder="请输入联系电话"
                disabled={loading}
                autoComplete="tel"
              />
            </Field>
            <Field label="邮箱" error={errors.email?.message}>
              <Input
                {...register("email")}
                placeholder="请输入邮箱"
                disabled={loading}
                autoComplete="email"
              />
            </Field>
            <Field label="排序" error={errors.sortOrder?.message}>
              <Input
                {...register("sortOrder", { valueAsNumber: true })}
                type="number"
                min={0}
                placeholder="请输入排序"
                disabled={loading}
              />
            </Field>
            <Field label="状态" error={errors.status?.message}>
              <Select
                {...register("status", { valueAsNumber: true })}
                disabled={loading}
              >
                {statusOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>
            <div className="md:col-span-2">
              <Field label="备注" error={errors.remark?.message}>
                <Textarea
                  {...register("remark")}
                  placeholder="请输入备注"
                  disabled={loading}
                />
              </Field>
            </div>
      </div>
    </FormDialog>
  );
}
