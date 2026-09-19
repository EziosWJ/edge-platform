import type { UseFormReturn } from "react-hook-form";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { DictSelectOption } from "@/constants/dicts";
import type { ApiStatus, RoleListRecord } from "@/types";
import type { RoleFormMode, RoleFormValues } from "./schema";

type RoleFormDialogProps = {
  open: boolean;
  mode: RoleFormMode;
  form: UseFormReturn<RoleFormValues>;
  loading: boolean;
  editingRole: RoleListRecord | null;
  statusOptions: DictSelectOption<ApiStatus>[];
  onCancel: () => void;
  onSubmit: (values: RoleFormValues) => void;
};

export function RoleFormDialog({
  open,
  mode,
  form,
  loading,
  editingRole,
  statusOptions,
  onCancel,
  onSubmit,
}: RoleFormDialogProps) {
  const {
    formState: { errors },
    handleSubmit,
    register,
  } = form;
  const isBuiltin = mode === "edit" && editingRole?.isBuiltin === 1;

  return (
    <FormDialog
      open={open}
      title={mode === "edit" ? "编辑角色" : "新建角色"}
      description="内置角色的角色编码不可修改。"
      loading={loading}
      loadingText="保存中..."
      contentClassName="max-w-modal-md"
      bodyClassName="max-h-[calc(100vh-150px)] px-card py-space-5"
      closeOnEscape={false}
      closeOnOverlayClick={false}
      trapFocus={false}
      onCancel={onCancel}
      onSubmit={() => void handleSubmit(onSubmit)()}
    >
      <div className="grid gap-4 md:grid-cols-2">
            <Field
              label="角色名称"
              htmlFor="roleName"
              required
              error={errors.roleName?.message}
            >
              <Input
                id="roleName"
                disabled={loading}
                placeholder="例如：运营管理员"
                {...register("roleName")}
              />
            </Field>

            <Field
              label="角色编码"
              htmlFor="roleCode"
              required
              error={errors.roleCode?.message}
              help={isBuiltin ? "内置角色编码由系统维护。" : undefined}
            >
              <Input
                id="roleCode"
                disabled={loading}
                readOnly={isBuiltin}
                className={isBuiltin ? "bg-neutral-background text-text-tertiary" : undefined}
                placeholder="例如：operation_admin"
                {...register("roleCode")}
              />
            </Field>

            <Field label="状态" htmlFor="status" error={errors.status?.message}>
              <Select id="status" disabled={loading} {...register("status")}>
                {statusOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>

            <Field
              label="排序"
              htmlFor="sortOrder"
              error={errors.sortOrder?.message}
            >
              <Input
                id="sortOrder"
                type="number"
                min={0}
                inputMode="numeric"
                disabled={loading}
                {...register("sortOrder")}
              />
            </Field>
      </div>

      <div className="mt-4">
            <Field label="备注" htmlFor="remark" error={errors.remark?.message}>
              <Textarea
                id="remark"
                disabled={loading}
                placeholder="补充角色用途或管理说明"
                {...register("remark")}
              />
            </Field>
      </div>
    </FormDialog>
  );
}
