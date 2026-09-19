import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { updateFile } from "@/api/file";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { toast } from "@/components/common/toast-store";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import type { DictSelectOption } from "@/constants/dicts";
import type { FileRecord } from "@/types";
import { getErrorMessage } from "@/lib/api-error";

const fileEditSchema = z.object({
  businessModule: z
    .string()
    .max(100, "业务模块不能超过 100 个字符")
    .optional(),
  remark: z.string().max(500, "备注不能超过 500 个字符").optional(),
});

type FileEditValues = z.infer<typeof fileEditSchema>;

type FileEditDialogProps = {
  open: boolean;
  record: FileRecord | null;
  businessModuleOptions: DictSelectOption[];
  onCancel: () => void;
  onSaved: () => void;
};

export function FileEditDialog({
  open,
  record,
  businessModuleOptions,
  onCancel,
  onSaved,
}: FileEditDialogProps) {
  const {
    formState: { errors, isSubmitting },
    handleSubmit,
    register,
    reset,
  } = useForm<FileEditValues>({
    resolver: zodResolver(fileEditSchema),
    defaultValues: {
      businessModule: "",
      remark: "",
    },
  });

  useEffect(() => {
    if (open && record) {
      reset({
        businessModule: record.businessModule ?? "",
        remark: record.remark ?? "",
      });
    }
  }, [open, record, reset]);

  const onSubmit = async (values: FileEditValues) => {
    if (!record) return;

    try {
      await updateFile(record.id, {
        businessModule: values.businessModule?.trim() || undefined,
        remark: values.remark?.trim() || undefined,
      });
      toast.success("文件信息已更新");
      onSaved();
    } catch (submitError) {
      toast.error({
        title: "更新失败",
        description: getErrorMessage(submitError, "请检查表单后重试"),
      });
    }
  };

  return (
    <FormDialog
      open={open}
      title="编辑文件信息"
      description={record?.originalName}
      loading={isSubmitting}
      submitText="保存"
      onCancel={onCancel}
      onSubmit={handleSubmit(onSubmit)}
    >
      <div className="space-y-4">
        <Field
          label="业务模块"
          htmlFor="edit-business-module"
          error={errors.businessModule?.message}
          help="可选，用于标识文件所属业务"
        >
          {businessModuleOptions.length > 0 ? (
            <Select
              id="edit-business-module"
              disabled={isSubmitting}
              {...register("businessModule")}
            >
              <option value="">请选择业务模块</option>
              {businessModuleOptions.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </Select>
          ) : (
            <Input
              id="edit-business-module"
              disabled={isSubmitting}
              placeholder="例如：user、system-config"
              {...register("businessModule")}
            />
          )}
        </Field>
        <Field
          label="备注"
          htmlFor="edit-remark"
          error={errors.remark?.message}
        >
          <Textarea
            id="edit-remark"
            disabled={isSubmitting}
            placeholder="补充说明"
            rows={3}
            {...register("remark")}
          />
        </Field>
      </div>
    </FormDialog>
  );
}
