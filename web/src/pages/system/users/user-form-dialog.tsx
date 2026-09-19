import type { ReactNode } from "react";
import type { UseFormReturn } from "react-hook-form";
import { deptOptionsToTreeSelectNodes } from "@/api/dept";
import { FormDialog } from "@/components/common/form-dialog";
import { TreeSelect } from "@/components/common/tree-select";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { DictSelectOption } from "@/constants/dicts";
import type { ApiStatus, DeptOption, UserGender } from "@/types";
import type { UserFormMode, UserFormValues } from "./schema";

type UserFormDialogProps = {
  open: boolean;
  mode: UserFormMode;
  form: UseFormReturn<UserFormValues>;
  loading: boolean;
  optionsLoading: boolean;
  deptOptions: DeptOption[];
  genderOptions: DictSelectOption<UserGender>[];
  statusOptions: DictSelectOption<ApiStatus>[];
  onCancel: () => void;
  onSubmit: (values: UserFormValues) => void | Promise<void>;
};

export function UserFormDialog({
  open,
  mode,
  form,
  loading,
  optionsLoading,
  deptOptions,
  genderOptions,
  statusOptions,
  onCancel,
  onSubmit,
}: UserFormDialogProps) {
  const {
    formState: { errors },
    handleSubmit,
    register,
    setValue,
    watch,
  } = form;
  const deptId = watch("deptId");
  const deptTreeNodes = deptOptionsToTreeSelectNodes(deptOptions);

  return (
    <FormDialog
      open={open}
      title={mode === "create" ? "新建用户" : "编辑用户"}
      description="新增用户默认密码由后端生成，请选择用户所属部门。"
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
            <FormField label="用户名" error={errors.username?.message} required>
              <Input
                {...register("username")}
                placeholder="请输入用户名"
                disabled={loading || mode === "edit"}
                autoComplete="username"
              />
            </FormField>
            <FormField label="昵称" error={errors.nickname?.message} required>
              <Input
                {...register("nickname")}
                placeholder="请输入昵称"
                disabled={loading}
              />
            </FormField>
            <FormField label="手机号" error={errors.phone?.message}>
              <Input
                {...register("phone")}
                placeholder="请输入手机号"
                disabled={loading}
                autoComplete="tel"
              />
            </FormField>
            <FormField label="邮箱" error={errors.email?.message}>
              <Input
                {...register("email")}
                placeholder="请输入邮箱"
                disabled={loading}
                autoComplete="email"
              />
            </FormField>
            <FormField label="性别" error={errors.gender?.message}>
              <Select {...register("gender")} disabled={loading}>
                {genderOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </FormField>
            <FormField label="状态" error={errors.status?.message}>
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
            </FormField>
            <FormField label="所属部门" error={errors.deptId?.message}>
              <TreeSelect
                value={deptId ?? null}
                nodes={deptTreeNodes}
                placeholder={optionsLoading ? "部门树加载中" : "请选择所属部门"}
                disabled={loading || optionsLoading}
                onChange={(value) => {
                  setValue("deptId", value == null ? undefined : Number(value), {
                    shouldDirty: true,
                    shouldValidate: true,
                  });
                }}
              />
            </FormField>
            <FormField
              label="备注"
              error={errors.remark?.message}
              className="md:col-span-2"
            >
              <Textarea
                {...register("remark")}
                placeholder="请输入备注"
                disabled={loading}
              />
            </FormField>
      </div>
    </FormDialog>
  );
}

function FormField({
  label,
  error,
  required = false,
  className,
  children,
}: {
  label: string;
  error?: string;
  required?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <label className={className}>
      <span className="mb-1.5 block text-sm font-medium text-text-primary">
        {label}
        {required && <span className="ml-1 text-error">*</span>}
      </span>
      {children}
      <span className="mt-1 block min-h-[18px] text-xs text-error">
        {error ?? ""}
      </span>
    </label>
  );
}
