import { KeyRound, RotateCcw, Save } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { changeCurrentUserPassword } from "@/api/account";
import { ContentCard } from "@/components/common/content-card";
import { Field } from "@/components/common/field";
import { FormSection } from "@/components/common/form-section";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { isApiError } from "@/lib/api-error";

const passwordSchema = z
  .object({
    oldPassword: z.string().min(1, "请输入当前密码"),
    newPassword: z
      .string()
      .min(6, "新密码不少于 6 位")
      .max(32, "新密码不能超过 32 个字符"),
    confirmPassword: z.string().min(1, "请再次输入新密码"),
  })
  .refine((values) => values.newPassword === values.confirmPassword, {
    path: ["confirmPassword"],
    message: "两次输入的新密码不一致",
  })
  .refine((values) => values.oldPassword !== values.newPassword, {
    path: ["newPassword"],
    message: "新密码不能与当前密码相同",
  });

type PasswordFormValues = z.infer<typeof passwordSchema>;

const defaultValues: PasswordFormValues = {
  oldPassword: "",
  newPassword: "",
  confirmPassword: "",
};

function getErrorMessage(error: unknown) {
  if (isApiError(error)) return error.message;
  if (error instanceof Error) return error.message;
  return "密码修改失败";
}

export function ChangePasswordPage() {
  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<PasswordFormValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues,
    mode: "onBlur",
  });

  const onSubmit = async (values: PasswordFormValues) => {
    try {
      await changeCurrentUserPassword({
        oldPassword: values.oldPassword,
        newPassword: values.newPassword,
      });
      reset(defaultValues);
      toast.success("密码已修改");
    } catch (error) {
      if (isApiError(error) && error.fieldErrors) {
        Object.entries(error.fieldErrors).forEach(([field, message]) => {
          if (field === "oldPassword" || field === "newPassword") {
            setError(field, { type: "server", message });
          }
        });
      }

      toast.error({
        title: "密码修改失败",
        description: getErrorMessage(error),
      });
    }
  };

  const handleReset = () => {
    reset(defaultValues);
  };

  return (
    <>
      <PageHeader
        title="修改密码"
        description="修改当前登录账号密码。"
      />

      <div className="grid max-w-[1120px] gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
        <ContentCard title="安全提示">
          <div className="flex items-start gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-50 text-primary">
              <KeyRound className="h-5 w-5" aria-hidden />
            </div>
            <div>
              <div className="font-medium text-text-primary">密码规则</div>
              <p className="mt-1 text-sm text-text-tertiary">
                原密码必填，新密码不少于 6 位，确认密码必须与新密码一致。
              </p>
              <div className="mt-3">
                <StatusTag tone="info">账号安全</StatusTag>
              </div>
            </div>
          </div>
        </ContentCard>

        <form className="space-y-6" onSubmit={handleSubmit(onSubmit)} noValidate>
          <FormSection title="密码信息" description="请填写当前密码和新密码。">
            <div className="md:col-span-2">
              <Field
                label="当前密码"
                htmlFor="oldPassword"
                required
                error={errors.oldPassword?.message}
              >
                <Input
                  id="oldPassword"
                  type="password"
                  autoComplete="current-password"
                  disabled={isSubmitting}
                  {...register("oldPassword")}
                />
              </Field>
            </div>

            <Field
              label="新密码"
              htmlFor="newPassword"
              required
              error={errors.newPassword?.message}
            >
              <Input
                id="newPassword"
                type="password"
                autoComplete="new-password"
                disabled={isSubmitting}
                {...register("newPassword")}
              />
            </Field>

            <Field
              label="确认新密码"
              htmlFor="confirmPassword"
              required
              error={errors.confirmPassword?.message}
            >
              <Input
                id="confirmPassword"
                type="password"
                autoComplete="new-password"
                disabled={isSubmitting}
                {...register("confirmPassword")}
              />
            </Field>
          </FormSection>

          <div className="flex flex-col gap-3 rounded-admin border border-border bg-surface px-5 py-4 shadow-admin md:flex-row md:items-center md:justify-between">
            <p className="text-sm text-text-tertiary">
              修改成功后请使用新密码继续登录。
            </p>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="secondary"
                onClick={handleReset}
                disabled={isSubmitting}
              >
                <RotateCcw className="h-4 w-4" aria-hidden />
                重置
              </Button>
              <Button type="submit" variant="primary" disabled={isSubmitting}>
                <Save className="h-4 w-4" aria-hidden />
                {isSubmitting ? "提交中..." : "提交"}
              </Button>
            </div>
          </div>
        </form>
      </div>
    </>
  );
}
