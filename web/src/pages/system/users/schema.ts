import { z } from "zod";
import type { ApiStatus, UserRecord } from "@/types";

export type FilterState = {
  username: string;
  nickname: string;
  phone: string;
  email: string;
  status: "all" | ApiStatus;
  deptId: string;
};

export type UserFormMode = "create" | "edit";

export const DEFAULT_FILTERS: FilterState = {
  username: "",
  nickname: "",
  phone: "",
  email: "",
  status: "all",
  deptId: "",
};

const emptyToUndefined = (value: unknown) =>
  value === "" || value === null ? undefined : value;

export const userFormSchema = z.object({
  username: z
    .string()
    .trim()
    .min(2, "用户名至少 2 个字符")
    .max(32, "用户名不能超过 32 个字符"),
  nickname: z
    .string()
    .trim()
    .min(1, "昵称不能为空")
    .max(32, "昵称不能超过 32 个字符"),
  phone: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(20, "手机号不能超过 20 个字符").optional(),
  ),
  email: z.preprocess(
    emptyToUndefined,
    z.string().trim().email("邮箱格式不正确").optional(),
  ),
  gender: z.enum(["MALE", "FEMALE", "UNKNOWN"]),
  deptId: z.preprocess(
    emptyToUndefined,
    z.coerce
      .number()
      .int("部门 ID 必须是整数")
      .positive("部门 ID 必须大于 0")
      .optional(),
  ),
  status: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
  remark: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
  ),
});

export type UserFormValues = z.infer<typeof userFormSchema>;

export function buildQuery(
  filters: FilterState,
  page: number,
  pageSize: number,
) {
  return {
    page,
    pageSize,
    username: filters.username.trim() || undefined,
    nickname: filters.nickname.trim() || undefined,
    phone: filters.phone.trim() || undefined,
    email: filters.email.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
    deptId: filters.deptId ? Number(filters.deptId) : undefined,
  };
}

export function buildUserPayload(values: UserFormValues) {
  return {
    username: values.username.trim(),
    nickname: values.nickname.trim(),
    phone: values.phone?.trim(),
    email: values.email?.trim(),
    gender: values.gender,
    deptId: values.deptId,
    status: values.status,
    remark: values.remark?.trim(),
  };
}

export function buildUserUpdatePayload(values: UserFormValues) {
  const payload = buildUserPayload(values);
  return {
    nickname: payload.nickname,
    phone: payload.phone,
    email: payload.email,
    gender: payload.gender,
    deptId: payload.deptId,
    status: payload.status,
    remark: payload.remark,
  };
}

export function toFormValues(user?: UserRecord): UserFormValues {
  return {
    username: user?.username ?? "",
    nickname: user?.nickname ?? "",
    phone: user?.phone ?? "",
    email: user?.email ?? "",
    gender: user?.gender ?? "UNKNOWN",
    deptId: user?.deptId ?? undefined,
    status: user?.status === 0 ? 0 : 1,
    remark: user?.remark ?? "",
  };
}
