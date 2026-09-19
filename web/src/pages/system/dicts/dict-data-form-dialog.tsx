import type { UseFormReturn } from "react-hook-form";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { SystemDictTypeRecord } from "@/types";
import type { DictDataFormValues, FormMode } from "./schema";

type DictDataFormDialogProps = {
  open: boolean;
  mode: FormMode;
  form: UseFormReturn<DictDataFormValues>;
  loading: boolean;
  dictType?: SystemDictTypeRecord | null;
  onCancel: () => void;
  onSubmit: (values: DictDataFormValues) => void | Promise<void>;
};

export function DictDataFormDialog({
  open,
  mode,
  form,
  loading,
  dictType,
  onCancel,
  onSubmit,
}: DictDataFormDialogProps) {
  const {
    formState: { errors },
    handleSubmit,
    register,
  } = form;

  return (
    <FormDialog
      open={open}
      title={mode === "edit" ? "编辑字典项" : "新建字典项"}
      description={
        dictType
          ? `${dictType.dictName} / ${dictType.dictCode}`
          : "未选择字典类型"
      }
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
              label="字典项名称"
              htmlFor="dictLabel"
              required
              error={errors.dictLabel?.message}
            >
              <Input
                id="dictLabel"
                disabled={loading}
                placeholder="例如：男"
                {...register("dictLabel")}
              />
            </Field>

            <Field
              label="字典项值"
              htmlFor="dictValue"
              required
              error={errors.dictValue?.message}
            >
              <Input
                id="dictValue"
                disabled={loading}
                placeholder="例如：MALE"
                {...register("dictValue")}
              />
            </Field>

            <Field
              label="排序"
              htmlFor="dataSortOrder"
              error={errors.sortOrder?.message}
            >
              <Input
                id="dataSortOrder"
                type="number"
                min={0}
                inputMode="numeric"
                disabled={loading}
                {...register("sortOrder")}
              />
            </Field>
      </div>

      <div className="mt-4">
            <Field
              label="备注"
              htmlFor="dataRemark"
              error={errors.remark?.message}
            >
              <Textarea
                id="dataRemark"
                disabled={loading}
                placeholder="补充字典项说明"
                {...register("remark")}
              />
            </Field>
      </div>
    </FormDialog>
  );
}
