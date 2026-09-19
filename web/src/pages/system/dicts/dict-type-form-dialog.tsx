import type { UseFormReturn } from "react-hook-form";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { DictSelectOption } from "@/constants/dicts";
import type { ApiStatus, SystemDictTypeRecord } from "@/types";
import type { DictTypeFormValues, FormMode } from "./schema";

type DictTypeFormDialogProps = {
  open: boolean;
  mode: FormMode;
  form: UseFormReturn<DictTypeFormValues>;
  loading: boolean;
  editingType: SystemDictTypeRecord | null;
  statusOptions: DictSelectOption<ApiStatus>[];
  onCancel: () => void;
  onSubmit: (values: DictTypeFormValues) => void | Promise<void>;
};

export function DictTypeFormDialog({
  open,
  mode,
  form,
  loading,
  editingType,
  statusOptions,
  onCancel,
  onSubmit,
}: DictTypeFormDialogProps) {
  const {
    formState: { errors },
    handleSubmit,
    register,
  } = form;
  const isBuiltin = mode === "edit" && editingType?.isBuiltin === 1;

  return (
    <FormDialog
      open={open}
      title={mode === "edit" ? "编辑字典类型" : "新建字典类型"}
      description="内置字典的字典编码不可修改。"
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
              label="字典名称"
              htmlFor="dictName"
              required
              error={errors.dictName?.message}
            >
              <Input
                id="dictName"
                disabled={loading}
                placeholder="例如：性别"
                {...register("dictName")}
              />
            </Field>

            <Field
              label="字典编码"
              htmlFor="dictCode"
              required
              error={errors.dictCode?.message}
              help={isBuiltin ? "内置字典编码由系统维护。" : undefined}
            >
              <Input
                id="dictCode"
                disabled={loading}
                readOnly={isBuiltin}
                className={isBuiltin ? "bg-neutral-background text-text-tertiary" : undefined}
                placeholder="例如：gender"
                {...register("dictCode")}
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
                placeholder="补充字典用途或维护说明"
                {...register("remark")}
              />
            </Field>
      </div>
    </FormDialog>
  );
}
